# 03 — 心跳与动态剔除

**What to build:** Node 每 15s 向 hub 续租心跳;hub 注册表对超过 TTL(60s)未续租的 node 能力进行过期剔除。能力健康状态变化时更新注册。这样调用方实时看到的只有"现在还活着"的 node——死机/关机自动消失,新机器自动出现。

**Blocked by:** 01 (最小垂直闭环)

**Status:** ready-for-agent

- [ ] Node 心跳 15s;hub 注册表 TTL 60s 过期剔除
- [ ] 停掉心跳后,~60s 内该 node 从 `cap discover` 消失
- [ ] 新启动的 node 自动出现在 `cap discover`
- [ ] 能力健康变化时注册记录被更新
- [ ] 系统测试:心跳停 → 消失;新 node → 出现
