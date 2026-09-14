# 03 — 心跳与动态剔除

**What to build:** Node 每 15s 向 hub 续租心跳;hub 注册表对超过 TTL(60s)未续租的 node 能力进行过期剔除。能力健康状态变化时更新注册。这样调用方实时看到的只有"现在还活着"的 node——死机/关机自动消失,新机器自动出现。

**Blocked by:** 01 (最小垂直闭环)

**Status:** resolved

## Answer

2026-09-15 完成。

- 新增 hub 注册表服务(`cap hub`,`internal/cap/registry.go`):`cap.reg.set`(注册,回 ack 确认)、`cap.reg.hb.<node>`(心跳续租)、`cap.reg.list`(发现),TTL 过期剔除(sweep)。
- Node 心跳(默认 15s,`HeartbeatInterval` 可配)+ 健康重查(默认 10s,转发能力变化时重新注册)。
- 发现从 `$SRV.PING`(nats-micro)迁移到 `cap.reg.list`——注册表才有 TTL/lease 语义,满足动态剔除。
- 系统测试:`TestSystem_HeartbeatExpiry`(停心跳→TTL 内消失)、`TestSystem_NewNodeAppears`(新 node→出现)、`TestSystem_ForwardHealthChange`(本地模型挂→能力剔除)。全部通过。
