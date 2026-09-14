# capnet — 能力共享网络

让多台机器(NAT 内的 RTX 4070、云 VM、公网 server)把空闲算力注册成**能力**,任意 CLI/agent 发现并直连调用。Go 实现,基于 NATS JetStream + nats-micro。

```
[CLI/agent]──发现($SRV.PING)──▶ [服务器 hub](只做发现)
    └──直连 node──▶ cap.<cap>.rpc ──▶ [4070/node] 执行 → 结果直回
```

## 快速开始

```bash
# 本地跑通最小链路
nats-server -c deploy/hub.conf              # hub(JetStream)
nats-server -c deploy/leaf.conf             # NAT 内机器 leaf(拨 hub)
CONFIG=configs/capabilities.yaml go run ./cmd/capnode   # 一个 node
go run ./cmd/cap nodes                      # 发现能力
go run ./cmd/cap call hello '{"who":"world"}'           # 调用
```

## 文档

- [`docs/design.md`](docs/design.md) — 架构设计(v0.5:发现走服务器、调用直连 node;Tailscale underlay;流式/SSE/媒体分层)
- [`CLAUDE.md`](CLAUDE.md) — Claude Code 项目说明(构建/测试/坑)
- [`AGENTS.md`](AGENTS.md) — coding agent 约定(加能力/接口契约)
- [`context.md`](context.md) — 决策记录与现状

## 扩展能力

- **配置表**(默认):`configs/capabilities.yaml` 加一条 `{name, version, schema, needs_gpu, exec}`,零代码。
- **代码接口**:实现 `Capability` 接口(`cmd/capnode/capability.go`)注册。

## 状态

v0.3 PoC 已实测跑通(cloud hub + 本地 leaf 全链路)。v0.5 直连模型 + Tailscale underlay + 一键入网脚本待实现。详见 `context.md`。
