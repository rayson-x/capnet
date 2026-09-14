package cap

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// registryEntry 是 hub 注册表里的一条记录。
type registryEntry struct {
	NodeID       string    `json:"node_id"`
	BaseURL      string    `json:"base_url"`
	Capabilities []capMeta `json:"capabilities"`
	LastSeen     time.Time `json:"-"`
}

// Registry 是 hub 侧动态注册表:node 注册 + 心跳续租,TTL 未续则过期剔除。
// 只做发现,不代理流量。
type Registry struct {
	nc      *nats.Conn
	ttl     time.Duration
	sweep   time.Duration
	mu      sync.Mutex
	entries map[string]*registryEntry
}

// RunRegistry 启动注册表服务(hub 侧,连 nats-server)。
func RunRegistry(nc *nats.Conn, ttl, sweep time.Duration) (*Registry, error) {
	r := &Registry{nc: nc, ttl: ttl, sweep: sweep, entries: map[string]*registryEntry{}}

	// 注册 / 更新能力(回 ack,node 等确认注册完成)
	if _, err := nc.Subscribe("cap.reg.set", func(m *nats.Msg) {
		var e registryEntry
		if err := json.Unmarshal(m.Data, &e); err != nil {
			log.Printf("registry: bad set: %v", err)
			return
		}
		e.LastSeen = time.Now()
		r.mu.Lock()
		r.entries[e.NodeID] = &e
		r.mu.Unlock()
		if m.Reply != "" {
			_ = nc.Publish(m.Reply, []byte("ok"))
		}
	}); err != nil {
		return nil, err
	}

	// 心跳续租(兼容:node 每次心跳也会重发 cap.reg.set,set 自带续租)
	if _, err := nc.Subscribe("cap.reg.hb.>", func(m *nats.Msg) {
		id := m.Subject[len("cap.reg.hb."):]
		r.mu.Lock()
		if e, ok := r.entries[id]; ok {
			e.LastSeen = time.Now()
		}
		r.mu.Unlock()
	}); err != nil {
		return nil, err
	}

	// 发现:返回当前(未过期)条目
	if _, err := nc.Subscribe("cap.reg.list", func(m *nats.Msg) {
		r.mu.Lock()
		out := make([]registryEntry, 0, len(r.entries))
		for _, e := range r.entries {
			out = append(out, *e)
		}
		r.mu.Unlock()
		b, _ := json.Marshal(out)
		_ = nc.Publish(m.Reply, b)
	}); err != nil {
		return nil, err
	}

	// 定时剔除过期条目
	go func() {
		ticker := time.NewTicker(r.sweep)
		defer ticker.Stop()
		for range ticker.C {
			r.expire()
		}
	}()
	return r, nil
}

func (r *Registry) expire() {
	cutoff := time.Now().Add(-r.ttl)
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, e := range r.entries {
		if e.LastSeen.Before(cutoff) {
			log.Printf("registry: expiring node %s (idle %s)", id, time.Since(e.LastSeen).Round(time.Second))
			delete(r.entries, id)
		}
	}
}
