// Package cap 实现 capnet 核心:可扩展 node + 动态注册表发现 + 直连调用。
package cap

import "net/http"

// Capability 是能力接口。插件在 node 进程内实现它,node 以 HTTP 端点暴露。
// 请求直接进代码,不经转发。
type Capability interface {
	Name() string
	// Schema 返回 JSON Schema(OpenAI 兼容工具形状),供调用方校验/构造调用。
	Schema() string
	// HandleHTTP 处理对该能力的调用(POST /cap/<name>,body 为任务 JSON)。
	HandleHTTP(w http.ResponseWriter, r *http.Request)
}

// capMeta 是注册记录里单个能力的元数据(name + JSON Schema)。
type capMeta struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}
