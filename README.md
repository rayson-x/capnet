# capnet

用自家机器替代云 STT/OCR/视频 API。一个 `cap` 二进制:启动**可扩展 node 外壳**,把机器上的能力(模型服务)注册到 hub,agent/代码**动态发现并直连调用**。

```
[你的代码/agent]
   │ cap discover stt → 活的 node + base_url
   ▼
[hub 注册表](nats-micro)── 动态发现 / 心跳 15s / TTL 60s 剔除
   ▲
[可扩展 node = cap 外壳]
   ├─ 插件:进程内 Go 实现能力(实现 Capability 接口)
   └─ 转发:反向代理本机已有服务(faster-whisper / PaddleOCR / ComfyUI)
   │        ▲ Tailscale 直连
[模型实现不归 capnet 管]
```

## 设计文档

- `docs/adr/0001-self-hosted-model-api.md` — 收敛决策(自托管模型 API + 动态注册表 + 可扩展 node)
- `CONTEXT.md` — 术语表
- `CLAUDE.md` / `AGENTS.md` — 构建与约定

## 状态

设计定稿(ADR-0001),旧模型代码已清除;**实现待 spec 后从零搭建**。详见 `CONTEXT.md` 与 CLAUDE.md。
