# 04 — CLI 完整命令面

**What to build:** 补全 CLI 命令:`cap nodes` 列出全网 node 与各自能力;`cap node status` 显示本机 node 注册状态;`cap discover <cap>` 返回的每个能力带出 JSON Schema;`cap call` 按 schema 校验入参后再直连调用。让 agent 可以用完整 CLI 完成发现、校验、调用。

**Blocked by:** 01 (最小垂直闭环)

**Status:** resolved

## Answer

2026-09-15 完成。

- `cap nodes` 列全网 node + 能力。
- `cap node status` 显示本机在 hub 的注册状态(读本地配置的 node_id,过滤发现结果)。
- `cap discover <cap>` 返回含每个能力的 JSON Schema。
- `cap call` 按能力 schema 校验入参(santhosh-tekuri/jsonschema),非法入参给清晰错误(如 `missing properties: 'who'`)。
- 测试:`TestValidateAgainstSchema`(合法通过 / 缺字段失败 / 类型错误失败 / 空 schema 跳过)。通过。

注:`cap node status` 需能读到本地 capabilities.yaml(用 `CAP_CONFIG` 指定或 cwd 有该文件)才知道 node_id。
