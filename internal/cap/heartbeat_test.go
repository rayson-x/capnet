package cap

import (
	"testing"
	"time"
)

// T3:停掉心跳后,TTL 内该 node 从发现消失(死机自动剔除)。
func TestSystem_HeartbeatExpiry(t *testing.T) {
	hub := startHubTTL(t, 2*time.Second)

	cfg := Config{
		NodeID:            "hb-node",
		Listen:            "127.0.0.1:0",
		HeartbeatInterval: 1, // 1s 心跳,快于 TTL 2s
		Capabilities:      []CapConfig{{Name: "hello", Kind: "plugin"}},
	}
	node, err := StartNode(hub, cfg, map[string]Capability{"hello": &helloCap{}})
	if err != nil {
		t.Fatalf("start node: %v", err)
	}

	// 活着时:发现能看到
	infos, err := Discover(hub, "hello", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 live node, got %d", len(infos))
	}

	// 停止 node(心跳停,不主动注销)→ 等 TTL + sweep 余量
	node.Stop()
	time.Sleep(4 * time.Second)

	infos, err = Discover(hub, "hello", 2*time.Second)
	if err != nil {
		t.Fatalf("discover after expiry: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("expired node should be gone, got %+v", infos)
	}
}

// T3:新 node 启动后自动出现在发现里。
func TestSystem_NewNodeAppears(t *testing.T) {
	hub := startHub(t)

	cfgA := Config{NodeID: "node-a", Listen: "127.0.0.1:0",
		Capabilities: []CapConfig{{Name: "hello", Kind: "plugin"}}}
	nodeA, err := StartNode(hub, cfgA, map[string]Capability{"hello": &helloCap{}})
	if err != nil {
		t.Fatalf("start node A: %v", err)
	}
	defer nodeA.Stop()

	// 后启动 node B
	cfgB := Config{NodeID: "node-b", Listen: "127.0.0.1:0",
		Capabilities: []CapConfig{{Name: "hello", Kind: "plugin"}}}
	nodeB, err := StartNode(hub, cfgB, map[string]Capability{"hello": &helloCap{}})
	if err != nil {
		t.Fatalf("start node B: %v", err)
	}
	defer nodeB.Stop()

	infos, err := Discover(hub, "hello", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	ids := map[string]bool{}
	for _, i := range infos {
		ids[i.NodeID] = true
	}
	if !ids["node-a"] || !ids["node-b"] {
		t.Fatalf("want node-a and node-b, got %+v", ids)
	}
}

// T3:转发能力健康变化时注册记录被更新(本地模型挂了 → 从发现消失)。
func TestSystem_ForwardHealthChange(t *testing.T) {
	hub := startHub(t)
	fake := newFakeModel(t)

	cfg := Config{
		NodeID:         "hf-node",
		Listen:         "127.0.0.1:0",
		HealthInterval: 1, // 1s 健康重查
		Capabilities: []CapConfig{{
			Name: "stt", Kind: "forward",
			Local: fake.srv.URL + "/v1/audio/transcriptions",
			Probe: "/health",
		}},
	}
	node, err := StartNode(hub, cfg, nil)
	if err != nil {
		t.Fatalf("start node: %v", err)
	}
	defer node.Stop()

	// 健康时:注册了 stt
	infos, err := Discover(hub, "stt", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want stt registered, got %d", len(infos))
	}

	// 本地模型挂了 → 健康重查后 stt 从发现消失(但 node 心跳仍在,能力剔除)
	fake.Close()
	time.Sleep(4 * time.Second) // > HealthInterval 1s + sweep 余量

	infos, err = Discover(hub, "stt", 2*time.Second)
	if err != nil {
		t.Fatalf("discover after health change: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("unhealthy stt should be gone, got %+v", infos)
	}
}
