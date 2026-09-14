# cap-node 系统设计:基于 NATS JetStream + nats-micro

> 状态:设计稿 v0.4(2026-09-15)。上游选型结论见 `cap-node-porting-reference.md`。
> 目标:**能力共享网络** —— 每台机器都是 node,通过类似 Tailscale 的方式被**直连访问**,服务器只负责**发现 node**。全部编译产物、低内存、Go 栈。

## 0. v0.4 核心模型:控制面走服务器,数据面直连 node(参考 Tailscale)

**分层**:
- **控制面(发现)= 服务器**:node 把 {capability, node_id, **direct_addr**, schema} 注册到服务器 registry;CLI 问"谁能做 stt" → 服务器回答 node 列表。
- **数据面(调用)= 直连 node**:CLI 拿到 node 的直连地址后,直接连接该 node 调用能力,**不经过 hub 转发**。
- **传输层 = Tailscale tailnet**(或等价平面网):NAT 内 node 靠打洞/DERP 获得可拨地址,CLI 才能直连。

```
[云服务器 hub]  ←─ 只做发现(registry),不承载数据流
   ▲  node 注册 {capability, node_id, direct_addr: nats://100.x.y.z:4222, schema}
   │
[CLI]  cap find stt → 得到直连地址列表
   │  ← 直连(Tailscale P2P 打洞 / DERP 回退)──▶
   ▼
[4070 node] nats-server + cap-node
   nc.Request("cap.stt.rpc", task) → 执行 → 结果直回
```

**node 侧**(每台机器):
- nats-server + cap-node 在本机提供能力端点(`cap.<cap>.rpc`,request-reply;长任务用 node 本地 JetStream)。
- 同时把能力表 + 直连地址注册到服务器 registry(控制面)。

**CLI 侧**:
- `cap find <cap>`:查服务器 registry → 所有有能力 node + 直连地址 + schema。
- `cap call <cap>`:选一个 node(按健康/负载)→ 直连 → `nc.Request`(短任务)/ 发布到 node 本地 JetStream(长任务)→ 订阅结果。
- **客户端调度**:发现返回所有有能力节点,CLI 挑一个,失败换下一个(替代 hub 集中队列的跨 node 重试)。

**取舍**:hub 集中 work-queue(自动负载均衡/跨 node 重试)退化为"服务器只做注册表 + CLI 客户端调度";换来的是直连、低延迟、hub 不承载数据流、hub 挂了已发现的调用照常。

> 注:下面的 §1-§8 保留 leaf/hub 网格与 hub JetStream 作为**回退/控制面**机制(发现本身走 hub;无 Tailscale 时数据面也可退化为经 hub 中转)。

## 0.2 v0.5:CLI 与 node 的 P2P 调用设计

**模型**:发现走 mesh(经 hub 汇聚),**调用直连 node 本机 nats-server**。node 既是"能力提供者"也是"可拨号端点"。

### 连接模型

```
[CLI]                                 [hub / 本地 leaf]
  │ 连接①(控制面)──── $SRV.PING 发现 ──▶  mesh 汇聚
  │                                     (只查,不承载数据)
  │ 连接②(数据面)──── 直连 node 本机 ──▶ [node 的 nats-server: nats://<tailnet-ip>:4222]
  ▼                                          │ cap-node 注册的端点
  nc.Request("cap.stt.rpc", task) ──────────▶│ 执行 → 回复结果
```

### 1. node 侧:一个 micro 服务 = 一个 node,能力是它的端点

cap-node 在本机 nats-server 上用 nats-micro 注册**以 node 为单位的服务**,每个能力一个 request-reply 端点:

```go
micro.AddService(nc, micro.Config{
    Name:    "4070-home",                    // node_id
    Version: "capnode-0.1.0",
    Metadata: map[string]string{
        "direct_addr": "nats://100.x.y.z:4222",          // 本机直连地址(CLI 用它拨号)
        "gpu":         "nvidia-rtx4070-12gb",
        "capabilities": `[{"name":"stt","version":"1.0.0","schema":"{...}","needs_gpu":true},...]`, // JSON 数组
    },
    Endpoints: []micro.EndpointConfig{       // 每个能力 = 一个请求-响应端点
        {Subject: "cap.stt.rpc",  Handler: sttHandler},
        {Subject: "cap.ocr.rpc",  Handler: ocrHandler},
        {Subject: "cap.video_gen.rpc", Handler: videoGenHandler},
    },
})
```

