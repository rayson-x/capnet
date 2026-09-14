# capnet

让 agent 和代码用自家机器替代云 STT/OCR/视频 API:机器上的能力被一个 node 外壳暴露、注册到 hub,按能力动态发现并直连调用。

## Language

**Node**:
一台机器上运行 `cap node start` 的进程,负责暴露能力、注册到 hub、心跳。
_Avoid_: worker, agent

**Capability**:
node 对外提供的某个服务(如 `stt`、`ocr`、`video_gen`),由插件或转发两种方式提供。
_Avoid_: tool(MCP 语境词,本上下文不用)

**Plugin**:
在 node 进程内用 Go 实现的能力(实现 `Capability` 接口),请求直接进代码、不经转发。
_Avoid_: 内置脚本

**Forward**:
node 把请求反向代理到本机已有的服务(如 faster-whisper server),capnet 只负责暴露与注册。
_Avoid_: proxy 能力

**Hub(注册表)**:
运行 nats-micro 的服务,收集各 node 注册的能力与心跳,供 `cap discover` 查询;负责死机剔除。
_Avoid_: broker(与传输层混淆), server

**Discover**:
`cap discover <cap>`:查询 hub,返回当前活着的、提供该能力的 node 列表与 base_url。

**Heartbeat**:
node 每 15s 向 hub 续租的存活信号;TTL 60s 未续则从发现中剔除。

**Self-hosted model API**:
目标本身——现有调用云 API(Whisper/OCR)的代码改指自家机器即可,接口保持 OpenAI 兼容形状。

**Tailscale**:
连通层;给 NAT 内 node 可拨地址,agent/代码从任何地方直连。

_Avoid_(旧框架废弃词):
- capability network / 能力共享网络(旧目标,已收敛为"自托管模型 API")
- work-queue / task orchestration / 多 GPU 编排(明确不做)
- MCP(不是调用主干;仅未来可选适配)
