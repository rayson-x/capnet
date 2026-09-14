# 01 — 最小垂直闭环:cap 骨架 + 测试 seam + hello 插件能力

**What to build:** 一个 `cap` 单二进制,子命令分发(node start / discover / call / nodes)。Node 读取能力配置,把一个 `hello` 插件能力(进程内 Go 实现)注册到 hub;`cap discover hello` 经 hub 返回该 node 的 base_url 与 schema;`cap call hello` 直连 node 的 HTTP 端点拿到结果。配套进程内系统测试 seam(内嵌真实 nats-server + hub 注册表 + node + client),只断言外部行为。

**Blocked by:** None — can start immediately.

**Status:** resolved

## Answer

2026-09-15 完成。实现 + 验证:

- `cap` 单二进制,子命令 `node start` / `discover` / `call` / `nodes`。
- `Capability` 接口 + `hello` 插件(`internal/cap/plugins.go` 内置工厂)。
- Node 注册到 hub(nats-micro,一个 node 一个服务,metadata 带 base_url + capabilities);HTTP 暴露 `POST /cap/<name>`。
- 发现经 `$SRV.PING`(PublishRequest),调用直连 node HTTP,注册表不代理。
- 进程内系统测试 seam(`internal/cap/system_test.go`,内嵌真实 nats-server)通过:discover 看到 hello + 直连调用返回 `hello, world`。
- 真实进程级 CLI 演示通过:`cap node start` → `cap discover hello` → `cap call hello` → `{"greeting":"hello, world"}`。

验收全部满足。
