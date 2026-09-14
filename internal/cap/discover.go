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

// Discover 查询 hub 注册表(cap.reg.list),返回当前(未过期)提供该能力的 node。
// capability 为空时返回全部。
func Discover(nc *nats.Conn, capability string, timeout time.Duration) ([]NodeInfo, error) {
	resp, err := nc.Request("cap.reg.list", []byte("{}"), timeout)
	if err != nil {
		return nil, err
	}
	var entries []registryEntry
	if err := json.Unmarshal(resp.Data, &entries); err != nil {
		return nil, err
	}
	var out []NodeInfo
	for _, e := range entries {
		for _, c := range e.Capabilities {
			if capability == "" || c.Name == capability {
				out = append(out, NodeInfo{
					NodeID:     e.NodeID,
					BaseURL:    e.BaseURL,
					Capability: c.Name,
					Schema:     c.Schema,
				})
			}
		}
	}
	return out, nil
}
