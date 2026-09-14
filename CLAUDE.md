# capnet — 能力共享网络(cap-node)

让多台机器(NAT 内的 4070、云 VM、公网 server)互相注册/发现/调用能力,服务器只做发现,调用直连 node。Go 实现,基于 NATS JetStream + nats-micro。

## 快速命令

```bash
# 构建
go build ./cmd/capnode        # 本地 cap-node
go build ./cmd/cap            # CLI

# 交叉编译(发给各机器)
GOOS=linux  GOARCH=amd64 go build -o capnode-linux-amd64   ./cmd/capnode
GOOS=linux  GOARCH=arm64 go build -o capnode-linux-arm64   ./cmd/capnode
GOOS=darwin GOARCH=arm64 go build -o capnode-darwin-arm64  ./cmd/capnode
GOOS=windows GOARCH=amd64 go build -o capnode-windows-amd64.exe ./cmd/capnode

# 本地起 hub + node + CLI 跑通
nats-server -c deploy/hub.conf                       # 云端/本机 hub(JetStream)
nats-server -c deploy/leaf.conf                      # NAT 内机器的 leaf(拨出)
CONFIG=configs/capabilities.yaml ./capnode           # 一个 node
./cap nodes                                          # 发现
./cap call hello '{"who":"world"}'                   # 调用
```

## 架构(v0.5,详见 docs/design.md)

- **发现走 mesh(hub 汇聚)**:node 在本机 nats-server 上注册一个 micro 服务(node_id 为名),metadata 带 `direct_addr` + `capabilities[]`;全网 `$SRV.PING` 可见。
- **调用直连 node**:CLI 拿 `direct_addr` 后直连 node 本机 nats-server,`nc.Request("cap.<cap>.rpc", task)`。
- **扩展**:默认配置表(`configs/capabilities.yaml` → CommandCapability);自定义逻辑实现 `Capability` 接口(`cmd/capnode/capability.go`)注册。
- **传输**:Tailscale/tsnet 做 P2P 直连 underlay(待接入);大文件走对象存储;实时媒体走 WebRTC/LiveKit。

## 关键坑(踩过的,别重踩)

- nats.go v1.39:`CreateOrUpdateStream/CreateOrUpdateConsumer` 需要 `context.Context`;`Consumer.Fetch(batch, opts...)` 不需要 ctx。
- workqueue stream 的 consumer 必须 `DeliverPolicy: DeliverAllPolicy`(DeliverNew 会 400)。
- micro 发现必须 `nc.PublishRequest`(带 reply),纯 `Publish` 无回程。
- leaf 走 WebSocket:URL 要显式 `:80` + 路径 `/leafnode`;hub 必须配 `leafnodes { listen }` 块否则拒绝 leaf。
- `CommandCapability` 的 `{field}` 占位符是手动替换,不是 `os.Expand`(`os.Expand` 只认 `$var`)。
- 构建 CLI 注意:`cap call hello` 里被 `{who}` 注入的命令是 `sh -c`,不要把不可信 payload 直接拼进 exec(生产要白名单)。

## 状态

- [x] v0.2/0.3 PoC:hub work-queue + micro 发现,跨 mesh 全链路跑通(cloud hub + 本地 leaf)
- [ ] v0.5 改造:每 node 一个 micro 服务 + rpc 端点 + CLI `find`/直连 `call`
- [ ] Tailscale/tsnet underlay 接入
- [ ] 一键入网脚本 deploy/join.sh / join.ps1
- [ ] 真实能力(4070 STT/OCR/video)

测试环境:云端 hub 在 cloud-developer,当前跑的是旧模型(node 注册 + JetStream work-queue),配置在 `/home/ubuntu/capnode/`。

## Agent skills

### Issue tracker

GitHub issues via the `gh` CLI(repo `rayson-x/capnet`)。See `docs/agents/issue-tracker.md`.

### Triage labels

默认五个标准标签(needs-triage / needs-info / ready-for-agent / ready-for-human / wontfix)。See `docs/agents/triage-labels.md`.

### Domain docs

single-context:根 `CONTEXT.md` + `docs/adr/`。See `docs/agents/domain.md`。
