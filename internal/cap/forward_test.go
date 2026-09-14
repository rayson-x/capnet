package cap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeModel 模拟本机已有的模型服务(faster-whisper 之类)。
type fakeModel struct {
	srv *httptest.Server
}

func newFakeModel(t *testing.T) *fakeModel {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"text":"transcribed:%s"}`, body)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// 流式端点(SSE):边写边 flush
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: chunk%d\n\n", i)
			if fl != nil {
				fl.Flush()
			}
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &fakeModel{srv: srv}
}

// Close 关闭假服务(测试中模拟"本地模型挂了")。
func (f *fakeModel) Close() { f.srv.Close() }

func TestSystem_ForwardAndProbe(t *testing.T) {
	hub := startHub(t)
	fake := newFakeModel(t)

	// node 用 forward 转发到假模型服务
	cfg := Config{
		NodeID: "test-forward",
		Listen: "127.0.0.1:0",
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

	// 发现:forward 能力被注册
	infos, err := Discover(hub, "stt", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 node with stt, got %d: %+v", len(infos), infos)
	}

	// 直连调用:请求经 node 反向代理到假模型,拿到代理后的结果
	body, err := Call(context.Background(), infos[0], []byte("audio-bytes"))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("bad response %q: %v", body, err)
	}
	if out["text"] != "transcribed:audio-bytes" {
		t.Errorf("want transcribed:audio-bytes, got %q", out["text"])
	}
}

func TestSystem_Forward_ProbeFail_NotRegistered(t *testing.T) {
	hub := startHub(t)

	// local 指向一个不存在的服务 → 探活失败 → 不注册
	cfg := Config{
		NodeID: "test-dead",
		Listen: "127.0.0.1:0",
		Capabilities: []CapConfig{{
			Name: "stt", Kind: "forward",
			Local: "http://127.0.0.1:1/v1/audio/transcriptions", // 必然连不上
			Probe: "/health",
		}},
	}
	node, err := StartNode(hub, cfg, nil)
	if err != nil {
		t.Fatalf("start node: %v", err)
	}
	defer node.Stop()

	infos, err := Discover(hub, "stt", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("dead forward capability should NOT register, got %+v", infos)
	}
}

func TestSystem_Forward_StreamingPassThrough(t *testing.T) {
	hub := startHub(t)
	fake := newFakeModel(t)

	cfg := Config{
		NodeID: "test-stream",
		Listen: "127.0.0.1:0",
		Capabilities: []CapConfig{{
			Name: "stream", Kind: "forward",
			Local: fake.srv.URL + "/stream",
		}},
	}
	node, err := StartNode(hub, cfg, nil)
	if err != nil {
		t.Fatalf("start node: %v", err)
	}
	defer node.Stop()

	infos, err := Discover(hub, "stream", 2*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1 stream node, got %d", len(infos))
	}

	// 直连调用,读流式响应:应收到 3 个 chunk(逐块到达)
	url := infos[0].BaseURL + "/cap/stream"
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(""))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream call: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("want text/event-stream, got %q", resp.Header.Get("Content-Type"))
	}
	all, _ := io.ReadAll(resp.Body)
	got := string(all)
	for i := 0; i < 3; i++ {
		if !strings.Contains(got, fmt.Sprintf("chunk%d", i)) {
			t.Errorf("stream missing chunk%d: %q", i, got)
		}
	}
}
