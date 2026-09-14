package cap

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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
	mu         sync.Mutex
	handlers   map[string]http.Handler // 能力名 -> handler(动态,随健康变化)
	registered []capMeta
	done       chan struct{}
}

// StartNode 启动 node:① 起 HTTP 服务暴露每个能力(POST /cap/<name>)② 注册到 hub + 心跳。
func StartNode(nc *nats.Conn, cfg Config, caps map[string]Capability) (*Node, error) {
	n := &Node{
		cfg:      cfg,
		caps:     caps,
		nc:       nc,
		handlers: map[string]http.Handler{},
		done:     make(chan struct{}),
	}
	n.recompute() // 初始 handlers + metas

	// ① HTTP 服务:统一分发 /cap/<name> → 动态查 handler
	mux := http.NewServeMux()
	mux.HandleFunc("/cap/", n.dispatchCapability)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

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

// dispatchCapability 按能力名查动态 handler;无则 404。
func (n *Node) dispatchCapability(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/cap/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	n.mu.Lock()
	h := n.handlers[name]
	n.mu.Unlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.ServeHTTP(w, r)
}

// recompute 依据当前健康状态重算 handlers(插件恒在;转发探活通过才挂载)与注册 metas。
func (n *Node) recompute() (handlers map[string]http.Handler, metas []capMeta) {
	handlers = map[string]http.Handler{}
	var out []capMeta

	for name, impl := range n.caps {
		c := impl
		handlers[name] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.HandleHTTP(w, r)
		})
		out = append(out, capMeta{Name: name, Schema: impl.Schema()})
	}
	for _, cc := range n.cfg.Capabilities {
		if cc.Kind != "forward" {
			continue
		}
		if _, isPlugin := n.caps[cc.Name]; isPlugin {
			continue
		}
		if !probeOK(cc.Local, cc.Probe) {
			continue // 当前不健康,不挂载不注册
		}
		target, err := url.Parse(cc.Local)
		if err != nil {
			log.Printf("capability [%s] bad local %q: %v; skipping", cc.Name, cc.Local, err)
			continue
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme = target.Scheme
				pr.Out.URL.Host = target.Host
				pr.Out.URL.Path = target.Path
				pr.Out.URL.RawPath = target.RawPath
				pr.Out.URL.RawQuery = target.RawQuery
			},
		}
		handlers[cc.Name] = proxy
		out = append(out, capMeta{Name: cc.Name, Schema: cc.Schema})
	}
	n.mu.Lock()
	n.handlers = handlers
	n.registered = out
	n.mu.Unlock()
	return handlers, out
}

func (n *Node) republish() {
	_, metas := n.recompute()
	reg := registryEntry{NodeID: n.cfg.NodeID, BaseURL: n.baseURL, Capabilities: metas}
	b, _ := json.Marshal(reg)
	_ = n.nc.Publish("cap.reg.set", b)
}

// republishSync 注册并等 registry ack(启动时用,确保注册完成才返回)。
func (n *Node) republishSync() {
	_, metas := n.recompute()
	reg := registryEntry{NodeID: n.cfg.NodeID, BaseURL: n.baseURL, Capabilities: metas}
	b, _ := json.Marshal(reg)
	_, _ = n.nc.Request("cap.reg.set", b, 2*time.Second)
	n.mu.Lock()
	n.registered = metas
	n.mu.Unlock()
}

// heartbeatLoop 每间隔重发注册(含 set):既续租,又保证 hub 重启后能重新出现。
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
			n.republish()
		case <-n.done:
			return
		}
	}
}

// healthLoop 周期重查转发能力健康;handlers 或注册集合变化时同步更新(先死后活恢复)。
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
			_, cur := n.recompute()
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
	am := map[string]capMeta{}
	for _, m := range a {
		am[m.Name] = m
	}
	for _, m := range b {
		cur, ok := am[m.Name]
		if !ok || cur.Schema != m.Schema {
			return false
		}
	}
	return true
}

func (n *Node) registeredJSON() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, _ := json.Marshal(n.registered)
	return string(b)
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
