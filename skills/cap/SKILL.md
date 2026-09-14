---
name: cap
description: Download and use the capnet CLI (`cap`) — a capability network where machines register STT/OCR/video abilities on a hub, agents discover them (`cap discover`) and call them directly (`cap call`). Use when a task needs STT, OCR, video understanding/generation, or any node-registered capability.
---

# cap — capnet 能力网络 CLI

`cap` 是一个单二进制:可扩展 node 外壳 + 动态注册表发现 + 直连调用。各机器(node)把能力(如 stt/ocr/video_gen)注册到 hub,本技能教你下载 CLI、发现能力、调用能力。

## 安装(自动下载)

```bash
# 按平台下载到 ~/.local/bin(不存在则创建)
os=$(uname -s | tr '[:upper:]' '[:lower:]')          # darwin | linux
arch=$(uname -m); [ "$arch" = "x86_64" ] && arch=amd64; [ "$arch" = "aarch64" ] && arch=arm64
url="https://github.com/rayson-x/capnet/releases/download/v0.1.0/cap-${os}-${arch}"
mkdir -p ~/.local/bin
curl -fsSL -o ~/.local/bin/cap "$url"
chmod +x ~/.local/bin/cap
export PATH="$HOME/.local/bin:$PATH"
cap --help 2>/dev/null || cap nodes --help >/dev/null 2>&1 && echo "cap installed"
```

Windows: 下载 `cap-windows-amd64.exe`,改名 `cap.exe`,放入 PATH。
若下载 404:release 还没建,先让仓库拥有者跑 `gh release create v0.1.0 /tmp/cap-* --repo rayson-x/capnet`。

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `CAP_HUB_URL` | `nats://127.0.0.1:4222` | hub nats 地址(远端可 `nats://<host>:4222` 或经 ssh -L 隧道用 `nats://127.0.0.1:<port>`) |
| `CAP_CONFIG` | `capabilities.yaml` | node 配置路径 |
| `TS_AUTHKEY` | 空 | 设置后 node 用内嵌 tsnet 自带 tailscale 入网(无需单独装) |

## 使用

```bash
cap nodes                        # 列出全网 node 与能力(先看有什么)
cap discover <capability>        # 查 hub:活的 node + base_url + schema
cap call <capability> '<json>'   # 按 schema 校验入参,直连 node 调用,打印结果
cap node status                  # 看本机 node 在 hub 的注册状态
cap hub                          # (仅 hub 机)跑动态注册表
cap node start                   # (node 机)启动 node 外壳
```

## 发现→调用 工作流(给 agent 的典型做法)

```bash
# 1. 先看有哪些能力
cap nodes
# 2. 找某个能力(如 STT)的活 node
cap discover stt
# 3. 直连调用(把 {audio: ...} 换成实际入参;schema 在 discover 结果里)
cap call stt '{"audio":"<audio 引用>"}'
```

- 返回的 `base_url` 是 node 的直连地址,调用直接打过去,**不经过 hub**。
- `cap call` 会按该能力的 JSON Schema 校验入参,非法会报错(如 `missing properties: 'who'`)。
- 长任务/流式:响应按 node 能力透传(SSE/分块)。

## 加能力(node 侧,给 node 拥有者)

编辑 `capabilities.yaml`(node 机器上),加一条:

```yaml
capabilities:
  - name: stt
    kind: forward                    # 转发到本机已有模型服务
    local: "http://127.0.0.1:9000/v1/audio/transcriptions"
    probe: "/health"
    schema: '{"type":"object","properties":{"audio":{"type":"string"}}}'
```

- `kind: forward`:node 反向代理到 `local` 服务,探活通过才注册。
- `kind: plugin`:进程内 Go 实现(需要改代码,见 capnet 仓库 `internal/cap/plugins.go`)。
- 心跳 15s / TTL 60s:死机自动从发现消失,新机器自动出现。

## 注意

- 无 node 提供该能力时 `cap discover` 返回空,`cap call` 报 `no live node provides ...`。
- 远端 hub 连不上先检查网络/隧道;本技能不解决 NAT 连通(那是 Tailscale/隧道的职责)。
