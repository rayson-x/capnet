# 05 — 真实模型接入:4070 faster-whisper 端到端

**What to build:** 在一台真实机器(优先家里 4070)上装 Tailscale + faster-whisper server + `cap node start`,注册 `stt` 转发能力;从 Mac 上 `cap discover stt` 找到它,直连 `/v1/audio/transcriptions` 对一段真实音频完成转写。这是"自托管模型 API 替代云服务"的第一条有效服务闭环。

**Blocked by:** 02 (转发能力)

**Status:** ready-for-agent

- [ ] 真实机器完成 Tailscale 入网 + faster-whisper server 启动
- [ ] `cap node start` 注册 `stt` 转发能力,Mac 上 `cap discover stt` 能发现
- [ ] Mac 上直连 `/v1/audio/transcriptions` 对真实音频返回可用的转写文本
- [ ] 记录部署步骤与踩坑(供后续机器一键入网参考)