- node 本机 nats-server 监听 `0.0.0.0:4222`(tailnet 可达),`authorization { token }` 鉴权。
- 同时 `leafnodes { remotes: [hub] }` 挂进 mesh(控制面发现 + 回退中转)。
- micro 服务经 leaf/hub 兴趣桥接,**全网 $SRV.PING 可见**。

### 2. CLI 侧:两条连接

```
cap find <cap>     连接①:  $SRV.PING → 过滤 capabilities → [{node, direct_addr, schema, gpu}]
cap call <cap>     连接②:  直连 direct_addr → nc.Request("cap.<cap>.rpc", task) → 结果
cap call --stream  先订阅 progress_subject → 再 Request → 实时显示 → 最终结果
cap submit/wait    长任务:直连发布到 node 本地 JetStream cap.<cap>.task → task_id → 订阅 result
```

- 连接① 复用已有 mesh(连 hub 或本地 leaf),只做发现。
- 连接② 每次调用新建到 node 的直连(或 CLI 维护一个到常用 node 的连接池)。
- 失败重试:find 返回所有有能力 node,CLI 换下一个(`cap call --any` 语义)。

### 3. 消息格式(沿用,加 progress_subject)

```jsonc
// 任务(cap.<cap>.rpc 请求体 / cap.<cap>.task 消息)
{ "task_id": "...", "capability": "stt", "payload": {...},
  "progress_subject": "cap.stt.progress.<task_id>" }

// 进度(节点执行中发,可多条)
{ "task_id": "...", "type": "progress", "data": {"partial": "你好", "pct": 42} }

// 结果(回复 / cap.<cap>.result.<task_id>)
{ "task_id": "...", "capability": "stt", "status": "completed|failed",
  "output": {...}, "error": "...", "meta": {"node": "4070-home", "duration_ms": ...} }
```

### 4. 鉴权

- node nats-server:`authorization { token }`,CLI 配置里带同一 token(v1 共享 token,后续按 node 发 JWT)。
- 网络层隔离靠 Tailscale ACL;应用层靠 NATS token。

### 5. 与旧模型关系

- 短任务默认直连(request-reply);hub 集中 work-queue 保留为"调度后端"(客户端不想自己选 node 时)。
- 长任务:node 本地 JetStream(直连投递),CLI 断线可重连补拉。

## 0.1 旧模型(v0.2,保留作回退):能力共享网络 = 每台机器一个 nats-server

```
   ┌────────── hub(公网,被动监听) ──────────┐
   │  nats-server + JetStream(任务骨干)      │
   │  cap-node daemon + nats-micro(能力)     │
   └─────┬─────────┬─────────┬──────────────┘
    leaf │ 拨出    │ 拨出    │ 拨出
   ┌─────▼───┐ ┌──▼──────┐ ┌▼────────┐
   │ leaf A  │ │ leaf B  │ │ hub C   │  ← 每台机器都跑 nats-server,都能当转发
   │ 家里4070│ │ 云VM    │ │ 也是node │
   └─────────┘ └─────────┘ └─────────┘

   CLI 连任意可达的 server(A 或 B 或 C 或 hub)→ 通过 leaf/hub 兴趣桥接,全网可达
   · CLI→A→B:CLI 连 A,A 是 hub 或 leaf,经网状链路转发到 B
   · CLI→B→A:CLI 连 B,同理 —— 只取决于 CLI 拨到了谁
```

**核心模型:每个 node = nats-server(转发角色)+ cap-node daemon(能力角色),同一二进制。**

- **hub**:有公网/可达地址,`leafnodes { listen: 7422 }` **被动接受**连接;运行 JetStream(任务骨干)。
- **leaf**:NAT 内,`leafnodes { remotes: [{ url: nats://hub:7422 }] }` **主动拨出**;可同时拨多个 hub、可链式(leaf 的 hub 再作为上层 leaf)。
- **两者是同一个 nats-server 二进制,唯一差异就是 leafnodes 配置里是 listen 还是 remotes** —— 这正是"差异只有主动连接与被动连接"。

官方文档为此场景的定性(NATS Docs,leaf nodes):"网络在防火墙后面,互联网无法拨入,只能向外拨出……gateway 不适用(gateway 要求两端都能互连),**leaf node 正是为这种约束设计的**。leaf 向 hub 拨出一条连接,桥接即成立:无需入站防火墙规则、leaf 侧无需公网地址。"

## 1. 网状拓扑怎么支撑"node 互相发现 + 互相转发"

