# 04 — CLI 完整命令面

**What to build:** 补全 CLI 命令:`cap nodes` 列出全网 node 与各自能力;`cap node status` 显示本机 node 注册状态;`cap discover <cap>` 返回的每个能力带出 JSON Schema;`cap call` 按 schema 校验入参后再直连调用。让 agent 可以用完整 CLI 完成发现、校验、调用。

**Blocked by:** 01 (最小垂直闭环)

**Status:** ready-for-agent

- [ ] `cap nodes` 列出全网 node + 能力
- [ ] `cap node status` 显示本机注册状态(便于本地调试)
- [ ] `cap discover <cap>` 返回结果含每个能力的 JSON Schema
- [ ] `cap call` 按 schema 校验入参,非法入参给出清晰错误
- [ ] 系统测试覆盖上述命令输出
