# 安全策略

## 报告漏洞

如果你发现了安全漏洞，**请勿公开提交 Issue**。

请通过 [GitHub Security Advisory](https://github.com/kserksi/summerain/security/advisories/new) 私下报告，或发邮件创建 advisory。我们会在收到报告后尽快确认并评估影响。

报告时请尽量包含：

- 问题的清晰描述与影响范围
- 复现步骤（最小可行示例）
- 受影响的版本
- 建议的修复方案（如有）

## 响应流程

1. 收到报告后 72 小时内确认
2. 评估严重程度并验证漏洞
3. 开发修复，视严重程度决定是否走私有分支
4. 发布修复版本，公开致谢（如报告者同意）

## 支持的版本

仅最新发布版本（`main` 分支）接收安全修复。旧版本不单独维护补丁。

## 部署安全要点

详见 [docs/USAGE.md](docs/USAGE.md)。关键项：

- 生产环境必须设置强随机的 `COOKIE_SECRET`、`IMGPROXY_KEY`、`IMGPROXY_SALT`
- `__Host-` 前缀 Cookie 要求 HTTPS + 同源，本地开发需用自签证书
- MySQL/Redis/imgproxy 容器应仅内网通信，不对外暴露端口
