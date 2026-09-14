# 01 — 最小垂直闭环:cap 骨架 + 测试 seam + hello 插件能力

**What to build:** 一个 `cap` 单二进制,子命令分发(node start / discover / call / nodes)。Node 读取能力配置,把一个 `hello` 插件能力(进程内 Go 实现)注册到 hub;`cap discover hello` 经 hub 返回该 node 的 base_url 与 schema;`cap call hello` 直连 node 的 HTTP 端点拿到结果。配套进程内系统测试 seam(内嵌真实 nats-server + hub 注册表 + node + client),只断言外部行为。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `cap` 单二进制,子命令 `node start` / `discover` / `call` / `nodes` 可分发
- [ ] Node 读能力配置,`hello` 插件能力实现 `Capability` 接口并在 node 内直接服务
- [ ] Node 启动后注册到 hub(nats-micro),注册记录含 base_url 与 schema
- [ ] `cap discover hello` 返回活的 node + base_url + schema
- [ ] `cap call hello` 直连 node 的 HTTP 端点返回预期结果(不经 hub 代理)
- [ ] 进程内系统测试 seam 就位:discover 看到 hello、直连调用返回结果
