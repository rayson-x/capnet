# 0001:自托管模型 API + 动态注册表,node 只做可扩展外壳

**Status**: accepted

capnet 收敛为"用自家机器替代云 STT/OCR/视频 API":一个 `cap` 二进制,`cap node start` 启动可扩展 node 外壳,`cap discover/call/nodes` 做 CLI;node 通过**插件**(进程内 Go 实现能力)或**转发**(代理本机已有模型 server)两种方式提供能力,注册到 nats-micro hub 注册表(心跳 15s / TTL 60s 动态剔除),agent/代码经 Tailscale 直连 node 的 HTTP API 调用。**模型实现本身不归 capnet 管。**

## 为什么

用户需求经多轮澄清收敛:不是"能力共享网络",不是"MCP 工具协议",不是"多 GPU 编排"——而是**把平时调云 API(STT/OCR/视频)换成调自己机器**,且要求动态发现(家里机器会关机/死机、新机器会加入)。capnet 的自研价值只剩一层薄动态注册表 + 可扩展 node 外壳,其余(faster-whisper/PaddleOCR/Tailscale)都是成熟件。

## Considered Options

- **MCP-first(每 node 一个 mcp-go server + 网关)**:调研确认 MCP 是 2026 年 agent↔tool 调用的事实标准(Claude Code 原生 `claude mcp add --transport http`)。但用户明确**不要 MCP 作为调用主干**——要的是 agent 写代码把接口当业务层调用。MCP 降级为未来可选适配出口,不作主干。
- **NATS-native 能力协议(v0.2–v0.5:micro 注册 + JetStream work-queue + `cap.<cap>.rpc`)**:自研协议接不进 agent 生态,且把范围做大了(任务队列/多 GPU 调度不是需求)。**砍掉 work-queue/调度**,只保留 nats-micro 做动态发现注册表。
- **多 GPU 编排 / dstack 式调度**:用户明确不在需求内。

## Consequences

- **注册表只做发现,不代理流量**;调用直连 node HTTP API(OpenAI 兼容形状,流式由下层服务自带)。
- 动态发现(注册/心跳/剔除/新机加入)是 capnet 唯一自研价值,薄。
- **CLI 与 node 同一二进制**(`cap`),子命令区分角色。
- node 两种能力提供:**插件**(进程内代码,避免转发)与**转发**(代理本机已有服务)。
- 连通依赖 Tailscale(NAT 内 node 才有可拨地址),这是直连的前提。
