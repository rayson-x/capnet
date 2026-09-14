// Capability 是能力的核心抽象。注册机制只依赖这个接口:
// 任何实现了它的东西(配置驱动的命令执行器、你的自定义 Go handler)都能注册进全网并消费任务。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
)

// Capability —— provider 只需实现这 4 个方法 + Execute
type Capability interface {
	Name() string
	Version() string
	Schema() string // JSON Schema 字符串,用于调用方校验/LLM tool-calling
	NeedsGPU() bool
	// Execute 执行一个任务,返回结构化结果。
	//   返回 error          → 瞬时失败,cap-node 会 Nak 并按 BackOff 重试
	//   返回 TerminalError  → 永久失败,cap-node 直接 Term(不重试)
	Execute(ctx context.Context, payload map[string]any) (map[string]any, error)
}

// TerminalError 包装永久失败:返回它则不再重试。
type TerminalError struct{ Err error }

func (e *TerminalError) Error() string { return e.Err.Error() }
func (e *TerminalError) Unwrap() error { return e.Err }
func Term(err error) *TerminalError    { return &TerminalError{Err: err} }

// task / result 与 CLI 约定的消息格式(设计文档 §3.4)
type task struct {
	TaskID        string         `json:"task_id"`
	Capability    string         `json:"capability"`
	Payload       map[string]any `json:"payload"`
	ResultSubject string         `json:"result_subject"`
}

// node 持有连接,负责"注册 + 消费"的机械部分
type node struct {
	nc *nats.Conn
	js jetstream.JetStream
	id string
}

// Register 对任意 Capability 做四件事:
//   ① micro 注册(可发现)  ② work-queue stream(任务入口)  ③ durable consumer(领取)  ④ 消费循环
func (n *node) Register(ctx context.Context, cap Capability) error {
	// ① 能力注册/发现:nats-micro
	svc, err := micro.AddService(n.nc, micro.Config{
		Name:    cap.Name(),
		Version: cap.Version(),
		Metadata: map[string]string{
			"node_id": n.id,
			"schema":  cap.Schema(),
			"gpu":     strconv.FormatBool(cap.NeedsGPU()),
		},
	})
	if err != nil {
		return err
	}
	// ② 任务入口:work-queue stream(持久,重启不丢)
	stream, err := n.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "CAP_" + strings.ToUpper(cap.Name()),
		Subjects:  []string{"cap." + cap.Name() + ".task"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		svc.Stop()
		return err
	}
	// ③ 领取通道:durable pull consumer(所有 node 同名拉取 = 自动负载均衡)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       cap.Name(),
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       5 * time.Minute,
		MaxDeliver:    3,
		BackOff:       []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute},
		DeliverPolicy: jetstream.DeliverAllPolicy, // workqueue 要求 DeliverAll
	})
	if err != nil {
		svc.Stop()
		return err
	}
	log.Printf("registered capability [%s] v%s", cap.Name(), cap.Version())
	// ④ 消费循环
	go n.consume(ctx, cap, cons)
	return nil
}

func (n *node) consume(ctx context.Context, cap Capability, cons jetstream.Consumer) {
	for {
		msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(15*time.Second))
		if err != nil {
			continue
		}
		for m := range msgs.Messages() {
			n.handle(ctx, cap, m)
		}
	}
}

func (n *node) handle(ctx context.Context, cap Capability, m jetstream.Msg) {
	var t task
	if err := json.Unmarshal(m.Data(), &t); err != nil {
		log.Printf("[%s] bad task, Term: %v", cap.Name(), err)
		m.Term()
		return
	}
	res, err := cap.Execute(ctx, t.Payload)

	status, output, errMsg := "completed", any(res), ""
	if err != nil {
		status, output, errMsg = "failed", nil, err.Error()
		var term *TerminalError
		if errors.As(err, &term) {
			m.Term() // 永久失败:不重试
		} else {
			m.Nak() // 瞬时失败:BackOff 后重投
		}
	} else {
		m.Ack()
	}

	result := map[string]any{
		"task_id":    t.TaskID,
		"capability": cap.Name(),
		"status":     status,
		"output":     output,
		"error":      errMsg,
		"meta":       map[string]any{"node": n.id, "ts": time.Now().Unix()},
	}
	subj := t.ResultSubject
	if subj == "" {
		subj = "cap." + cap.Name() + ".result." + t.TaskID
	}
	out, _ := json.Marshal(result)
	if err := n.nc.Publish(subj, out); err != nil {
		log.Printf("[%s] publish result: %v", cap.Name(), err)
	} else {
		log.Printf("[%s] %s task %s", cap.Name(), status, t.TaskID)
	}
}
