// cap:capnet 单二进制 —— 子命令区分角色。
//
//	node start     启动 node 外壳(读 capabilities.yaml,注册到 hub,暴露能力 HTTP)
//	discover <cap> 查 hub 注册表,返回活的 node + base_url + schema
//	call <cap> '<json>'  直连 node 调用能力,打印结果
//	nodes          列出全网 node 与能力
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"capnet/internal/cap"

	"github.com/nats-io/nats.go"
)

func hubURL() string {
	if v := os.Getenv("CAP_HUB_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:4222"
}

func connect() *nats.Conn {
	nc, err := nats.Connect(hubURL())
	if err != nil {
		log.Fatalf("connect hub %s: %v", hubURL(), err)
	}
	return nc
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "hub":
		hubCmd()
	case "node":
		nodeCmd(os.Args[2:])
	case "discover":
		discoverCmd(os.Args[2:])
	case "call":
		callCmd(os.Args[2:])
	case "nodes":
		nodesCmd(nil)
	default:
		usage()
		os.Exit(1)
	}
}

// hubCmd 在 hub 上跑动态注册表(心跳 TTL 60s 剔除)。
func hubCmd() {
	nc := connect()
	defer nc.Close()
	if _, err := cap.RunRegistry(nc, 60*time.Second, 5*time.Second); err != nil {
		log.Fatalf("run registry: %v", err)
	}
	log.Printf("hub registry running on %s (ttl 60s)", hubURL())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

func usage() {
	fmt.Println(`usage: cap <command>
  node start           启动 node 外壳(读 capabilities.yaml)
  discover <capability> 查 hub,返回活的 node + base_url + schema
  call <capability> '<json>'  直连调用能力
  nodes                列出全网 node 与能力
env: CAP_HUB_URL(默认 nats://127.0.0.1:4222)`)
}

func nodeCmd(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: cap node start | status")
	}
	switch args[0] {
	case "start":
		nodeStart()
	case "status":
		nodeStatus()
	default:
		log.Fatal("usage: cap node start | status")
	}
}

func nodeStart() {
	configPath := "capabilities.yaml"
	if v := os.Getenv("CAP_CONFIG"); v != "" {
		configPath = v
	}
	cfg, err := cap.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 插件:从内置工厂按 name 实例化
	caps := map[string]cap.Capability{}
	for _, c := range cfg.Capabilities {
		if c.Kind != "plugin" {
			continue // forward 见 ticket 02
		}
		factory, ok := cap.BuiltinPlugins()[c.Name]
		if !ok {
			log.Printf("no plugin for %q (kind=plugin); skipping", c.Name)
			continue
		}
		caps[c.Name] = factory()
	}

	nc := connect()
	defer nc.Close()
	node, err := cap.StartNode(nc, cfg, caps)
	if err != nil {
		log.Fatalf("start node: %v", err)
	}
	defer node.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

func nodeStatus() {
	configPath := "capabilities.yaml"
	if v := os.Getenv("CAP_CONFIG"); v != "" {
		configPath = v
	}
	cfg, err := cap.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	nc := connect()
	defer nc.Close()
	infos, err := cap.Discover(nc, "", 2*time.Second)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	var mine []cap.NodeInfo
	for _, i := range infos {
		if i.NodeID == cfg.NodeID {
			mine = append(mine, i)
		}
	}
	if len(mine) == 0 {
		fmt.Printf("node [%s] not registered on hub\n", cfg.NodeID)
		return
	}
	b, _ := json.MarshalIndent(mine, "", "  ")
	fmt.Println(string(b))
}

func discoverCmd(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: cap discover <capability>")
	}
	nc := connect()
	defer nc.Close()
	infos, err := cap.Discover(nc, args[0], 2*time.Second)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	b, _ := json.MarshalIndent(infos, "", "  ")
	fmt.Println(string(b))
}

func callCmd(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: cap call <capability> '<json>'")
	}
	nc := connect()
	defer nc.Close()
	infos, err := cap.Discover(nc, args[0], 2*time.Second)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	if len(infos) == 0 {
		log.Fatalf("no live node provides %q", args[0])
	}
	// 按能力 schema 校验入参,非法给出清晰错误
	if err := cap.ValidateAgainstSchema(infos[0].Schema, []byte(args[1])); err != nil {
		log.Fatalf("validate: %v", err)
	}
	body, err := cap.Call(context.Background(), infos[0], []byte(args[1]))
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	fmt.Println(string(body))
}

func nodesCmd(_ []string) {
	nc := connect()
	defer nc.Close()
	infos, err := cap.Discover(nc, "", 2*time.Second)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	b, _ := json.MarshalIndent(infos, "", "  ")
	fmt.Println(string(b))
}