- **兴趣桥接**:leaf→hub 连接把两端订阅兴趣互相传播;只有对端有兴趣的 subject 才跨链。因此 `$SRV.*`(micro 发现)、任务/结果 subject 都能穿透全网。
- **node 间互相发现**:任意 node 上注册的 nats-micro 服务,经 leaf/hub 链路**全网格可见**。A 能发现 B 的能力,B 也能发现 A 的 —— 不依赖单一中心。
- **任意节点可做中转**:转发发生在 nats-server 层(leaf/hub),与能力提供解耦。一台机器可以只当中转(不开 cap-node),也可以又当 leaf 又提供能力。
- **CLI 入口**:CLI 配置一个"可达地址列表",依次尝试,连上谁就从谁进网;进网后经网格路由到目标能力 —— 天然支持 CLI→A→B 与 CLI→B→A。

## 2. 能力注册与发现(nats-micro,跨网格)

### 2.1 注册模型:一个 micro 服务 = 一个能力

每个支持某能力的 node 注册该能力的一个 micro 服务实例;注册在任意节点上都会经 leaf/hub 传播到全网。

```go
micro.AddService(nc, micro.Config{
    Name:    "stt",                    // 能力名(kebab-case)
    Version: "1.0.0",                  // schema 版本
    Metadata: map[string]string{
        "node_id":    "4070-home",
        "node_label": "家里的 RTX 4070",
        "gpu":        "nvidia-rtx4070,12GB",
        "schema":     `{"type":"object","properties":{"audio":{"type":"string"},"lang":{"enum":["zh","en"]}},"required":["audio"]}`, // JSON Schema 序列化字符串
        "route":      "leaf→hub-east", // 可达路径描述(可选,调试用)
    },
    Endpoints: []micro.EndpointConfig{
        {Subject: "cap.stt.rpc"},      // 可选:短任务 RPC 入口,见 §4
    },
})
```

⚠️ **已知点**(已从 nats.go 源码核实):当前 micro 没有 JSON Schema 字段,`schema` 必须塞进 `Metadata` 字符串。micro 自带在线心跳(`$SRV.PING/INFO/STATS`),node 挂掉自动从发现中消失。

### 2.2 发现

| 查询 | 返回 |
|---|---|
| `$SRV.PING` | 全网全部能力 × node 实例(含 metadata) |
| `$SRV.PING.stt` | 全网提供 stt 的所有 node |
| `$SRV.INFO.stt.<id>` | 单个实例明细 |

`cap nodes` 封装这些,输出:`[{"capability":"stt","nodes":["4070-home","cloud-gpu"],"schema":{...},"gpu":"..."}]`。

## 3. 任务分发(JetStream,跑在 hub 上)

JetStream 只在 hub 上跑(leaf 不跑 JetStream,经 leaf 链路访问 hub 的 JetStream)。**一个 hub(或 hub 集群)作为全网的持久任务骨干**。

### 3.1 Stream 约定

| Stream | subjects | retention | 说明 |
|---|---|---|---|
| `CAP_STT`(每能力一个) | `cap.stt.task` | WorkQueue(每条被消费一次) | 任务队列;storage=File(重启不丢) |
| `CAP_RESULTS`(全局一个) | `cap.<cap>.result.>` | Limits(留数天) | 结果;客户机离线可补拉 |

Stream 由 node 注册能力时幂等创建(`AddStream`,不存在才建)。

### 3.2 Consumer 约定(领取)

**每个能力一个共享 durable pull consumer**(durable 名 = 能力名,如 `stt`),全网支持该能力的 node **用同一 durable 名拉取** → NATS 保证每条任务只投给一个 node(work-queue 模式,自动负载均衡;单 node 挂掉由其它 node 顶上)。

```go
js.CreateOrUpdateConsumer("CAP_STT", consumer.Config{
    Durable:       "stt",
    AckPolicy:     consumer.AckExplicitPolicy,
    AckWait:       5 * time.Minute,     // 按能力给:STT 短,视频生成长(可到 30min)
    MaxDeliver:    3,
    BackOff:       []time.Duration{30 * time.Second, 5 * time.Minute, 30 * time.Minute},
    DeliverPolicy: consumer.DeliverNewPolicy,
})
```

### 3.3 任务生命周期

```
client 发布 → CAP_STT → 任一 node pull 领取(fetch(1)) → 执行 →
  ✓ 成功:    Ack() + 发布结果到 cap.stt.result.<task_id>
  ✗ 瞬时失败: Nak() → BackOff 后重投(重试计数 +1)
  ✗ 永久失败: Term()  → 结果标记 failed
  💀 node 崩溃: 不 ack → AckWait 超时 → 重投给其它 node
  ⚠️ 超限:   MaxDeliver 耗尽 → 旁路 DLQ stream(见 §7 坑)
```

