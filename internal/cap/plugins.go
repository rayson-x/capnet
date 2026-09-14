package cap

import (
	"encoding/json"
	"net/http"
)

// BuiltinPlugins 是内置插件工厂:配置里 kind: plugin 的能力按 name 在这里找实现。
// 这是"直接 coding 实现能力"的入口——加新插件 = 在这里加一个工厂 + 一个实现。
func BuiltinPlugins() map[string]func() Capability {
	return map[string]func() Capability{
		"hello": func() Capability { return &helloCap{} },
	}
}

// helloCap:最小验证能力,证明 插件(进程内,不经转发)链路。
type helloCap struct{}

func (h *helloCap) Name() string { return "hello" }
func (h *helloCap) Schema() string {
	return `{"type":"object","properties":{"who":{"type":"string"}},"required":["who"]}`
}
func (h *helloCap) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Who string `json:"who"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"greeting": "hello, " + in.Who})
}
