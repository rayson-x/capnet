# 02 — 转发能力 + 探活 + 流式透传

**What to build:** Node 支持 `kind: forward` 的能力:把请求反向代理到本机已有的服务(如 faster-whisper / PaddleOCR),capnet 只负责暴露、探活与注册。本地服务探活失败则不注册该能力;转发时 SSE / 分块流式透传、不缓冲。用假模型服务桩验证。

**Blocked by:** 01 (最小垂直闭环)

**Status:** resolved

## Answer

2026-09-15 完成。

- node 支持 `kind: forward`:反向代理到 `local`(精确改写为目标完整 URL,而非拼接路径)。
- 探活(`probe` 2xx)通过才注册;失败跳过/剔除。
- 流式(SSE/分块)经 ReverseProxy 透传,不缓冲。
- 系统测试:`TestSystem_ForwardAndProbe`(代理结果)、`TestSystem_Forward_ProbeFail_NotRegistered`(探活失败不注册)、`TestSystem_Forward_StreamingPassThrough`(3 chunk 透传)。全部通过。
