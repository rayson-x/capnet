package cap

import (
	"encoding/json"
	"log"
	"net"
	"net/http"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

// Node 是每台机器上的可扩展外壳:把能力暴露成 HTTP 服务,并注册到 hub(nats-micro)。
type Node struct {
	cfg     Config
	caps    map[string]Capability // 插件:进程内实现
	nc      *nats.Conn
	baseURL string
	svc     micro.Service
	ln      net.Listener
}

// StartNode 启动 node:① 起 HTTP 服务暴露每个能力(POST /cap/<name>)② 注册到 hub。
// caps 是进程内插件(kind: plugin 的实现)。
func StartNode(nc *nats.Conn, cfg Config, caps map[string]Capability) (*Node, error) {
	n := &Node{cfg: cfg, caps: caps, nc: nc}

	// ① HTTP 服务:每个能力一个端点
	mux := http.NewServeMux()
	for name, impl := range caps {
		c := impl
		mux.HandleFunc("/cap/"+name, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			c.HandleHTTP(w, r)
		})
	}
	// 健康/描述端点(供探活/调试)
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

	// ② 注册到 hub:nats-micro,一个 node 一个服务,能力列表放 metadata
	metas := capabilityMetas(cfg.Capabilities, caps)
	metaJSON, _ := json.Marshal(metas)
	svc, err := micro.AddService(nc, micro.Config{
		Name:    cfg.NodeID,
		Version: "0.1.0",
		Metadata: map[string]string{
			"base_url":     n.baseURL,
			"capabilities": string(metaJSON),
		},
	})
	if err != nil {
		ln.Close()
		return nil, err
	}
	n.svc = svc
	log.Printf("node [%s] registered: base_url=%s capabilities=%s", cfg.NodeID, n.baseURL, metaJSON)
	return n, nil
}

// BaseURL 返回该 node 的对外可达地址(调用方直连用)。
func (n *Node) BaseURL() string { return n.baseURL }

// Stop 注销注册并关闭 HTTP 服务。
func (n *Node) Stop() {
	if n.svc != nil {
		n.svc.Stop()
	}
	if n.ln != nil {
		n.ln.Close()
	}
}

type capMeta struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

// capabilityMetas 汇总 node 声明的能力(以插件实现为准,fallback 配置里的 schema)。
func capabilityMetas(cfg []CapConfig, caps map[string]Capability) []capMeta {
	out := []capMeta{}
	seen := map[string]bool{}
	for _, c := range cfg {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		schema := c.Schema
		if impl, ok := caps[c.Name]; ok && impl.Schema() != "" {
			schema = impl.Schema()
		}
		out = append(out, capMeta{Name: c.Name, Schema: schema})
	}
	return out
}
