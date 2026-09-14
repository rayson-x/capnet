# AGENTS.md — 给 coding agent 的约定

## 这是什么

capnet = 自托管模型 API + 动态注册表。一个 `cap` 二进制:node 外壳把机器能力注册到 hub,agent/代码动态发现并直连调用。**模型实现本身不归本项目管。**

设计权威: `docs/adr/0001-self-hosted-model-api.md` + `CONTEXT.md`。**改设计先改 ADR/术语表,再改代码。**

## 代码布局

```
cmd/cap/main.go          单二进制 CLI 分发:hub / node start|status / discover / call / nodes
internal/cap/
  capabilities.go        Capability 接口 + capMeta(能力元数据)
  config.go              capabilities.yaml 加载
  node.go                node 外壳:读配置、探活、注册、心跳、健康重查、HTTP 暴露
  plugins.go             内置插件工厂(hello)
  registry.go            hub 侧注册表服务(cap.reg.set/hb/list + TTL 剔除)
  discover.go            发现(cap.reg.list)
  call.go                直连调用
  validate.go            schema 校验
configs/capabilities.yaml 能力配置表(kind: plugin|forward)
```

## 加一个能力(两种方式)

```yaml
# 方式 1 插件(进程内代码):实现 Capability 接口
# 方式 2 转发(指到本机已有服务):
capabilities:
  - name: stt
    kind: forward
    local: "http://127.0.0.1:9000/v1/audio/transcriptions"
    probe: "/health"
    schema: '{"type":"object",...}'
```

## Capability 接口契约

```go
type Capability interface {
    Name() string
    Schema() string           // JSON Schema(OpenAI 兼容工具形状)
    HandleHTTP(w, r)          // 插件对外是 HTTP handler,请求直接进代码
}
```

插件 = 进程内 HTTP handler,直接实现逻辑;转发 = 配置驱动,node 反向代理到 `local`。

## 注册表协议(cap hub)

- `cap.reg.set`:node 注册/更新能力(registry 回 ack,node 等确认)
- `cap.reg.hb.<node>`:心跳续租(兼容)
- `cap.reg.list`:发现(请求-响应,返回未过期条目)
- node 每次心跳重发 `cap.reg.set`(保证 hub 重启后能重新出现);TTL 60s 过期剔除

## 约定

- 单二进制,子命令化,不拆 CLI/node 两个二进制。
- 注册表只做发现,不代理流量;调用直连 node。
- 心跳 15s / TTL 60s;探活失败的能力不要注册/及时剔除。
- 流式(SSE/WS)要透传,别缓冲。
- 术语用 `CONTEXT.md`(Node/Capability/Plugin/Forward/Hub/Discover/Heartbeat)。
