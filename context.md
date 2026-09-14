# context.md — 项目背景与决策记录

## 目标

把家里 RTX 4070 + Mac + 云服务器的空闲算力组成一个"能力共享网络":node 注册能力(STT/OCR/视频理解/视频生成),agent/CLI 发现并调用。**服务器只做发现,调用直连 node(P2P 风格)。**

## 决策记录(按时间)

1. **2026-09-13 选型**:弃 openOcto(Python,7★,单作者,只当架构参考)。底座选 **NATS JetStream + nats-micro**。A2A v1 作对外协议互通层(a2a-go/a2a-rs 含 server 实现)。
2. **2026-09-14 网状拓扑**:最初设计每台机器一个 nats-server,leaf/hub 组网,任务经 hub work-queue。**实测打通**:本地 CLI → 本地 leaf → WS(经 Caddy :80)→ 云 hub → 云 cap-node → 结果回。云上只开 80 端口,leaf 被迫走 WebSocket。
3. **2026-09-15 v0.4/v0.5 转向 P2P 直连**:
   - 参考 **Tailscale**:控制面(发现)走服务器,数据面 node 直连。Tailscale 客户端 BSD-3 开源(Go),协调服务器闭源(开源替代 Headscale),DERP 中继开源。
   - **v0.5 调用模型**:node 在自己 nats-server 上注册 micro 服务(端点=能力 rpc),CLI 经 mesh `$SRV.PING` 发现 `direct_addr`,直连 node `nc.Request("cap.<cap>.rpc")`。详见 `docs/design.md` §0.2。
   - 传输层计划用 **Tailscale/tsnet**(直连打洞 + DERP 回退);流媒体:文件走对象存储,实时媒体走 WebRTC/LiveKit(已有基建)。

## 架构现状(v0.3 已跑通, v0.5 待实现)

```
[CLI]──发现($SRV.PING,经 mesh)──▶[hub 云服务器]◀──node 注册──[4070/node]
   └──直连(node 本机 nats-server)──▶  cap.<cap>.rpc → 结果
```

- 当前云上在跑的是 v0.3 模型(每能力一个 micro 服务 + hub JetStream work-queue),cloud-developer `/home/ubuntu/capnode/`。
- v0.5 代码改造(每 node 一个服务 + rpc 端点 + CLI find/直连)尚未实现。

## 关键基础设施

- **云端 hub**:cloud-developer(54.151.241.139),只开 22/80。nats-server v2.14.6(hub.conf, JetStream + websocket:8081 + leafnodes:7422)+ cap-node(node-node)。Caddy `/nats*` → ws 8081。
- **本地**:Mac 跑 leaf nats-server(127.0.0.1:4222)拨云 hub(WS)。
- **二进制产物**:capnode + cap,跨平台已编译(linux amd64/arm64、darwin arm64、windows amd64)。

## 待办

- [ ] v0.5 实现(每 node 一个 micro 服务 + rpc 端点;CLI find/直连 call/--stream/submit-wait)
- [ ] Tailscale auth key → 云上入网 + join 脚本(装 tailscale + cap-node + systemd)
- [ ] 4070 真实模型(STT faster-whisper / OCR / video)
- [ ] 消息流式(progress_subject)+ SSE 网关(cap serve)
- [ ] NATS token 鉴权;对象存储接入

## 风险/注意

- NAT 内 node 直连必须靠 Tailscale(或类似),否则只能经 hub 中转。
- DERP 公共中继不适合大流量,兜底要自托管 DERP。
- workqueue consumer 必须 DeliverAll;micro 发现要 PublishRequest。
