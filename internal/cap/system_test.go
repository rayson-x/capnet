package cap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// startHub 起一个内嵌真实 nats-server 作为 hub 注册表(hub = 纯发现,无代理)。
func startHub(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server not ready")
	}
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func TestSystem_MinimalLoop_PluginHello(t *testing.T) {
	hub := startHub(t)

	// 起一个 node:hello 插件能力
	cfg := Config{
		NodeID: "test-node",
		Listen: "127.0.0.1:0", // 随机端口
		Capabilities: []CapConfig{{
			Name: "hello", Kind: "plugin",
			Schema: `{"type":"object","properties":{"who":{"type":"string"}},"required":["who"]}`,
		}},
	}
	node, err := StartNode(hub, cfg, map[string]Capability{"hello": &helloCap{}})
	if err != nil {
		t.Fatalf("start node: %v", err)
	}
	defer node.Stop()

	// 发现:cap discover hello → 1 个活 node,带 base_url + schema
	infos, err := Discover(hub, "hello", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 node, got %d: %+v", len(infos), infos)
	}
	if infos[0].NodeID != "test-node" {
		t.Errorf("want node_id test-node, got %s", infos[0].NodeID)
	}
	if infos[0].BaseURL == "" {
		t.Error("base_url empty")
	}
	if infos[0].Schema == "" {
		t.Error("schema empty")
	}

	// 直连调用:cap call hello '{"who":"world"}' → {"greeting":"hello, world"}
	body, err := Call(context.Background(), infos[0], []byte(`{"who":"world"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("bad response %q: %v", body, err)
	}
	if out["greeting"] != "hello, world" {
		t.Errorf("want greeting=hello, world, got %q", out["greeting"])
	}
}
