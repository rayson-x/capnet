# AGENTS.md — 给 coding agent 的约定

## 这是什么

cap-node 是一个"能力共享网络":每台机器跑一个 node,注册本机能力(如 STT/OCR/video_gen),任意 CLI/agent 发现并调用。服务器只做发现,调用直连 node。Go + NATS。

## 代码布局

```
cmd/capnode/            cap-node 二进制
  main.go               装配:读配置表 + 注册内置能力
  capability.go         核心:Capability 接口 + node.Register(注册/消费)
  command.go            CommandCapability:配置表驱动,执行 shell/docker 命令
  funcadapter.go        FuncCapability:Go 函数实现接口
cmd/cap/main.go         cap CLI(nodes / find / call / submit / wait / pub)
configs/capabilities.yaml  能力配置表(加条目 = 加能力)
deploy/                 hub.conf / leaf.conf / Caddyfile / (join.sh 待写)
docs/design.md          设计文档(v0.5)
```

## 加一个新能力

```bash
# 方式 1(默认,零代码):configs/capabilities.yaml 加一条
- name: ocr
  version: "1.0.0"
  schema: '{"type":"object","properties":{"image":{"type":"string"}},"required":["image"]}'
  needs_gpu: true
  exec: "echo fake-ocr:[{image}]"   # {field} 由 payload[field] 替换

# 方式 2(代码):实现 Capability 接口(cmd/capnode/capability.go)后 n.Register(ctx, cap)
```

## Capability 接口契约

```go
type Capability interface {
    Name() string
    Version() string
    Schema() string // JSON Schema 字符串(供调用方校验 / LLM tool-calling)
    NeedsGPU() bool
    Execute(ctx context.Context, payload map[string]any) (map[string]any, error)
    // 返回 error        → 瞬时失败,Nak 重试(按 BackOff)
    // 返回 TerminalError → 永久失败,Term 不重试
}
```

## 构建 / 测试 / 运行

见 `CLAUDE.md` 快速命令。本地跑通的最小链路:

```bash
nats-server -c deploy/hub.conf     # hub(开 JetStream)
nats-server -c deploy/leaf.conf    # 本地 leaf(拨 hub,若 hub 在别处改 remotes)
CONFIG=configs/capabilities.yaml ./capnode
./cap nodes && ./cap call hello '{"who":"world"}'
```

`nats-server` 需要自己装(brew install nats-server 或从 GitHub releases 下载)。

## 约定

- 消息格式(任务/结果/progress)以 `docs/design.md` §3.4 / §0.2 为准,改协议先改设计文档。
- nats.go API 是 v1.39(注意 ctx 签名差异,见 CLAUDE.md 坑)。
- 不要在 daemon 轮询里做阻塞投递(保持"先释放槽位再发结果")。
- 新增跨平台能力时同步更新交叉编译清单(CLAUDE.md)。
