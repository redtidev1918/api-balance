# 快速开始

**语言 / Language:** 中文 · [English](/en/QUICKSTART.md)

## 安装

```bash
# 1. 下载 .deb（从 GitHub Release）
wget https://github.com/redtidev1918/api-balance/releases/latest/download/api-balance_0.1.0_amd64.deb

# 2. 安装
sudo apt install ./api-balance_0.1.0_amd64.deb

# 3. 验证
which api-balance   # → /usr/bin/api-balance
api-balance version
```

> .deb 安装**不会自动启动**服务，需要时手动启用：
> ```bash
> sudo systemctl enable --now api-balance
> ```

## 配置

配置文件位于 `/etc/api-balance/api-balance.yaml`，参考示例：

```bash
sudo mkdir -p /etc/api-balance
# 从仓库复制 config.example.yaml 后编辑
```

为 Provider 填入你的 API Key：

```yaml
providers:
  - name: deepseek
    api_key: sk-xxx
  - name: openrouter
    api_key: sk-or-xxx
  - name: siliconflow
    api_key: sk-sf-xxx
  - name: moonshot
    api_key: sk-ms-xxx
```

## 查询余额

```bash
api-balance check
```

## 低余额监控

```bash
# 后台定时查询（默认 30 分钟），低于阈值时通知
api-balance watch

# 通过 systemd 常驻运行
sudo systemctl start api-balance
```

阈值与 Telegram / Webhook 通知在配置中设置。

## 提示

- Markdown 直接生效，保存推送后文档站自动更新
- 左侧导航编辑 `_sidebar.md`
- 英文页面写到 `docs/en/` 下的同名文件