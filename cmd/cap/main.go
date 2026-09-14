// cap CLI: 发现能力(cap nodes)+ 调用能力(cap call) —— 连接本地 leaf,经云 hub 触达全网
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

const pingReq = `{"type":"io.nats.micro.v1.ping_request"}`

type pingResp struct {
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	ID       string            `json:"id"`
	Version  string            `json:"version"`
	Metadata map[string]string `json:"metadata"`
}

func main() {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222" // 本地 leaf
	}
	nc, err := nats.Connect(url)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	cmd := "nodes"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "nodes":
		discover(nc)
	case "call":
		if len(os.Args) < 4 {
			log.Fatal("usage: cap call <capability> '<json payload>'")
		}
		call(nc, os.Args[2], os.Args[3])
	case "pub":
		// 直接发到能力队列(不订阅结果),验证跨网投递
		if len(os.Args) < 4 {
			log.Fatal("usage: cap pub <capability> '<json payload>'")
		}
		nc.Publish("cap."+os.Args[2]+".task", []byte(os.Args[3]))
		fmt.Println("published to cap." + os.Args[2] + ".task")
	default:
		log.Fatalf("unknown command: %s (nodes|call|pub)", cmd)
	}
}

// cap nodes: 经 $SRV.PING 列出全网能力
func discover(nc *nats.Conn) {
	reply := nats.NewInbox()
	sub, err := nc.SubscribeSync(reply)
	if err != nil {
		log.Fatalf("sub: %v", err)
	}
	defer sub.Unsubscribe()
	if err := nc.PublishRequest("$SRV.PING", reply, []byte(pingReq)); err != nil {
		log.Fatalf("ping: %v", err)
	}
	type capRow struct {
		Capability string
		Nodes      []string
		Schema     string
	}
	seen := map[string]*capRow{}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		m, err := sub.NextMsg(200 * time.Millisecond)
		if err != nil {
			continue
		}
		var r pingResp
		if err := json.Unmarshal(m.Data, &r); err != nil {
			continue
		}
		cr := seen[r.Name]
		if cr == nil {
			cr = &capRow{Capability: r.Name, Schema: r.Metadata["schema"]}
			seen[r.Name] = cr
		}
		cr.Nodes = append(cr.Nodes, r.Metadata["node_id"]+"("+r.Metadata["node_label"]+")")
	}
	out := make([]*capRow, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

// cap call: 发布任务到能力队列 + 订阅结果,阻塞返回
func call(nc *nats.Conn, capName, payload string) {
	taskID := fmt.Sprintf("t-%d", time.Now().UnixNano())
	resSubj := "cap." + capName + ".result." + taskID
	sub, err := nc.SubscribeSync(resSubj)
	if err != nil {
		log.Fatalf("sub result: %v", err)
	}
	defer sub.Unsubscribe()

	task := map[string]any{
		"task_id":        taskID,
		"capability":     capName,
		"payload":        json.RawMessage(payload),
		"result_subject": resSubj,
	}
	b, _ := json.Marshal(task)
	subj := "cap." + capName + ".task"
	if err := nc.Publish(subj, b); err != nil {
		log.Fatalf("publish: %v", err)
	}
	fmt.Printf("submitted %s -> %s (waiting result on %s)\n", taskID, subj, resSubj)

	m, err := sub.NextMsg(20 * time.Second)
	if err != nil {
		log.Fatalf("no result in 20s: %v", err)
	}
	var res map[string]any
	json.Unmarshal(m.Data, &res)
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
}
