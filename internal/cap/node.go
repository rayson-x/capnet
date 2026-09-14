package cap

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// Node 是每台机器上的可扩展外壳:把能力暴露成 HTTP 服务,注册到 hub(心跳续租)。
// 能力两种提供方式:
//   - 插件(Plugin):进程内实现 Capability,请求直接进代码,不经转发。
//   - 转发(Forward):反向代理到本机已有服务(local),探活通过才注册,流式透传。
type Node struct {
	cfg        Config
	caps       map[string]Capability // 插件
	nc         *nats.Conn
	baseURL    string
	ln         net.Listener
	forward    map[string]string // 能力名 -> local 地址(转发)
	mu         *sync.Mutex
	registered []capMeta
	done       chan struct{}
}

// StartNode 启动 node:① 起 HTTP 服务暴露每个能力(POST /cap/<name>)② 注册到 hub + 心跳。
func StartNode(nc *nats.Conn, cfg Config, caps map[string]Capability) (*Node, error) {
	n := &Node{
		cfg:     cfg,
		caps:    caps,
		nc:      nc,
		forward: map[string]string{},
		mu:      &sync.Mutex{},
		done:    make(chan struct{}),
	}

	// ① HTTP 服务
	mux, err := n.buildMux()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, err
	}
	n.ln = ln
	n.baseURL = "http://" + ln.Addr().String()
	go http.Serve(ln, mux)

	// ② 注册 + 心跳 + 健康重查
	n.republishSync() // 等 registry ack,确保注册完成
	go n.heartbeatLoop()
	go n.healthLoop()

	log.Printf("node [%s] registered: base_url=%s capabilities=%s", cfg.NodeID, n.baseURL, n.registeredJSON())
	return n, nil
}

// buildMux 组装能力 handler:插件直接实现;转发探活通过才挂载。
func (n *Node) buildMux() (*http.ServeMux, error) {
	mux := http.NewServeMux()
	for name, impl := range n.caps {
		c := impl
		mux.HandleFunc("/cap/"+name, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			c.HandleHTTP(w, r)
		})
	}
	for _, cc := range n.cfg.Capabilities {
		if cc.Kind != "forward" {
			continue
		}
		if _, exists := n.caps[cc.Name]; exists {
			continue
		}
		if !probeOK(cc.Local, cc.Probe) {
			log.Printf("capability [%s] probe failed, skipping", cc.Name)
			continue
		}
		target, err := url.Parse(cc.Local)
		if err != nil {
			log.Printf("capability [%s] bad local %q: %v; skipping", cc.Name, cc.Local, err)
			continue
		}
		n.forward[cc.Name] = cc.Local
		name := cc.Name
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme = target.Scheme
				pr.Out.URL.Host = target.Host
				pr.Out.URL.Path = target.Path
				pr.Out.URL.RawPath = target.RawPath
				pr.Out.URL.RawQuery = target.RawQuery
			},
		}
		mux.HandleFunc("/cap/"+name, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			proxy.ServeHTTP(w, r)
		})
	}
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux, nil
}

// currentMetas 计算当前应注册的能力列表(插件恒在;转发按当前探活)。
func (n *Node) currentMetas() []capMeta {
	var metas []capMeta
	for name, impl := range n.caps {
		metas = append(metas, capMeta{Name: name, Schema: impl.Schema()})
	}
	for _, cc := range n.cfg.Capabilities {
		if cc.Kind != "forward" {
			continue
		}
		if _, exists := n.caps[cc.Name]; exists {
			continue
		}
		if !probeOK(cc.Local, cc.Probe) {
			continue // 当前不健康,不注册
		}
		metas = append(metas, capMeta{Name: cc.Name, Schema: cc.Schema})
	}
	return metas
}

// republishSync 注册并等 registry ack(启动时用,确保注册完成才返回)。
func (n *Node) republishSync() {
	metas := n.currentMetas()
	reg := registryEntry{NodeID: n.cfg.NodeID, BaseURL: n.baseURL, Capabilities: metas}
	b, _ := json.Marshal(reg)
	_, _ = n.nc.Request("cap.reg.set", b, 2*time.Second)
	n.mu.Lock()
	n.registered = metas
	n.mu.Unlock()
}

// republish 注册(健康重查用,fire-and-forget,ack 不阻塞)。
func (n *Node) republish() {
	metas := n.currentMetas()
	reg := registryEntry{NodeID: n.cfg.NodeID, BaseURL: n.baseURL, Capabilities: metas}
	b, _ := json.Marshal(reg)
	_ = n.nc.Publish("cap.reg.set", b)
	n.mu.Lock()
	n.registered = metas
	n.mu.Unlock()
}

func (n *Node) registeredJSON() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, _ := json.Marshal(n.registered)
	return string(b)
}

func (n *Node) heartbeatLoop() {
	interval := time.Duration(n.cfg.HeartbeatInterval) * time.Second
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = n.nc.Publish("cap.reg.hb."+n.cfg.NodeID, []byte("hb"))
		case <-n.done:
			return
		}
	}
}

// healthLoop 周期重查转发能力健康;集合变化时重新注册。
func (n *Node) healthLoop() {
	interval := time.Duration(n.cfg.HealthInterval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.mu.Lock()
			old := n.registered
			n.mu.Unlock()
			cur := n.currentMetas()
			if !metasEqual(old, cur) {
				log.Printf("node [%s] capabilities changed, re-registering", n.cfg.NodeID)
				n.republish()
			}
		case <-n.done:
			return
		}
	}
}

func metasEqual(a, b []capMeta) bool {
	if len(a) != len(b) {
		return false
	}
	// 顺序无关比较
	am := map[string]capMeta{}
	for _, m := range a {
		am[m.Name] = m
	}
	for _, m := range b {
		if am[m.Name].Name != m.Name || am[m.Name].Schema != m.Schema {
			return false
		}
	}
	return true
}

// BaseURL 返回该 node 的对外可达地址(调用方直连用)。
func (n *Node) BaseURL() string { return n.baseURL }

// Stop 停止心跳与健康循环、关闭 HTTP(不主动注销,由 hub TTL 过期剔除)。
func (n *Node) Stop() {
	close(n.done)
	if n.ln != nil {
		n.ln.Close()
	}
}

// probeOK 探活本机服务:probe 为空视为健康;否则 GET local-base+probe 期望 2xx。
func probeOK(local, probe string) bool {
	if probe == "" {
		return true
	}
	u, err := url.Parse(local)
	if err != nil {
		return false
	}
	base := &url.URL{Scheme: u.Scheme, Host: u.Host}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(base.String() + probe)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
