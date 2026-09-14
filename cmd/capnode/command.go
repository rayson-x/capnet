// CommandCapability:配置表驱动的能力实现。
// 把"能力"映射成一条 shell/docker 命令,零 Go 代码即可注册新能力(默认扩展路径)。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommandCapability 实现 Capability 接口
type CommandCapability struct {
	name    string
	version string
	schema  string
	needsG  bool
	command string // 命令模板,{field} 会被 payload[field] 替换
}

func (c *CommandCapability) Name() string       { return c.name }
func (c *CommandCapability) Version() string    { return c.version }
func (c *CommandCapability) Schema() string     { return c.schema }
func (c *CommandCapability) NeedsGPU() bool     { return c.needsG }

func (c *CommandCapability) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	cmd := c.command
	for k, v := range payload {
		cmd = strings.ReplaceAll(cmd, "{"+k+"}", fmt.Sprint(v))
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(cctx, "sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, err
	}
	// stdout 若为合法 JSON 则原样返回,否则包成 {output: string}
	var parsed map[string]any
	if json.Unmarshal(out, &parsed) == nil {
		return parsed, nil
	}
	return map[string]any{"output": string(out)}, nil
}
