# api-balance

多 Provider AI API 余额 / 配额查询与低余额监控 CLI。一个轻量、单二进制的 Linux 服务器工具。

```console
$ api-balance check
DeepSeek      ¥42.18
Kimi          ¥17.32
SiliconFlow   ¥8.61
OpenRouter    $12.43
MiniMax       23% remaining
```

```console
$ api-balance watch
watching every 30m0s (ctrl-c to stop)
```

## 特性

- **多 Provider 并发查询** — 单个 Provider 故障不影响其他
- **余额 / 配额区分** — 余额是钱(`¥42.18`),配额是剩余额度(`23% remaining`),绝不混淆
- **阈值告警** — 低于阈值时通过 Telegram / Webhook 通知
- **告警去重** — 已告警的不重复刷屏;恢复后再低于阈值会重新告警
- **自定义 Provider** — 无需改代码,用 YAML + JMESPath 定义任意 JSON 接口
- **机器可读 JSON** — `check --json` 输出稳定 schema
- **资源占用极低** — 空载 RAM < 5MB,适合 N100 之类的小服务器
- **无运行时依赖** — 单个 Go 静态二进制,直接拷走就能用

## 支持 Provider

| Provider | 类型 | 稳定性 |
|---|---|---|
| DeepSeek | balance | stable |
| OpenRouter | balance | stable |
| SiliconFlow | balance | stable |
| Moonshot / Kimi | balance | stable |
| MiniMax | quota | experimental |
| 任意自定义 (custom) | balance/quota/usage | — |

- `experimental` = 使用未公开 / 易变动的端点,API 可能随服务商更新失效
- `custom` = 用户通过配置从任意 JSON 接口提取余额

## 安装

**方式一:Debian / Ubuntu (.deb)**

```console
sudo apt install ./api-balance_0.1.0_amd64.deb
```

安装后:

```console
$ which api-balance
/usr/bin/api-balance
$ api-balance version
api-balance 0.1.0
```

**方式二:独立二进制**

```console
# 下载对应架构的二进制,放到 PATH 即可
sudo install -m755 api-balance-linux-amd64 /usr/local/bin/api-balance
```

## 配置

配置文件默认位置(优先级从低到高):

1. `/etc/api-balance/config.yaml` (系统级)
2. `~/.config/api-balance/config.yaml` (用户级)
3. CLI 参数 `--config PATH` (最高)

API key 建议用 `api_key_env` 指向环境变量,避免明文写入配置:

```yaml
providers:
  deepseek:
    api_key_env: DEEPSEEK_API_KEY
    threshold:
      balance: 10        # 余额低于 ¥10 告警

  openrouter:
    api_key_env: OPENROUTER_API_KEY
    threshold:
      balance: 5

  minimax:
    api_key_env: MINIMAX_API_KEY
    threshold:
      remaining_percent: 20   # 剩余低于 20% 告警

notifications:
  telegram:
    enabled: true
    bot_token_env: TELEGRAM_BOT_TOKEN
    chat_id_env: TELEGRAM_CHAT_ID
  webhook:
    enabled: false
    url: "https://example.com/hook"
  recovery: true   # 余额恢复后发送恢复通知
```

完整示例见 `docs/config.example.yaml`。

### systemd 服务长期运行

```console
sudo systemctl enable --now api-balance
```

服务以 **非 root 用户** `api-balance` 运行,加固(`NoNewPrivileges`、`ProtectSystem=strict`)。API key 放在 `/etc/api-balance/api-balance.env`(该文件应为 `root:root` 权限 `600`):

```
DEEPSEEK_API_KEY=sk-xxxx
OPENROUTER_API_KEY=sk-yyyy
```

日志:

```console
journalctl -u api-balance -f
```

## 命令

```console
api-balance check                    # 查询所有 Provider
api-balance check --json             # JSON 输出
api-balance watch                    # 持续监控(默认每30分钟)
api-balance watch --interval 15m     # 自定义间隔
api-balance watch --once             # 单次检查+评估(测试用)
api-balance providers                # 列出内置 Provider
api-balance config validate          # 校验配置
api-balance version
```

## 自定义 Provider

任意返回 JSON 的接口都能当作 Provider。用 JMESPath 提取字段:

```yaml
providers:
  my-api:
    type: custom
    endpoint: "https://api.example.com/v1/balance"
    method: GET
    auth:
      type: bearer            # bearer | header | query | basic
      token_env: MY_API_KEY
    extract:
      balance: "data.balance"    # JMESPath
      currency: "data.currency"
```

支持 `GET` / `POST`、`Bearer` / 消息头 / 查询参数 / Basic 认证,以及 `balance` / `used` / `total` / `remaining` / `reset_at` 的 JSON 提取。见 `docs/config.example.yaml`。

## 构建

需要 Go 1.23+:

```console
go build -o api-balance ./cmd/api-balance
```

发布构建(生成 amd64/arm64 二进制、.deb、checksums):

```console
bash scripts/build-release.sh 0.1.0
```

GitHub Actions(见 `.github/workflows/release.yml`)会在打 `v*` tag 时自动构建并发布到 GitHub Release。

## 测试

```console
go test ./...
go vet ./...
```

所有 Provider 都有 mock HTTP 测试,覆盖:正常响应、401/403、429、500、超时、非法 JSON、缺失字段等;核心逻辑测试覆盖并发、阈值、告警去重、恢复、JSON 输出、配置校验、密钥脱敏、状态持久化、通知失败。

## 安全

- **绝不打印 API key** — 日志、错误、debug 输出全部脱敏
- 配置文件建议 `chmod 600`
- systemd 服务以非 root 用户运行,带系统加固

## 致谢

本项目参考了以下出色的社区项目(仅作 API 端点与响应格式参考,不构成运行时依赖):

- [akitaonrails/ai-usagebar](https://github.com/akitaonrails/ai-usagebar) — AI 用量/余额监控(Rust)
- [Lottle7/dsh-quota](https://github.com/Lottle7/dsh-quota) — 可编程 AI API 余额查询(TypeScript)
- [wenzetan/dsh-quota-panel](https://github.com/wenzetan/dsh-quota-panel) — dsh-quota 管理面板,Provider 目录实现最易读的参考

## 许可证

MIT License。见 `LICENSE` 文件。