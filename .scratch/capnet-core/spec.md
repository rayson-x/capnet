## Problem Statement

用户平时做大量 STT、OCR、图像/视频理解、视频生成,正常需要调云 API(Whisper、OCR 云服务等)。用户希望用自己的机器(家里 RTX 4070、Mac、云服务器)直接实现同样的效果:现有代码/agent 像调云 API 一样调自己机器,接口形状保持 OpenAI 兼容。同时要求**动态发现**:家里机器会关机/死机、会有新机器/新能力加入,调用方要能实时知道"现在哪些机器活着、提供什么能力、怎么连"。

## Solution

一个 `cap` 单二进制,两种角色:

- **node 外壳**:`cap node start` 在每台机器上启动,把本机能力暴露成可发现的 HTTP 服务。能力由两种方式提供:
  - **插件**:进程内 Go 实现 `Capability` 接口,直接 coding,不经转发。
  - **转发**:反向代理本机已有的模型 server(如 faster-whisper/PaddleOCR),capnet 只暴露与注册。
- **CLI/客户端**:`cap discover <cap>` 查 hub 注册表,返回当前活着的、提供该能力的 node 列表(base_url + schema);`cap call` 直连调用;`cap nodes` 列全部。

hub 注册表用 nats-micro:node 注册 `{node_id, capability, base_url, schema}`,心跳 15s、TTL 60s,死机自动剔除、新机器自动加入。调用方经 Tailscale 直连 node 的 HTTP API(流式由下层服务/插件自带,node 透传)。**模型实现本身不归 capnet 管。**

## User Stories

1. As a user, I want to run `cap node start` on my 4070, so that its capabilities become discoverable.
2. As a user, I want to add a new capability by editing `capabilities.yaml`, so that existing model servers need no recompile.
3. As a developer, I want to implement a capability as a Go plugin implementing `Capability`, so that it runs in-process without forwarding.
4. As a user, I want my node to register STT by forwarding to a local faster-whisper server, so that capnet doesn't implement the model.
5. As an agent, I want to call `cap discover stt`, so that I can find currently-alive nodes that provide STT.
6. As an agent, I want the discovery result to include base_url and schema, so that I can call the node directly.
7. As a user, I want a dead node to disappear from discovery within ~60s, so that I don't call machines that are down.
8. As a user, I want a newly-started node to appear in discovery automatically, so that new machines work without config.
9. As an agent, I want to call a capability directly over HTTP (OpenAI-compatible shape), so that my existing code works by just changing base_url.
10. As a user, I want streaming (SSE / chunked) to pass through the node, so that STT/OCR/video can stream results.
11. As a user, I want a node whose local model server is down to not register that capability, so that discovery reflects reality.
12. As a user, I want `cap nodes` to list all nodes and capabilities, so that I can inspect the network.
13. As a user, I want the same binary for node and CLI, so that deployment is one artifact.
14. As a user, I want each capability exposed at a tailnet URL, so that it's reachable from anywhere including behind NAT.
15. As a user, I want probe failures to prevent registration, so that only healthy capabilities are advertised.
16. As a developer, I want the plugin capability to serve arbitrary HTTP (including streaming), so that in-process capabilities are as capable as forwarded ones.
17. As an agent, I want discovery to expose the JSON Schema, so that I can validate or construct calls.
18. As a user, I want the node to re-register (or update) if a capability's health changes, so that discovery stays accurate over time.
19. As a user, I want `cap node status` to show what my node registered, so that I can debug locally.
20. As an agent, I want the direct call to not be routed through the hub, so that the hub is discovery-only and not a bottleneck.

## Implementation Decisions

- **单二进制**:`cap`,子命令 `node start` / `discover` / `call` / `nodes` / `node status`。
- **Capability 接口**:`Name()`, `Schema()`(JSON Schema,OpenAI 兼容工具形状),插件对外是 HTTP handler(`ServeHTTP`)。
- **Node 配置**(`capabilities.yaml`):`{name, kind: plugin|forward, local(forward 用), probe, schema}`。
- **插件**:进程内 Go 实现 `Capability`,编译进 `cap`。
- **转发**:node 反向代理到 `local`;探活(`probe`)通过才注册;透传流式(SSE/WS/分块,不缓冲)。
- **Hub**:nats-micro 注册表服务。node 注册一个服务(name = node_id),能力列表放 metadata(含 base_url + schema)。
- **发现**:`cap discover <cap>` → `$SRV.PING`(**必须 PublishRequest 带 reply**)→ 过滤能力 → 返回活的 node `{base_url, schema}`。
- **心跳**:15s 间隔,TTL 60s,hub 过期剔除。
- **暴露**:每个能力一个 `<tailnet-ip>:<port>` URL;调用直连 HTTP,注册表不代理流量。
- **连通**:Tailscale(前提:NAT 内 node 才有可拨地址)。
- **注册记录**:`{node_id, capability, base_url, schema}`。

## Testing Decisions

- **单一 seam**(已确认):进程内系统集成测试——内嵌真实 nats-server + hub 注册表 + cap node + cap client 于一个 Go 测试进程,只断言**外部行为**。
- 覆盖:① discover 能看到已注册能力(带 base_url + schema)② 真实 HTTP 调用返回预期结果(plugin 与 forward 各一)③ 停止心跳 → TTL 内从发现消失 ④ 新 node → 自动出现 ⑤ 转发模式流式透传。
- 不测实现细节;不做更低层的次级 seam。

## Out of Scope

- 模型实现本身(faster-whisper / PaddleOCR / 视频理解 / 视频生成)—— node 层独立,不归 capnet。
- 多 GPU 编排 / 调度 / 任务队列。
- MCP 作为调用主干(仅未来可选适配)。
- Hub 代理流量(注册表只做发现)。
- 鉴权模型(v1 靠 Tailscale ACL;NATS token / 每 node JWT 后续)。

## Further Notes

- 术语以 `CONTEXT.md` 为准(Node / Capability / Plugin / Forward / Hub / Discover / Heartbeat)。
- 收敛决策见 `docs/adr/0001-self-hosted-model-api.md`。
- 连通依赖 Tailscale;直连调用是设计核心(注册表只发现,不中转)。