### 3.4 消息格式

Task(发布到 `cap.<cap>.task`):
```json
{
  "task_id": "0193-...-uuid",
  "capability": "stt",
  "payload": { "audio": "s3://.../a.mp3", "lang": "zh" },
  "result_subject": "cap.stt.result.<task_id>",
  "timeout": 300
}
```

Result(发布到 `cap.<cap>.result.<task_id>`):
```json
{
  "task_id": "0193-...-uuid",
  "capability": "stt",
  "status": "completed",
  "output": { "text": "你好" },
  "meta": { "node": "4070-home", "duration_ms": 1200, "gpu_util": 45 }
}
```

## 4. 调用体验(cap CLI)

| 命令 | 语义 | 底层 |
|---|---|---|
| `cap nodes` | 列全网能力 + node + schema | `$SRV.PING` |
| `cap call stt --json '{...}'` | 短任务,**阻塞返回**(像本地命令) | 发布任务 + 订阅 result subject 等 |
| `cap submit video_gen --json '{...}'` | 长任务,立即返回 `{task_id}` | 发布任务 + 记 id |
| `cap wait <task_id>` | 等结果(轮询/阻塞) | 订阅 result subject |
| `cap connect <addr>` | 指定从哪个入口进网 | 连可达 server |

- 短任务走同一 JetStream 路径,只是 CLI 端阻塞(默认超时,如 60s);长任务 `submit` 返回 task_id,显式 `wait`,可跨会话恢复。
- (可选)亚秒级 RPC 可走 micro endpoint `cap.stt.rpc` 的 `nc.Request`,micro 队列组负载均衡;但建议统一走 JetStream,一条代码路径。

## 5. 失败与异步:原生语义对照

| 你的需求 | NATS 原生机制 | 备注 |
|---|---|---|
| 领取 | pull consumer `fetch` + AckExplicit | 领取即绑定,超时未 ack 重投 |
| 崩溃重投 | 不 ack → AckWait 过期 | 其它 node 自动接 |
| 失败重试 | `Nak()` + `BackOff` + `MaxDeliver` | 退避节奏按能力配 |
| 死信 | **无内建 DLQ 字段**(已核实) | 模式:`MaxDeliver` 超限 + 旁路 stream 承接 |
| 异步完成 | 结果 subject 订阅 / 结果 stream 补拉 | 客户机离线可后补 |
| 能力发现/在线 | micro PING/INFO/STATS 心跳,跨网格传播 | 无需自写 |

## 6. 每台机器要跑什么

| 角色 | nats-server | cap-node daemon | JetStream | 说明 |
|---|---|---|---|---|
| hub(公网) | `leafnodes { listen }` + 接受 client | ✅ | ✅ 任务骨干 | 云服务器 |
| leaf(4070/云VM) | `leafnodes { remotes }` 拨出 | ✅ | ❌(用 hub 的) | 同一二进制,只改配置 |
| 纯中转(可选) | 同上 | ❌ | 可选 | 只转发,不提供能力 |

部署 = 一台 hub(你的云服务器)+ 每台 NAT 内机器一个 leaf,全是同一个 `nats-server` + 同一个 `cap-node` 二进制,配置文件不同而已。

## 7. 断线重连与高可用(一直能找到 hub)

**leaf 重连是内置的,无需自写。** 四层保障:

1. **无限重连循环**:leaf 配了 `remotes` 后,nats-server 永不停止地尝试连接/重连(默认无限次 + 退避)。网络波动断开 → 自动重连;hub 宕机 → 退避重试。已从 nats-server 源码确认 leaf 支持 `reconnect_delay` / `reconnect_interval`。
2. **多 hub 回退**(官方文档原话:"A leaf can dial any hub server, so listing several gives it somewhere to reconnect if one is down"):
   ```hocon
   leafnodes {
     remotes: [ { urls: ["tls://hub1.example.com:7422", "tls://hub2.example.com:7422"] } ]
     reconnect_interval: 5s
     ping_interval: 30s
     pong_max: 2
   }
   ```
3. **保活探测**:`ping_interval` / `pong_max` 探测半开连接(网络闪断但 TCP 未断),发现即重连。
4. **进程守护**:leaf 的 nats-server 与 cap-node 都挂 systemd `Restart=always` / docker `restart: unless-stopped`,进程崩溃自动拉起并重连。

**两层重连分工**:
```
cap-node ──localhost──▶ 本地 nats-server(leaf) ──WAN──▶ hub
   客户端库自带重连            nats-server 负责 WAN 重连(上面 1-4)
```

