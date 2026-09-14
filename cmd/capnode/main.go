// cap-node:能力注册节点。
// 扩展能力的两种方式(都通过 Capability 接口注册):
//   1) 配置表扩展(默认):往 capabilities.yaml 加条目 → CommandCapability
//   2) 代码扩展:在 main 里实现/注册你自己的 Capability 接口
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"gopkg.in/yaml.v3"
)

// ---- 配置表(默认扩展路径) ----
type nodeConfig struct {
	NodeID       string       `yaml:"node_id"`
	NodeLabel    string       `yaml:"node_label"`
	Capabilities []capConfig  `yaml:"capabilities"`
}

type capConfig struct {
	Name     string `yaml:"name"`
	Version  string `yaml:"version"`
	Schema   string `yaml:"schema"`
	NeedsGPU bool   `yaml:"needs_gpu"`
	Exec     string `yaml:"exec"` // 命令模板,{field} 由 payload[field] 填充
}

func loadConfig(path string) nodeConfig {
	var cfg nodeConfig
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read config: %v", err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}
	if cfg.NodeID == "" {
		cfg.NodeID = "node-" + hostname()
	}
	return cfg
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func main() {
	nc, err := nats.Connect(env("NATS_URL", "nats://127.0.0.1:4222"))
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	cfg := loadConfig(env("CONFIG", "capabilities.yaml"))
	n := &node{nc: nc, js: js, id: cfg.NodeID}
	ctx := context.Background()

	// 1) 默认路径:配置表 → CommandCapability(零代码扩展)
	for _, cc := range cfg.Capabilities {
		if cc.Exec == "" {
			log.Printf("skip %s: no exec", cc.Name)
			continue
		}
		if err := n.Register(ctx, &CommandCapability{
			name: cc.Name, version: cc.Version, schema: cc.Schema, needsG: cc.NeedsGPU, command: cc.Exec,
		}); err != nil {
			log.Printf("register %s failed: %v", cc.Name, err)
		}
	}

	// 2) 代码扩展路径:实现 Capability 接口直接注册(自定义逻辑示例,与配置表的 hello 区分开)
	if err := n.Register(ctx, &FuncCapability{
		name:    "hello_func",
		version: "1.0.0",
		schema:  `{"type":"object","properties":{"who":{"type":"string"}}}`,
		fn: func(ctx context.Context, p map[string]any) (map[string]any, error) {
			who, _ := p["who"].(string)
			return map[string]any{"greeting": "hello, " + who}, nil
		},
	}); err != nil {
		log.Printf("register hello_func failed: %v", err)
	}

	log.Printf("node [%s] ready", cfg.NodeID)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
