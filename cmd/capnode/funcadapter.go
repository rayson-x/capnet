// FuncCapability:代码扩展路径 —— 用 Go 函数实现 Capability 接口。
// 当"一条命令搞不定"、需要自定义逻辑(调库、流式、状态机)时用它。
package main

import "context"

type CapabilityFunc func(ctx context.Context, payload map[string]any) (map[string]any, error)

type FuncCapability struct {
	name    string
	version string
	schema  string
	needsG  bool
	fn      CapabilityFunc
}

func (c *FuncCapability) Name() string    { return c.name }
func (c *FuncCapability) Version() string { return c.version }
func (c *FuncCapability) Schema() string  { return c.schema }
func (c *FuncCapability) NeedsGPU() bool  { return c.needsG }
func (c *FuncCapability) Execute(ctx context.Context, p map[string]any) (map[string]any, error) {
	return c.fn(ctx, p)
}
