# 02 — 转发能力 + 探活 + 流式透传

**What to build:** Node 支持 `kind: forward` 的能力:把请求反向代理到本机已有的服务(如 faster-whisper / PaddleOCR),capnet 只负责暴露、探活与注册。本地服务探活失败则不注册该能力;转发时 SSE / 分块流式透传、不缓冲。用假模型服务桩验证。

**Blocked by:** 01 (最小垂直闭环)

**Status:** ready-for-agent

- [ ] 配置支持 `kind: forward` + `local` 地址,node 反向代理到本地服务
- [ ] 探活(probe)通过才注册该能力;探活失败不注册/及时剔除
- [ ] 转发时 SSE / 分块流式透传,不缓冲
- [ ] 系统测试:假模型服务桩经 forward 被发现、被调用返回真实代理结果
