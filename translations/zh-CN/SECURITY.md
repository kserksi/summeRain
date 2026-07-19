# 安全策略

## 报告漏洞

如果你发现安全漏洞，**请勿公开提交 Issue**。

请通过 [GitHub Security Advisories](https://github.com/kserksi/summerain/security/advisories/new) 私下报告。我们会尽快确认报告并评估其影响。

请尽量提供以下信息：

- 对问题及其影响的清晰描述
- 复现步骤，最好包含最小可复现示例
- 受影响的版本
- 建议的修复方案（如有）

## 响应流程

1. 我们会在 72 小时内确认收到报告。
2. 我们会评估严重程度并验证漏洞。
3. 我们会开发修复，并在严重程度需要时使用私有分支。
4. 我们会发布修复版本，并在报告者同意的情况下公开致谢。

## 支持的版本

仅 `main` 分支上的最新发布版本接收安全修复。旧版本不单独维护安全补丁。

## 部署安全

完整指南请参阅 [docs/USAGE.md](docs/USAGE.md)。关键要求包括：

- 生产环境必须为 `COOKIE_SECRET`、`IMGPROXY_KEY` 和 `IMGPROXY_SALT` 设置强随机值。
- 带 `__Host-` 前缀的 Cookie 要求 HTTPS 和同源部署。本地开发必须使用自签名证书。
- MySQL、Redis 和 imgproxy 容器应仅位于私有网络，不得公开暴露端口。
