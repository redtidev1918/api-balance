# api-balance ⚖️

**语言 / Language:** 中文 · [English](/en/)

多 Provider AI API **余额 / 配额查询**与**低余额监控** CLI，专为 Linux 服务器长期运行设计。

```text
$ api-balance check
DeepSeek      ¥42.18
Kimi          ¥17.32
SiliconFlow   ¥8.61
OpenRouter    $12.43
MiniMax       23% remaining
```

**核心能力**

- 🗂️ 多 Provider 统一查询：`check` 一次性检查所有已配置的 Provider
- 🔔 低余额监控：`watch` 后台定时查询，低于阈值时通过 **Telegram / Webhook** 通知
- 🧩 自定义 Provider：通过 JMESPath 对接任意 JSON 余额 API
- 🚀 轻量：Go 单二进制，内存占用 ~3MB，systemd 友好
- 🔒 安全：不泄露 API Key，单 Provider 故障不影响整体

**技术栈**：Go · 单二进制 · systemd · .deb 发行

- [快速开始](QUICKSTART.md)
- [📥 下载](download.md)
- [GitHub 仓库](https://github.com/redtidev1918/api-balance)