**任务不丢**:JetStream 在 hub(File 存储)+ durable consumer → leaf 重连后断点续领,断线期间任务不丢、不重复。断线期间该 leaf 能力从发现中短暂消失,重连后自动恢复(心跳健康语义,可接受)。

## 8. 媒体文件传输:控制面与媒体面分离(性能与 RTC 选型)

**关键判断:STT/OCR/视频理解/视频生成都是"文件型"任务,不是实时流。**

| 内容 | 形态 | 通道 |
|---|---|---|
| STT 输入(音频文件)/ 视频理解输入 / 视频生成输出 | 文件(可能 GB 级) | **对象存储 + 引用**,不走消息总线 |
| 实时语音流(以后才考虑:边打电话边转写) | 实时流 | **WebRTC/LiveKit 媒体面** + NATS 控制面 |

**RTC(WebRTC)不用于文件型任务**:它是为实时交互设计的(亚秒延迟、SRTP、DataChannel),代价是信令、STUN/TURN 打洞、分块重装、编解码开销;对"上传文件等结果"纯属开销。

**推荐路径(控制面与媒体面分离)**:
```
CLI ──(先)上传文件到对象存储 S3/MinIO ──▶ 拿到引用
CLI ──publish 任务 JSON{"audio":"s3://.../a.mp3"}──▶ hub ──▶ leaf 4070 拉文件处理
结果 JSON(含输出引用) ──▶ CLI
```
- NATS 只传小 JSON 控制消息(任务/状态/结果/引用),socket 性能非瓶颈(NATS 本地百万级 msg/s)。
- 大文件走对象存储(MinIO 或已有 S3 兼容存储),跨 WAN 带宽是唯一限制,与协议无关。
- 任务/结果消息里字段就是引用地址(设计 §3.4 已如此)。

**无共享存储时的备选**:JetStream 分块流(ordered、持久、可重投、断点续传),注意默认单消息上限 1MB,需调 `max_message_size`。

**RTC 何时才上**:以后做实时场景(直播转写、实时视频理解)时,NATS 做信令/控制面,WebRTC/**LiveKit** 做媒体面(已有 LiveKit 基建可复用);node 上的模型消费 RTC 流。CLI→hub→internal node 链路里 RTC 只承载"流",任务调度仍是 NATS。

## 9. 已知坑(调研核实,勿重踩)

- **micro 无 schema 字段** → schema 放 `Metadata["schema"]` 字符串。
- **JetStream 无内建 DLQ 字段** → `MaxDeliver` 超限 + 旁路 stream 承接;ordered push consumer 不支持(用 pull)。
- **leaf 不跑 JetStream**:JetStream 在 hub 上,leaf 经链路访问;一个 leaf 连多个 hub 时只能用一个 JetStream 域 —— 所以任务骨干收敛到单一 hub(或一个 hub 集群)。
- **gateway ≠ leaf**:gateway 要求两端都能互连(公网↔公网),NAT 内必须用 leaf。
- 云到 node 的链路务必开 **TLS**(nats-server 原生支持,leaf remotes 配 tls)。

## 10. 可选增强

1. **指定 node 执行**:默认自动负载均衡;要指定某台 node,加 per-node subject(`cap.stt.node.<node_id>`),该 node 额外建 consumer 拉它。调度逻辑留在业务层。
2. **权限隔离**:NATS 账号系统,node 与 client 分账号;leaf 链路按账号绑定。
3. **Agent 生态互通**:`cap nodes` 发现结果 → A2A Agent Card;套 `a2a-go`(已含 server+client)暴露给外部 agent。
4. **GPU 排他**:12G 显存一次一个重模型 → node 内按能力加互斥锁(业务层),或把"当前负载"放进 metadata,CLI 据此选 node。
5. **回调**:`task_id` + 轮询已够 agent 用;如需服务方回调,在 Task 里加 `callback_subject`,完成时多 publish 一次。

## 11. 参考主源

- NATS Leaf nodes(官方文档,含"防火墙后只能向外拨出"的原文场景): https://docs.nats.io/running-a-nats-service/configuration/leafnodes
- NATS Topologies(单机/集群/超集群/leaf): https://docs.nats.io/operate/topologies
- nats.go micro(无 schema 字段,已源码核实): https://raw.githubusercontent.com/nats-io/nats.go/main/micro/service.go
- nats.go JetStream ConsumerConfig(MaxDeliver/BackOff/AckWait,无 DLQ 字段): https://raw.githubusercontent.com/nats-io/nats.go/main/jetstream/consumer_config.go
