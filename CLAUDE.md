# capnet — 自托管模型 API + 动态注册表

一个 `cap` 二进制:启动可扩展 node 外壳,把机器能力注册到 hub,agent/代码动态发现并直连调用。Go + NATS。

## 设计(ADR-0001)

- **目标**:用自家机器替代云 STT/OCR/视频 API,代码改指自家 base_url 即可(OpenAI 兼容形状)。
- **node 外壳**:`cap node start`,两种能力提供:
  - **插件**:进程内 Go 实现 `Capability` 接口,请求直接进代码,不转发。
  - **转发**:反向代理本机已有服务(local 地址 → tailnet URL),capnet 只暴露/注册。
- **hub 注册表**:`cap hub` 服务(cap.reg.set/hb/list 主题),node 心跳重发 set,TTL 60s 过期剔除。
- **发现**:`cap discover <cap>` 请求 `cap.reg.list`,返回活的 node + base_url + schema(注册表只做发现,不代理流量)。
- **连通**:Tailscale;调用直连 node 的 HTTP API,注册表不代理流量。
- **不做**:多 GPU 编排/任务队列/MCP 主干(仅未来可选适配)。

## 快速命令(实现后)

```bash
go build -o cap ./cmd/cap          # 单二进制(node + CLI)
cap node start                      # 启动 node 外壳(读 capabilities.yaml)
cap discover stt                    # 查 hub:活的 node + base_url + schema
cap call stt '{"audio":"..."}'      # 直连调用(可选)
cap nodes                           # 列出全网 node 与能力
```

## 关键坑

- nats-micro 发现必须 `nc.PublishRequest`(带 reply),纯 `Publish` 无回程。
- **macOS 文件系统大小写不敏感**:`context.md` 与 `CONTEXT.md` 是同一文件,别并存(术语表只留 `CONTEXT.md`)。
- 转发模式要透传流式(SSE/WS/分块),反向代理别缓冲。
- 直连依赖 Tailscale:node 必须暴露在 tailnet 地址上,否则 NAT 内不可达。
- CLI 与 node 同一二进制,子命令区分角色;不要拆成两个二进制。

## Agent skills

### Issue tracker

本地 markdown:`.scratch/<feature-slug>/`(spec + issues,逐 ticket 一个文件)。See `docs/agents/issue-tracker.md`.

### Triage labels

默认五个标准标签(needs-triage / needs-info / ready-for-agent / ready-for-human / wontfix)。See `docs/agents/triage-labels.md`.

### Domain docs

single-context:根 `CONTEXT.md` + `docs/adr/`。See `docs/agents/domain.md`.

## 状态

- [x] 设计定稿(ADR-0001)+ 术语表(CONTEXT.md)
- [x] spec 已落本地 `.scratch/capnet-core/spec.md`
- [ ] to-tickets 拆票 → 实现(旧模型代码已清除)
