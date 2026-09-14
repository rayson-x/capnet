package cap

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
)

// NodeInfo 是发现返回的一条记录:调用方据此直连 node。
type NodeInfo struct {
	NodeID     string `json:"node_id"`
	BaseURL    string `json:"base_url"`
	Capability string `json:"capability"`
	Schema     string `json:"schema"`
}

type pingResp struct {
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	ID       string            `json:"id"`
	Version  string            `json:"version"`
	Metadata map[string]string `json:"metadata"`
}

const pingRequest = `{"type":"io.nats.micro.v1.ping_request"}`

// Discover 查询 hub 注册表($SRV.PING),返回当前提供该能力的 node。
// capability 为空时返回全部。
func Discover(nc *nats.Conn, capability string, timeout time.Duration) ([]NodeInfo, error) {
	reply := nats.NewInbox()
	sub, err := nc.SubscribeSync(reply)
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	// 注意:必须用 PublishRequest(带 reply),纯 Publish 无回程。
	if err := nc.PublishRequest("$SRV.PING", reply, []byte(pingRequest)); err != nil {
		return nil, err
	}

	var out []NodeInfo
	seen := map[string]bool{}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m, err := sub.NextMsg(200 * time.Millisecond)
		if err != nil {
			continue
		}
		var r pingResp
		if err := json.Unmarshal(m.Data, &r); err != nil {
			continue
		}
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if r.Name == "" || r.Metadata["base_url"] == "" {
			continue
		}
		var metas []capMeta
		_ = json.Unmarshal([]byte(r.Metadata["capabilities"]), &metas)
		for _, c := range metas {
			if capability == "" || c.Name == capability {
				out = append(out, NodeInfo{
					NodeID:     r.Name,
					BaseURL:    r.Metadata["base_url"],
					Capability: c.Name,
					Schema:     c.Schema,
				})
			}
		}
	}
	return out, nil
}
