# Cloudflare 免费版最大化 + 服务器最小攻击面 优化设计

- **日期**：2026-06-22
- **状态**：设计已审批，基于服务器实测信息更新，待编写实施计划
- **范围**：面向中国用户的同人志网站（美国纽约 VPS / Ubuntu 24.04 / Halo 博客 + 自研 Go 图床 summerain + ZFile 云盘 + Cloudreve 云盘 + WebFont 服务 + MySQL + Redis），通过 5 个阶段实现 Cloudflare 免费版能力最大化与服务器攻击面最小化
- **原则**：分阶段独立可验证、零停机或极短停机、每阶段有明确验证清单

---

## 一、背景与现状

### 1.1 运营数据（过去一个月）

| 指标 | 数值 |
|------|------|
| 唯一访问者 | 41.4K |
| 总请求数 | 8.07M |
| 带宽 | 1.1TB |
| 缓存百分比 | 98.9% |

### 1.2 服务器实测信息（cloud 别名，2026-06-22 探测）

| 项 | 值 |
|----|-----|
| 主机名 | kserks |
| 操作系统 | Ubuntu 24.04.4 LTS（内核 6.8.0-124-generic） |
| 公网 IP | 204.44.74.211（仅 IPv4） |
| SSH 端口 | 21093（仅密钥认证，禁止 root/密码） |
| CPU/内存 | 3 核 / 3.8GB（已用 1.8GB，可用 2.1GB，Swap 3GB） |
| 磁盘 | 59GB（已用 21GB，37%） |
| 域名 | kserks.org（在 Cloudflare 注册） |
| CDN | Cloudflare 免费版（传统代理模式，橙色云朵） |
| 邮件 | 域名无邮件服务 |
| 监控 | 无 |

### 1.3 当前子域名与服务架构

| 子域名 | 用途 | 后端 | 监听端口 |
|--------|------|------|---------|
| `www.kserks.org` | Halo 博客 | halo (Docker) | 127.0.0.1:8090 |
| `image.kserks.org` | summerain 图床（API + 前端 + 图片直链） | summerain-backend (Docker) | 127.0.0.1:8084 |
| `cloud.kserks.org` | ZFile 云盘 | zfile (Docker) | 127.0.0.1:8081 |
| `pan.kserks.org` | Cloudreve 云盘 | cloudreve (Docker) | 127.0.0.1:5212 |
| `font.kserks.org` | WebFont 服务 | python3（宿主机直跑，非 Docker） | 127.0.0.1:8087 |

### 1.4 Docker 架构（7 个容器）

```
网络隔离（已存在）：
  data_internal (internal)  ← MySQL, Redis, summerain-backend
  data_frontend (bridge)     ← MySQL, Redis, Halo, zfile, cloudreve
  summerain_ingress (bridge) ← summerain-backend

容器列表：
  mysql:9.7            — 无公网端口 ✓，内存限制1G，健康检查 ✓
  redis:8.8            — 无公网端口 ✓，有密码 ✓，内存限制512M
  halo:2               — 127.0.0.1:8090 ✓
  zfile:latest         — 127.0.0.1:8081 ✓，内存限制1G
  cloudreve:latest     — 127.0.0.1:5212 ✓，user:nobody
  summerain:latest     — 127.0.0.1:8084 ✓
  imgproxy:v3.12       — 内部8080（不发布）
```

Compose 文件位置（分散管理）：
- `/data/compose/mysql/docker-compose.yml`
- `/data/compose/redis/docker-compose.yml`
- `/data/compose/zfile/docker-compose.yml`
- `/data/compose/cloudreve/docker-compose.yml`
- `/data/halo/docker-compose.yml`
- `/home/kserks/summerain/docker-compose.yml`（summerain + imgproxy）

### 1.5 NGINX 现状（已很先进）

| 项 | 状态 |
|----|------|
| 版本 | nginx/1.30.2（自编译，含 HTTP/3 + Brotli + headers-more 模块） |
| HTTP/3 + QUIC | ✓ 已启用（`listen 443 quic`） |
| HTTP/2 | ✓ 已启用 |
| Brotli + gzip | ✓ 均启用 |
| headers-more 模块 | ✓（`more_clear_headers Server`） |
| CF Real IP | ✓ 完整配置所有 CF IP 段 |
| 安全头 | ✓ HSTS/CSP/X-Frame-Options/nosniff/Permissions-Policy |
| SSL | ✓ Mozilla Intermediate，TLS 1.2+1.3，OCSP Stapling |
| 证书 | ✓ 每子域名独立 CF Origin CA 证书 + `origin_ca_rsa_root.pem` |
| 配置结构 | `sites-available/` + `sites-enabled/` + `config/*.conf` |
| 监听地址 | ✗ `0.0.0.0`（需改为 `127.0.0.1`） |
| mTLS | ✗ 未启用（`ssl_verify_client` 缺失，但 CA 证书已有） |

### 1.6 安全状态

| 项 | 状态 | 评价 |
|----|------|------|
| UFW | ✓ active，default deny incoming，CF IP 白名单 + SSH 仅特定 IP | 已较好 |
| SSH | ✓ Port 21093，仅密钥，禁止 root/密码 | 已加固 |
| MySQL/Redis 公网暴露 | ✓ 不暴露 | 比预期好 |
| cloudflared | ✗ 未安装 | 需安装 |
| 容器加固 | ✗ 无 cap_drop / no-new-privileges | 需加固 |
| Redis 危险命令 | ✗ 未重命名 FLUSHDB/CONFIG/DEBUG/SHUTDOWN | 需重命名 |
| 密码管理 | ✗ 明文在 env/command | 需用 secrets |
| mTLS | ✗ 未启用 | 证书已有，仅需加 `ssl_verify_client on` |

### 1.7 已确认的关键决策

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 入站流量 | Cloudflare Tunnel | 公网 0 端口，最小攻击面 |
| 图片存储 | Cloudflare R2 | TOS 合规；~$0.60/月；出口流量 $0 |
| 图片展示 | R2 Custom Domain 直连 CDN | 不经源站，零 VPS 负载 |
| VPS 位置 | 未来迁至日本 | 中国用户延迟从 200-400ms 降至 80-200ms |
| 预算 | 免费为主 + R2 微付费 + 可考虑 Pro | 最大化免费版能力 |

### 1.8 现有代码资产

| 文件 | 内容 | 状态 |
|------|------|------|
| `backend/internal/service/r2_service.go` | R2 S3 SDK 集成（Upload/Download/Delete/MigrateLocalDir/Exists/PublicURL） | 已实现，默认禁用 |
| `backend/internal/handler/public_handler.go:109-120` | 公开图 R2 302 重定向逻辑 | 已实现，有条件 bug 需修复 |
| `docs/fixbug/security-audit.md` | 前后端安全审计报告 | 1 严重 + 1 高危 + 3 中危待修 |

---

## 二、目标架构

### 2.1 子域名规划（含新增）

| 子域名 | 用途 | 路由 | 服务端口 |
|--------|------|------|---------|
| `www.kserks.org` | Halo 博客 | Tunnel → NGINX → Halo | 127.0.0.1:8090 |
| `image.kserks.org` | 图床（API + 前端 + 私密图） | Tunnel → NGINX → summerain | 127.0.0.1:8084 |
| `cdn.kserks.org`（新增） | 公开图片展示（R2） | CNAME → R2 Custom Domain | — |
| `cloud.kserks.org` | ZFile 云盘 | Tunnel → NGINX → zfile | 127.0.0.1:8081 |
| `pan.kserks.org` | Cloudreve 云盘 | Tunnel → NGINX → cloudreve | 127.0.0.1:5212 |
| `font.kserks.org` | WebFont 服务 | Tunnel → NGINX → python3:8087 | 127.0.0.1:8087 |
| `status.kserks.org`（新增） | 公开状态页（UptimeFlare） | CNAME → CF Pages 域名 | — |

> **SSH 管理通道**：保留公网直连 `204.44.74.211:21093`，UFW 仅允许管理员 IP。SSH 已高度加固（非标端口、密钥认证、禁 root/密码、fail2ban），保留受限管理通道是合理的工程权衡，同时保持自动化工具（如 ssh-skill）的直连能力。

### 2.2 流量流向

```
中国用户（HTTP/3 + Brotli via CF edge）
       │
       ├─ www.kserks.org     ─┐
       ├─ image.kserks.org   ─┤
       ├─ cloud.kserks.org   ─┤  Cloudflare Tunnel（VPS 主动出站）
       ├─ pan.kserks.org     ─┤  → NGINX 127.0.0.1:443（mTLS）→ 后端
       ├─ font.kserks.org    ─┘
       │
       ├─ cdn.kserks.org → CNAME → R2 Custom Domain → CF CDN → bucket
       │
       └─ status.kserks.org → CNAME → CF Pages（UptimeFlare 状态页）
               ↑
      监控由 CF Workers 边缘 + UptimeRobot 外部独立运行
      image.kserks.org/i/<link>（公开图）→ 302 重定向 → cdn.kserks.org
      （私密图仍走 Tunnel → VPS，带 token 校验）

  管理员 SSH → 204.44.74.211:21093（UFW 仅允许管理员 IP，不经过 Tunnel）
```

**关键设计决策**：`image.kserks.org` 保留在 Tunnel 上（API + 前端 + 私密图使用同源相对路径，避免 CORS 问题）。公开图片展示通过 302 重定向到 `image-r2.kserks.org`（R2）。此逻辑已在 `public_handler.go:109-120` 实现。

### 2.3 安全分层（纵深防御）

| 层 | 机制 | 范围 |
|----|------|------|
| 1. 网络 | UFW：Web 端口仅 CF IP，SSH 仅管理员 IP，拒绝其余 | 80/443 零公网暴露 |
| 2. 隧道 | cloudflared 仅出站连接 | Web 无入站攻击面 |
| 3. 传输 | mTLS（Authenticated Origin Pulls） | 仅 CF 可达 NGINX |
| 4. 应用 | Zero Trust Access（OAuth/OTP） | 保护后台面板 |
| 5. 边缘 | WAF + Bot Fight + Rate Limiting | 在 CF 边缘拦截攻击 |
| 6. 容器 | Docker internal 网络 + cap_drop + no-new-privileges | MySQL/Redis/imgproxy 隔离 |
| 7. 系统 | SSH 仅密钥 + 非标端口 + UFW IP 限制 + fail2ban | SSH 管理通道最小暴露 |

### 2.4 相比初版设计的简化（基于实测）

服务器实际状态比设计假设的起点**好很多**，以下简化：

- **无需自编译 NGINX**：已自编译 1.30.2 + HTTP/3 + Brotli + headers-more，比设计预期更好
- **无需新建 CF Origin CA 证书**：`/etc/nginx/ssl/` 已有每子域名证书 + `origin_ca_rsa_root.pem`
- **MySQL/Redis 已不暴露公网**：阶段 1 的 Docker 端口隔离已完成
- **UFW 已配置 CF IP 白名单**：阶段 1 的防火墙基础已就绪
- **SSH 已加固**：端口 21093、仅密钥、禁止 root/密码
- **网络隔离已有 data_internal / data_frontend 分离**：Docker 网络架构基础良好
- **无需 UFW 白名单 CF IP（Tunnel 后）**：Tunnel 做出站连接，UFW 移除 80/443 的 CF IP 规则，仅保留 SSH 管理员 IP 规则

---

## 三、阶段 1：安全加固

**目标**：修复关键暴露、加固 Docker/系统，零停机。基于实测，多项基础已完成，本阶段聚焦剩余加固项。

### 3.1 Docker Compose 加固（剩余项）

实测发现 MySQL/Redis 已不暴露公网、已有网络隔离、已有内存限制。剩余加固：

| 问题 | 现状 | 修复 | 涉及文件 |
|------|------|-----|---------|
| 容器权限 | 未 drop | `cap_drop: ALL` + 每服务最小 `cap_add` | 所有 compose |
| 提权 | 允许 | `security_opt: no-new-privileges:true` | 所有 compose |
| Redis 危险命令 | 默认启用 | 重命名 FLUSHDB/FLUSHALL/CONFIG/DEBUG/SHUTDOWN | `/data/compose/redis/docker-compose.yml` |
| Redis 健康检查 | 无密码验证 | `redis-cli -a "$$(cat /run/secrets/redis_password)" ping` | 同上 |
| 密码管理 | 明文在 env/command | Docker secrets 文件 | 所有 compose |
| imgproxy key/salt | 需确认 | 确保 `IMGPROXY_KEY`/`IMGPROXY_SALT` 已设 | summerain compose |
| cloudreve user | `nobody` | ✓ 已是最佳实践 | 无需改 |

**Redis 危险命令重命名**（`/data/compose/redis/docker-compose.yml`）：
```yaml
command: >
  redis-server
  --requirepass "$$(cat /run/secrets/redis_password)"
  --appendonly yes
  --rename-command FLUSHDB ""
  --rename-command FLUSHALL ""
  --rename-command CONFIG ""
  --rename-command DEBUG ""
  --rename-command SHUTDOWN "CF_SHUTDOWN_6f3a"
```

**容器加固模板**（添加到每个服务）：
```yaml
security_opt:
  - no-new-privileges:true
cap_drop:
  - ALL
cap_add:              # 按需添加，多数服务不需要任何 cap
  - CHOWN
  - SETGID
  - SETUID
  - DAC_OVERRIDE
```

### 3.2 UFW 防火墙（过渡期 — Tunnel 前）

实测 UFW 已配置 CF IP 白名单 + SSH 仅特定 IP。无需大改，仅确认：

```bash
# 当前状态（已正确）：
# - default deny incoming
# - CF IP 段允许 80/443
# - SSH 仅 154.37.216.131 允许 21093

# 阶段 2 完成 Tunnel 迁移后，将移除所有入站规则
```

### 3.3 SSH 加固（已基本完成）

实测 SSH 已加固（Port 21093、仅密钥、禁止 root/密码）。剩余：

```ini
# 当前已配置：
Port 21093
PermitRootLogin no
PubkeyAuthentication yes
PasswordAuthentication no

# 阶段 2 保持 ListenAddress 0.0.0.0（SSH 保留公网直连，不走 Tunnel）
# 补充加固项：
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
ClientAliveInterval 300
ClientAliveCountMax 2
MaxAuthTries 3
LoginGraceTime 30
```

### 3.4 安全审计修复（来自 `docs/fixbug/security-audit.md`）

| 优先级 | 编号 | 问题 | 修复 | 工作量 |
|--------|------|------|------|--------|
| 严重 | SEC-C1 | `/api/v1/public/wm-test` 泄露全部配置 | 删除调试端点 | 1 分钟 |
| 高危 | SEC-H1 | `err.Error()` 泄露内部字段名 | 替换为 `"请求参数无效"` | 5 分钟 |
| 中危 | SEC-M1 | GIF MIME 不一致 | `allowedImageMIME` 添加 `"image/gif"` | 1 分钟 |
| 中危 | SEC-M2 | LIKE 通配符注入 | 转义 `%` 和 `_` | 5 分钟 |
| 中危 | SEC-M3 | 大小限制不一致（nginx 20m vs 后端 10m） | NGINX 统一为 10m | 1 分钟 |

### 3.5 内核与系统加固

- sysctl：`rp_filter=1`、`tcp_syncookies=1`、`tcp_congestion_control=bbr`、`tcp_fastopen=3`、`somaxconn=4096`、禁用 IPv6
- 禁用无用服务：`rpcbind`、`snapd`、`avahi-daemon`、`cups` 等
- `fail2ban`：SSH jail（maxretry=3, bantime=3600）
- `unattended-upgrades`：自动安装安全更新，不自动重启（✓ 已启用，✓ ESM 已配置）
- `logrotate`：NGINX + Docker 容器日志
- AppArmor：确认 enforced

### 3.6 CVE 自动修复与监控

**现状**：Ubuntu 系统包已有 `unattended-upgrades` 自动修复（含 ESM），但 Docker 镜像、NGINX（自编译）、Go 依赖均无自动跟踪机制。本节补充完整覆盖。

#### 3.6.1 覆盖范围与能力矩阵

| 层 | 机制 | 自动修复 | 监控告警 | 现状 |
|----|------|---------|---------|------|
| Ubuntu 系统包 | `unattended-upgrades` + ESM | ✅ 自动安装 | ✅ 日志 | ✓ 已启用 |
| Ubuntu 内核 | `unattended-upgrades` | ⚠️ 自动安装，不自动重启 | ✅ `/var/run/reboot-required` | ✓ 已启用 |
| Docker 镜像 | 自定义脚本（仅通知模式） | ❌ 仅通知，手动确认后更新 | ✅ 钉钉 | ✗ 需编写 |
| 容器 CVE 扫描 | Trivy（每日 cron） | ❌ 仅报告 | ✅ 钉钉 | ✗ 需安装 |
| 系统包 CVE | debsecan（每日 cron） | ❌ 仅报告 | ✅ 日志 | ✗ 需安装 |
| NGINX（自编译） | 版本检查脚本（每日 cron） | ❌ 手动重新编译 | ✅ 钉钉 | ✗ 需编写 |
| Go 依赖（summerain） | `govulncheck`（每周 cron） | ❌ 手动更新依赖 | ✅ 钉钉 | ✗ 需配置 |

> **设计原则**：系统包自动修复（低风险、高频率）；应用层仅通知不自动修复（避免破坏性更新）。所有 CVE 发现后通过钉钉告警，由管理员决定修复时机。
>
> **告警渠道选择**：Telegram 在中国大陆被墙，无法送达。钉钉 Webhook 是出站请求（VPS → `oapi.dingtalk.com`），不经 Cloudflare，纽约 VPS 到钉钉 API 约 200-400ms，对告警完全可用。

#### 3.6.2 Docker 镜像更新检查（自定义脚本）

Watchtower 的通知库（shoutrrr）不原生支持钉钉，改用自定义脚本实现相同功能：检查镜像更新、仅通知不更新。

```bash
# /opt/scripts/docker-update-check.sh
#!/bin/bash
source /opt/scripts/.env

# 遍历所有运行中容器的镜像
for img in $(sudo docker ps --format '{{.Image}}'); do
  # 拉取最新镜像（不替换运行中的容器）
  sudo docker pull "$img" > /dev/null 2>&1
  # 比较本地镜像与刚拉取的 digest
  local_digest=$(sudo docker images --digests "$img" --format '{{.Digest}}' | head -1)
  new_digest=$(sudo docker images --digests "$img" --format '{{.Digest}}' | tail -1)
  if [ "$local_digest" != "$new_digest" ] && [ -n "$new_digest" ]; then
    send_alert "🔔" "Docker 镜像更新可用：$img"
  fi
done
```

```bash
# crontab（每日 4AM）
0 4 * * * /opt/scripts/docker-update-check.sh >> /var/log/docker-update-check.log 2>&1
```

**关键设计**：
- **仅通知，不自动更新** — 避免镜像更新导致服务中断或配置不兼容
- 发现新镜像时通过钉钉通知，管理员手动 `docker pull` + `docker compose up -d`
- 统一使用 `/opt/scripts/.env` 中的钉钉 Webhook 配置

**监控的容器**：mysql、redis、halo、zfile、cloudreve、summerain-backend、imgproxy

#### 3.6.3 Trivy（容器镜像 CVE 扫描）

```bash
# 安装
sudo apt install -y trivy

# 每日扫描脚本：/opt/scripts/cve-scan.sh
#!/bin/bash
IMAGES=$(sudo docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>')
REPORT="/tmp/trivy-report-$(date +%Y%m%d).txt"
trivy image --severity HIGH,CRITICAL --format table -o "$REPORT" $IMAGES

# 有 HIGH/CRITICAL 时发送钉钉通知
CRITICAL_COUNT=$(trivy image --severity HIGH,CRITICAL --format json $IMAGES 2>/dev/null | jq '[.[].Results[].Vulnerabilities[]?] | length')
if [ "$CRITICAL_COUNT" -gt 0 ]; then
  send_alert "🔴" "Trivy CVE 扫描发现 $CRITICAL_COUNT 个 HIGH/CRITICAL 漏洞，详情见 $REPORT"
fi
```

```bash
# crontab
0 5 * * * /opt/scripts/cve-scan.sh >> /var/log/cve-scan.log 2>&1
```

**扫描范围**：所有 Docker 镜像的 HIGH + CRITICAL 级别漏洞。每日 5AM 执行（在镜像更新检查后 1 小时）。

#### 3.6.4 debsecan（系统包 CVE 监控）

```bash
# 安装
sudo apt install -y debsecan

# 每日报告
sudo debsecan --suite noble --format report --output /var/log/debsecan-report.txt

# crontab（每日 6AM）
0 6 * * * sudo debsecan --suite noble --format summary 2>&1 | mail -s "debsecan daily report" root@localhost
```

debsecan 列出所有已知 CVE 对应的包，管理员决定是否手动 `apt upgrade`（部分已被 unattended-upgrades 自动修复）。

#### 3.6.5 NGINX 版本检查（自编译版本专用）

NGINX 是自编译 1.30.2，apt 安全更新不覆盖。需手动跟踪 CVE 并重新编译。

```bash
# /opt/scripts/nginx-cve-check.sh
#!/bin/bash
CURRENT=$(nginx -v 2>&1 | grep -oP '\d+\.\d+\.\d+')
LATEST=$(curl -s https://nginx.org/en/CHANGES-1.30 2>/dev/null | grep -oP 'nginx/\K\d+\.\d+\.\d+' | head -1)

if [ -z "$LATEST" ]; then
  # 备用：从 nginx.org 主页获取
  LATEST=$(curl -s https://nginx.org/ 2>/dev/null | grep -oP 'nginx-\K\d+\.\d+\.\d+' | head -1)
fi

if [ "$CURRENT" != "$LATEST" ]; then
  send_alert "🔔" "NGINX 版本过期：当前 $CURRENT，最新 $LATEST。需手动重新编译。参考 https://nginx.org/en/CHANGES-1.30"
fi

# 检查已知 CVE
CVE_LIST=$(curl -s https://nvd.nist.gov/vuln/search/results?query=nginx&results_type=overview 2>/dev/null | grep -c 'CVE-')
echo "$(date): nginx=$CURRENT latest=$LATEST" >> /var/log/nginx-version-check.log
```

```bash
# crontab（每日 7AM）
0 7 * * * /opt/scripts/nginx-cve-check.sh >> /var/log/nginx-cve-check.log 2>&1
```

#### 3.6.6 govulncheck（Go 依赖漏洞扫描）

summerain 后端是 Go 项目，需定期扫描标准库和第三方依赖的已知漏洞。

```bash
# 安装（一次性）
go install golang.org/x/vuln/cmd/govulncheck@latest

# 扫描脚本：/opt/scripts/go-vuln-check.sh
#!/bin/bash
source /opt/scripts/.env
cd /home/kserks/imgcloud/backend
RESULT=$(/home/kserks/go/bin/govulncheck ./... 2>&1)
if echo "$RESULT" | grep -q "Vulnerability"; then
  COUNT=$(echo "$RESULT" | grep -c "Vulnerability")
  send_alert "⚠️" "govulncheck 发现 $COUNT 个 Go 依赖漏洞，需更新依赖。详见 /var/log/govulncheck.log"
  echo "$RESULT" > /var/log/govulncheck-$(date +%Y%m%d).log
fi
```

```bash
# crontab（每周一 8AM）
0 8 * * 1 /opt/scripts/go-vuln-check.sh
```

#### 3.6.7 告警通知渠道

所有 CVE 监控工具统一通过 **钉钉群机器人 Webhook** 通知：

```bash
# /opt/scripts/dingtalk-alert.sh（通用告警函数，所有 CVE 脚本 source 此文件）
#!/bin/bash
source /opt/scripts/.env

send_alert() {
  local severity=$1
  local message=$2
  curl -s -X POST "$DINGTALK_WEBHOOK" \
    -H "Content-Type: application/json" \
    -d "{\"msgtype\":\"text\",\"text\":{\"content\":\"$severity $message\"}}"
}
```

> **钉钉 Webhook 是出站请求**：VPS → `oapi.dingtalk.com`，不经 Cloudflare，不经 Tunnel。纽约到钉钉 API 约 200-400ms，对告警完全可用。钉钉不限制来源 IP，境外服务器可正常调用。

**通知分级**：

| 级别 | 触发条件 | 通知内容 |
|------|---------|---------|
| 🔴 CRITICAL | Trivy 发现 CRITICAL 漏洞 | 镜像名 + CVE ID + 修复版本 |
| 🟡 HIGH | Trivy 发现 HIGH 漏洞 | 镜像名 + CVE ID + 修复版本 |
| 🔔 INFO | 镜像更新检查发现新版本 | 容器名 + 当前版本 + 新版本 |
| 🔔 INFO | NGINX 版本过期 | 当前版本 + 最新版本 + CHANGES 链接 |
| 🔔 INFO | govulncheck 发现漏洞 | 包名 + CVE ID + 修复版本 |

**环境变量**（`/opt/scripts/.env`）：
```bash
# 钉钉群机器人 Webhook
# 获取方式：钉钉群 → 群设置 → 智能群助手 → 添加自定义机器人 → 复制 Webhook URL
DINGTALK_WEBHOOK=https://oapi.dingtalk.com/robot/send?access_token=<your_token>
```

#### 3.6.8 自动修复与手动修复边界

| 场景 | 自动修复 | 手动修复 | 理由 |
|------|---------|---------|------|
| Ubuntu 安全包 | ✅ unattended-upgrades | — | 低风险，高频率，自动修复利大于弊 |
| Ubuntu 内核 | ✅ 自动安装 | 手动重启 | 内核更新需重启，不自动重启避免服务中断 |
| Docker 镜像 | ❌ | ✅ 收到通知后 | 镜像更新可能引入 breaking change，需验证 |
| NGINX | ❌ | ✅ 收到通知后 | 自编译版本，需手动下载+编译+重启 |
| Go 依赖 | ❌ | ✅ 收到通知后 | 依赖更新需测试，避免引入新 bug |

#### 3.6.9 每日扫描时间线

| 时间 | 工具 | 动作 |
|------|------|------|
| 4:00 | unattended-upgrades | 自动安装 Ubuntu 安全包 |
| 4:00 | 镜像更新检查脚本 | 检查 Docker 镜像更新，仅通知 |
| 5:00 | Trivy | 扫描容器镜像 CVE，通知 HIGH/CRITICAL |
| 6:00 | debsecan | 报告系统包 CVE |
| 7:00 | NGINX 版本检查 | 对比最新版本，通知过期 |
| 8:00（周一） | govulncheck | 扫描 Go 依赖漏洞 |

> 错峰执行，避免资源争用。全部在凌晨低流量时段完成。

### 3.6 NGINX 加固

实测 NGINX 已很先进（HTTP/3、Brotli、安全头、CF Real IP、OCSP Stapling）。剩余：

- 确认 `server_tokens off`（已有）
- 确认安全头完整（已有）
- 确认 `set_real_ip_from` CF IP 段（已有）
- **阶段 2 将切换**：`listen 0.0.0.0:443` → `listen 127.0.0.1:443` + mTLS

### 3.7 验证清单

- [ ] 所有 compose 添加 `cap_drop: ALL` + `no-new-privileges:true`
- [ ] Redis 危险命令已重命名
- [ ] 密码改用 Docker secrets
- [ ] `curl https://image.kserks.org/api/v1/public/wm-test` → 404
- [ ] SEC-H1/M1/M2/M3 已修复
- [ ] `sysctl net.ipv4.tcp_congestion_control` → bbr
- [ ] fail2ban SSH jail active
- [ ] unattended-upgrades 启用（✓ 已有，确认 ESM 配置）
- [ ] `/opt/scripts/docker-update-check.sh` 可执行，crontab 配置完成
- [ ] Trivy 已安装，`/opt/scripts/cve-scan.sh` 可执行
- [ ] debsecan 已安装，crontab 配置完成
- [ ] `/opt/scripts/nginx-cve-check.sh` 可执行，crontab 配置完成
- [ ] `govulncheck` 已安装，`/opt/scripts/go-vuln-check.sh` 可执行
- [ ] `/opt/scripts/.env` 配置 `DINGTALK_WEBHOOK`
- [ ] 手动触发一次各扫描脚本，确认钉钉群收到测试通知

---

## 四、阶段 2：Cloudflare Tunnel 迁移

**目标**：Web 服务零公网入站端口（80/443）。SSH 保留受限管理通道。mTLS 确保仅 CF 可达 NGINX。

### 4.1 安装 cloudflared + 创建 Tunnel

```bash
# 添加 CF 包仓库 + 安装
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg
echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main" | sudo tee /etc/apt/sources.listd/cloudflared.list
sudo apt update && sudo apt install -y cloudflared

# 通过 Dashboard 创建 Tunnel（推荐）：
# CF Dashboard → Zero Trust → Networks → Tunnels → Create tunnel
# 名称：ny1 → 复制安装命令（含 token）→ 在服务器执行
```

### 4.2 配置 Public Hostnames

在 CF Dashboard → Tunnels → `ny1` → Public Hostnames 逐条添加：

| # | Subdomain | Domain | Service | URL | 备注 |
|---|-----------|--------|---------|-----|------|
| 1 | www | kserks.org | HTTPS | localhost:443 | TLS: Origin Server Name = www.kserks.org |
| 2 | image | kserks.org | HTTPS | localhost:443 | 同上 |
| 3 | cloud | kserks.org | HTTPS | localhost:443 | 同上 |
| 4 | pan | kserks.org | HTTPS | localhost:443 | 同上 |
| 5 | font | kserks.org | HTTPS | localhost:443 | 同上 |

每条 hostname 自动创建 CNAME → `<tunnel-id>.cfargotunnel.com`（橙色云朵）。**按子域名逐个切换**，CF DNS 即时更新，验证后再切换下一个。

> `status.kserks.org` 不走 Tunnel — 它是 UptimeFlare 的 CF Pages 自定义域名，直接 CNAME 到 Pages 域名。
>
> **SSH 不走 Tunnel** — 保留公网直连 `204.44.74.211:21093`，UFW 仅允许管理员 IP。SSH 已高度加固（密钥认证、禁 root/密码、非标端口、fail2ban），保留受限管理通道是合理的工程权衡，同时保持自动化工具的直连能力。

### 4.3 mTLS 启用（证书已有，仅需配置）

实测 `/etc/nginx/ssl/origin_ca_rsa_root.pem` 已存在。仅需在 NGINX 配置中添加 mTLS：

**每子域名 server 块添加**：
```nginx
ssl_client_certificate /etc/nginx/ssl/origin_ca_rsa_root.pem;
ssl_verify_client on;              # mTLS：拒绝非 CF 连接
```

**默认 server 块也添加**（`/etc/nginx/nginx.conf` 中的 default_server）：
```nginx
server {
    listen 127.0.0.1:80 default_server reuseport;
    listen 127.0.0.1:443 ssl default_server reuseport;
    listen 127.0.0.1:443 quic default_server reuseport;
    ssl_certificate /etc/nginx/ssl/kserks.org-fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/kserks.org.key;
    ssl_client_certificate /etc/nginx/ssl/origin_ca_rsa_root.pem;
    ssl_verify_client on;
    ssl_reject_handshake on;
    return 444;
}
```

**关键变更**：
- `listen` 绑定 `127.0.0.1`（当前为 `0.0.0.0`）
- `ssl_verify_client on` 强制 mTLS
- 非 CF 连接无法到达 NGINX

### 4.4 NGINX 配置变更（每子域名）

当前每个 site 配置中的 `listen` 段需统一修改：

```nginx
# 修改前（当前）：
listen 443 ssl;
listen [::]:443 ssl;
listen 443 quic;
listen [::]:443 quic;

# 修改后：
listen 127.0.0.1:443 ssl;
listen 127.0.0.1:443 quic;
# 移除 [::]: 监听（Tunnel 走 IPv4，IPv6 监听无意义）

# 添加 mTLS：
ssl_client_certificate /etc/nginx/ssl/origin_ca_rsa_root.pem;
ssl_verify_client on;
```

涉及文件（`/etc/nginx/sites-enabled/`）：
- `www.kserks.org.conf` → `/etc/nginx/sites-available/www.kserks.org.conf`
- `image.kserks.org.conf`（直接在 sites-enabled）
- `cloud.kserks.org.conf` → `/etc/nginx/sites-available/cloud.kserks.org.conf`
- `pan.kserks.org.conf` → `/etc/nginx/sites-available/pan.kserks.org.conf`
- `font.kserks.org.conf` → `/etc/nginx/sites-available/font.kserks.org.conf`

### 4.5 SSH 管理通道（保留公网直连）

**设计决策**：SSH 不走 Tunnel，保留公网直连。理由：

1. SSH 已高度加固（非标端口 21093、密钥认证、禁 root/密码、fail2ban）
2. UFW 仅允许管理员 IP（当前 154.37.216.131），攻击面极小
3. 保留自动化工具（如 ssh-skill / paramiko）的直连能力
4. "最小攻击面"不等于"零端口" — 保留受限管理通道是合理的工程权衡
5. 80/443 仍通过 Tunnel 实现零公网暴露，主要安全目标达成

**服务器端**：SSH 保持当前配置不变：
```ini
Port 21093
ListenAddress 0.0.0.0          # 保持公网监听
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
# 补充加固项：
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
ClientAliveInterval 300
ClientAliveCountMax 2
MaxAuthTries 3
LoginGraceTime 30
```

**UFW**：保留 SSH 管理员 IP 规则：
```bash
# 保持现有规则（不删除）
ufw allow from 154.37.216.131 to any port 21093 proto tcp
```

**客户端**：直连，无需 cloudflared 或 ProxyCommand：
```
Host cloud
  HostName 204.44.74.211
  User kserks
  Port 21093
  IdentityFile ~/.ssh/<your_key>
```

### 4.6 关闭 Web 公网端口（最终切换）

验证所有 Web 服务经 Tunnel 正常后，关闭 80/443 公网端口，**保留 SSH**：

```bash
# NGINX：以 127.0.0.1-only + mTLS 配置重启
sudo nginx -t && sudo systemctl restart nginx

# UFW：移除 80/443 的 CF IP 规则（循环删除所有 CF IP 段规则）
# 保留 SSH 规则：ufw allow from 154.37.216.131 to any port 21093 proto tcp
sudo ufw reload

# 验证：Web 端口已关闭，仅 SSH 开放
sudo ss -tlnp | grep -E '0\.0\.0\.0:(80|443)'
# 预期：无输出（80/443 不再监听公网）
sudo ss -tlnp | grep 21093
# 预期：0.0.0.0:21093（SSH 保留）
```

### 4.7 SSL/TLS Dashboard 设置

| 设置 | 值 |
|------|-----|
| 加密模式 | **Full (strict)** |
| Always Use HTTPS | On |
| Min TLS Version | 1.2 |
| TLS 1.3 | On |
| 0-RTT | On |
| HSTS | Enable（max-age=1年, includeSubDomains, preload） |
| Automatic HTTPS Rewrites | On |
| Authenticated Origin Pulls | On |

### 4.8 迁移顺序（每子域名约 5 分钟）

1. 验证当前服务正常（经旧 A 记录）
2. 添加 Tunnel public hostname（自动创建 CNAME，替换旧 A 记录）
3. 通过新 CNAME 测试（经 Tunnel）
4. 若故障：删除 Tunnel hostname，手动恢复旧 A 记录
5. 若正常：切换下一个子域名
6. 全部 5 个完成后：执行 4.6（关闭 Web 公网端口，保留 SSH）

### 4.9 验证清单

- [ ] `systemctl status cloudflared` → active (running)
- [ ] CF Dashboard Tunnel `ny1` → HEALTHY
- [ ] 所有 5 个子域名解析到 CF IP（`dig www.kserks.org +short` → 104.x/172.x）
- [ ] `ssh cloud` 直连正常（204.44.74.211:21093）
- [ ] `curl -k https://localhost:443 -H "Host: www.kserks.org"` 正常（mTLS）
- [ ] 直接 `curl https://204.44.74.211` → 连接被拒绝（80/443 已关闭）
- [ ] `ss -tlnp` → 80/443 不监听 0.0.0.0，仅 21093 监听（SSH 保留）
- [ ] `ufw status` → 80/443 规则已移除，仅保留 SSH 管理员 IP 规则

---

## 五、阶段 3：R2 启用

**目标**：公开图片从 R2 → CDN → 用户（展示零源站负载）。零停机。

### 5.1 创建 R2 Bucket

CF Dashboard → R2 → Create bucket：
- 名称：`doujin-images`
- Public access：**Enabled**（Public bucket）
- Custom Domain：`cdn.kserks.org`（自动在 CF DNS 创建 CNAME）
- Versioning：**Enabled**（误删恢复）
- Lifecycle：无（Standard 存储）

### 5.2 创建 R2 API Token

CF Dashboard → R2 → Manage R2 API Tokens → Create：
- 权限：**Object Read & Write**
- Bucket：`doujin-images`（限定范围）
- 返回：Access Key ID、Secret Access Key、Endpoint URL（`https://<account>.r2.cloudflarestorage.com`）

### 5.3 在后端启用 R2（经 Admin API）

`r2_service.go` 从 `system_configs` 表读取配置。通过以下接口设置：
```
PATCH /api/v1/admin/configs
{
  "items": [
    {"key": "r2_enabled", "value": "true"},
    {"key": "r2_endpoint", "value": "https://<account>.r2.cloudflarestorage.com"},
    {"key": "r2_access_key", "value": "<access_key>"},
    {"key": "r2_secret_key", "value": "<secret_key>"},
    {"key": "r2_bucket", "value": "doujin-images"},
    {"key": "r2_public_url", "value": "https://image-r2.kserks.org"}
  ]
}
```

R2 服务热重载配置（调用 `reload()`），无需重启。

### 5.4 迁移现有图片（零停机，用户自行执行）

`r2_service.go:165` 已实现 `MigrateLocalDir(basePath, subdir)`：
- 递归遍历 `/data/images/`（summerain_images volume）
- 将每个文件上传到 R2，保持相同相对路径
- 逐文件记录日志
- 通过 admin 端点或一次性脚本执行
- **无服务中断** — 图片同时存在于本地磁盘和 R2

> **由用户自行执行**：图片迁移涉及生产数据和带宽，由管理员自行选择时机和方式执行。本设计仅提供工具支持（`MigrateLocalDir` 已实现），不包含在自动执行计划中。迁移完成前，R2 重定向功能可暂不启用（`r2_enabled=false`），待迁移完成后再通过 Admin API 开启。

### 5.5 代码修复：R2 重定向条件（小改动）

**`public_handler.go:110` 的问题**：当前代码对原图和格式转换图都重定向到 R2。但处理图（如 `abc123.webp`）并未预先生成到 R2 — 它们由 imgproxy 从本地磁盘即时生成。重定向到不存在的 R2 对象 = 404。

**修复**：仅原图重定向到 R2；格式转换仍走 imgproxy：

```go
// 修改前：
if h.imageSvc.IsR2Enabled() && !isPrivate {

// 修改后：
if h.imageSvc.IsR2Enabled() && !isPrivate && format == "" {
```

**结果**：
- `/i/abc123`（原图）→ 302 到 `cdn.kserks.org/abc123`（R2，边缘缓存 1 年）
- `/i/abc123.webp`（格式转换）→ imgproxy 从本地磁盘 → CF 缓存结果
- 私密图 → VPS 带 token 校验（不走 R2）

### 5.6 代码修复：缓存头（小改动）

**`public_handler.go:199-210` 的问题**：公开图使用 `Cache-Control: no-cache, must-revalidate` — CF 每次请求都向源站验证。这是缓存率为 98.9% 而非 100% 的原因之一。

**修复**：按图片类型区分缓存头：

| 图片类型 | 当前头 | 新头 |
|---------|--------|------|
| 公开 + R2 重定向（302） | `no-cache, must-revalidate` | `public, max-age=86400`（缓存重定向 1 天） |
| 公开 + imgproxy（格式转换） | `no-cache, must-revalidate` | `public, max-age=31536000, immutable`（1 年） |
| 私密 | `no-store` | `no-store`（不变） |

### 5.7 imgproxy + R2 共存

**过渡期**：原图保留在本地磁盘（imgproxy 读取用于格式转换）。R2 服务原图用于展示。无需修改 imgproxy 配置。

**未来优化**（不在本计划范围）：配置 imgproxy 使用 S3 源，使其直接从 R2 读取，之后可清理本地磁盘。

### 5.8 R2 费用估算

| 项目 | 用量 | 免费额度 | 计费量 | 月费 |
|------|------|---------|--------|------|
| 存储（Standard） | ~50GB | 10GB | 40GB | $0.60 |
| Class A 操作（上传） | <1万次/月 | 100万次 | 0 | $0 |
| Class B 操作（读取/未命中回源） | ~96万次/月 | 1000万次 | 0 | $0 |
| 出口流量 | ∞ | ∞ | 0 | $0 |
| **合计** | | | | **~$0.60/月** |

### 5.9 CF TOS 合规

R2 属于 Cloudflare Developer Platform。通过 R2 Custom Domain 经 CDN 服务图片是条款明确允许的方式。

### 5.10 验证清单

- [ ] `dig cdn.kserks.org +short` → CF IP
- [ ] `curl -I https://cdn.kserks.org/<测试图片>` → 200 from R2
- [ ] Admin 配置显示 `r2_enabled=true`
- [ ] `curl -I https://image.kserks.org/i/<公开链接>` → 302 → `cdn.kserks.org`
- [ ] `curl -I https://image.kserks.org/i/<公开链接>.webp` → 200（imgproxy，非 R2）
- [ ] `curl -I https://image.kserks.org/i/<私密链接>` → 无 token 时 403
- [ ] CF Analytics 缓存率趋向 99%+

---

## 六、阶段 4：CF 控制台优化 + 监控

**目标**：最大化每个免费 CF 功能。零停机（全部为 Dashboard 配置变更）。

### 6.1 速度优化

CF Dashboard → Speed → Optimization：

| 设置 | 值 | 原因 |
|------|-----|------|
| HTTP/3 (QUIC) | **On** | 为中国用户减少 1-2 RTT |
| 0-RTT Connection Resumption | **On** | 回访用户零 RTT |
| Early Hints | **On** | TLS 握手期间预加载资源 |
| Brotli | **On** | 比 gzip 好 15-25% 压缩率 |
| Auto Minify | HTML + CSS + JS | 减小载荷体积 |
| Rocket Loader | **Off** | 破坏 Halo/zfile/cloudreve 前端 JS |
| Mirage | **Off** | 可能干扰同人志图片显示 |

### 6.2 Cache Rules（免费 10 条）

| # | 规则名 | 条件 | Edge TTL | Browser TTL |
|---|--------|------|----------|-------------|
| 1 | Cache-CDN-R2 | `Hostname eq cdn.kserks.org` | 1 year | 1 year |
| 2 | Cache-Img-Display | `Hostname eq image.kserks.org AND URI Path starts with /i/` | 1 year | 1 year |
| 3 | Cache-WWW-Assets | `Hostname eq www.kserks.org AND (URI Path ends with .css OR .js OR .woff2 OR .svg OR .png OR .jpg)` | 30 days | 30 days |
| 4 | Cache-WWW-HTML | `Hostname eq www.kserks.org AND NOT URI Path starts with /console` | 1 hour | — |
| 5 | Cache-Cloud-Static | `Hostname eq cloud.kserks.org AND (URI Path ends with .css OR .js OR .woff2)` | 30 days | 30 days |
| 6 | Cache-ImgApp-Static | `Hostname eq image.kserks.org AND (URI Path ends with .css OR .js OR .woff2 OR .svg)` | 30 days | 30 days |
| 7 | Bypass-Img-API | `Hostname eq image.kserks.org AND URI Path starts with /api/` | Bypass | — |
| 8 | Bypass-Admin | `URI Path starts with /console OR /admin OR /dashboard` | Bypass | — |
| 9 | Bypass-Status | `Hostname eq status.kserks.org` | Bypass | — |
| 10 | Bypass-Dynamic | `Hostname eq pan.kserks.org OR Hostname eq font.kserks.org` | Bypass | — |

**Cache Key 优化**（规则 1-2）：对不使用 `?w=` `?h=` `?q=` 的图片忽略 query string，跨变体共享缓存。

### 6.3 Tiered Cache + Always Online

- **Tiered Cache**：**On**（免费）— 上层 CF 节点在回源前先查缓存
- **Always Online**：**On**（免费）— 源站宕机时从 CF 缓存服务

### 6.4 WAF Custom Rules（免费 5 条）

| # | 规则名 | 表达式 | 动作 |
|---|--------|--------|------|
| 1 | Block-Scanner-Paths | `(uri.path eq "/xmlrpc.php") or (uri.path eq "/wp-login.php") or (uri.path eq "/.env") or (uri.path eq "/.git/config") or (uri.path eq "/wp-admin/")` | Block |
| 2 | Block-Bad-Methods | `(method eq "PUT" or method eq "DELETE" or method eq "TRACE") and not uri.path starts with "/api/"` | Block |
| 3 | Block-Bad-UA | `(ua contains "semrush") or (ua contains "ahrefs") or (ua contains "mj12bot") or (ua contains "dotbot")` | Block |
| 4 | Challenge-High-Threat | `(cf.threat_score gt 10)` | Managed Challenge |
| 5 | Limit-Upload-Spam | `(http.host eq "image.kserks.org") and (uri.path starts with "/api/v1/images/") and (method eq "POST") and (cf.threat_score gt 5)` | Challenge |

**同时启用**：
- Bot Fight Mode：**On**（免费）
- Browser Integrity Check：**On**
- Cloudflare Managed Ruleset：**On**（免费部分）
- Exposed Credentials Check：**On**

### 6.5 Rate Limiting（1 条自定义 + 1 条 Dashboard 基础）

| 规则 | 表达式 | 速率 | 动作 |
|------|--------|------|------|
| Limit-Upload（自定义） | `Hostname eq image.kserks.org AND URI Path starts with /api/v1/images/` | 10 req/min | Block |
| Limit-Login（Dashboard 基础） | `URI Path contains "/api/v1/auth/login" OR "/api/v1/auth/register"` | 5 req/10s | Challenge |

### 6.6 Transform Rules（免费 10 条 — Modify Response Header）

| # | 条件 | 动作 |
|---|------|------|
| 1 | All requests | Remove `Server` header |
| 2 | All requests | Set `X-Frame-Options: SAMEORIGIN` |
| 3 | All requests | Set `X-Content-Type-Options: nosniff` |
| 4 | All requests | Set `Referrer-Policy: strict-origin-when-cross-origin` |
| 5 | All requests | Set `Permissions-Policy: geolocation=(), microphone=(), camera=()` |

边缘层双保险（NGINX 也设置这些头）。

### 6.7 Zero Trust Access（免费最多 50 用户）

| Application | Domain | Session Duration |
|-------------|--------|------------------|
| Halo Admin | `www.kserks.org/console/*` | 24h |
| summerain Admin | `image.kserks.org/admin/*` | 24h |
| ZFile Admin | `cloud.kserks.org/admin/*` | 24h |
| Cloudreve Admin | `pan.kserks.org/dashboard/*` | 24h |

**Identity Provider**：One-time PIN（邮箱，零配置）或 GitHub OAuth（推荐）。

**Policy**：`Allow` 当 `Emails` 匹配你的邮箱。

### 6.8 Page Rules（免费 3 条，旧版 — 谨慎使用）

| # | URL pattern | 设置 |
|---|-------------|------|
| 1 | `*kserks.org/*console*` | Security Level: High |
| 2 | `cdn.kserks.org/*` | Cache Level: Cache Everything |
| 3 | 备用 | — |

### 6.9 Workers（免费 10万请求/天 — 有限使用）

站点约 26.9万请求/天。**不要全站用 Workers**（超免费额度）。

仅限以下场景：
- 自定义错误页（如 R2 404 → 品牌化"图片不存在"页面）
- 未来需要的缩略图 URL 重写

### 6.10 监控：UptimeFlare + UptimeRobot（混合方案）

**设计原则**：监控系统的可用性必须**高于**被监控系统的可用性。监控系统与被监控系统在同一台服务器上，若服务器宕机，监控本身也宕机，无法发出告警。因此采用**零 VPS 依赖**的外部监控方案。

#### 架构对比

| 方案 | 运行位置 | VPS 宕机时 | CF 宕机时 | 成本 | 服务器需求 |
|------|---------|-----------|-----------|------|-----------|
| ~~Uptime Kuma~~ | ~~VPS Docker~~ | ~~监控也宕机~~ | ~~正常~~ | ~~$0~~ | ~~需占用 VPS 资源~~ |
| UptimeFlare | CF Workers 边缘 | 正常告警 | 监控也宕机（极少） | $0 | 无 |
| UptimeRobot | UptimeRobot 自有服务器 | 正常告警 | 正常告警 | $0（免费50项） | 无 |
| **混合方案** | **CF 边缘 + 外部** | **双重告警** | **UptimeRobot 仍告警** | **$0** | **无** |

#### 6.10.1 UptimeFlare（主监控 — CF Workers 边缘）

- **仓库**：`github.com/lyc8503/UptimeFlare`
- **架构**：CF Workers + CRON Triggers + KV
- **特点**：全球数百城市发起检查，零 VPS 依赖，零维护
- **免费额度**：CF Workers 免费 10万请求/天 + KV 免费 1万次读/天

**部署步骤**：
1. `git clone https://github.com/lyc8503/UptimeFlare`
2. 编辑 `wrangler.toml`：配置 KV namespace、CRON schedule（每 5 分钟）
3. 编辑监控目标列表（`src/config.ts`）
4. `wrangler deploy` → 自动在 CF 边缘运行
5. 绑定自定义域名 `status.kserks.org`（CF Pages → Custom Domain）

**监控项**：
- `https://www.kserks.org` — HTTP(s)，5min
- `https://image.kserks.org/health` — HTTP(s)，5min
- `https://image.kserks.org/api/v1/public/stats` — HTTP(s) JSON 校验，5min
- `https://image-r2.kserks.org` — HTTP(s)，5min
- `https://cloud.kserks.org` — HTTP(s)，5min
- `https://pan.kserks.org` — HTTP(s)，5min
- `https://font.kserks.org` — HTTP(s)，5min

**公开状态页**：`status.kserks.org`（UptimeFlare 内置，CF Pages 托管）
**通知渠道**：钉钉 + 邮箱（UptimeFlare 内置支持 Webhook，可配置钉钉）

#### 6.10.2 UptimeRobot（辅助监控 — 外部独立）

- **网址**：`https://uptimerobot.com`
- **免费额度**：50 个监控项，5 分钟间隔
- **特点**：完全独立于 CF 和 VPS，双重保险

**配置步骤**：
1. 注册 UptimeRobot 账号
2. 添加监控项（同上 7 项）
3. 配置告警渠道：邮箱 + 钉钉 Webhook
4. 可选：启用公开状态页（UptimeRobot 自带 `status.uptimerobot.com/<your-id>`）

**作用**：
- 当 CF 宕机（极少但可能），UptimeFlare 也宕机时，UptimeRobot 仍能检测并告警
- 提供第二数据源，交叉验证可用性

#### 6.10.3 DNS 配置

`status.kserks.org` 不走 Tunnel，直接 CNAME 到 CF Pages 域名：

| 子域名 | 类型 | 目标 | 代理 |
|--------|------|------|------|
| `status` | CNAME | `<uptimeflare>.pages.dev` | 橙色云朵 |

> UptimeFlare 状态页托管在 CF Pages，`status.kserks.org` 作为 Pages 自定义域名，自动 HTTPS，无需 NGINX、无需 Tunnel、无需源站。

#### 6.10.4 为什么不用 Uptime Kuma

| 问题 | 说明 |
|------|------|
| 同机监控悖论 | 监控与被监控在同一 VPS，VPS 宕机时监控也宕机，无法告警 |
| 额外服务器成本 | 若单独租一台 VPS 跑 Kuma，增加 ~$24/月 成本和维护负担 |
| Push Monitor | Uptime Kuma 的 Push Monitor 需要脚本主动上报，脚本也在 VPS 上，同样会宕机 |
| 维护负担 | Kuma 需要更新、备份、维护，与"最小化"目标矛盾 |

UptimeFlare + UptimeRobot 完全无服务器、零维护、零额外成本，且监控可用性独立于 VPS 和 CF。

### 6.11 Notifications

CF Dashboard → Notifications：
- Security Events → 邮箱
- Tunnel Disconnect → 邮箱 + 钉钉
- Origin 5xx 激增 → 邮箱
- R2 操作错误 → 邮箱
- Weekly Analytics Summary → 邮箱

### 6.12 Pro Plan（$25/月）— 可选附加

| 功能 | 收益 | 是否值得 |
|------|------|---------|
| **Image Resizing** | CF 边缘转换图片（imgproxy 替代） | 可考虑 — 消除 imgproxy 容器 |
| **Polish** | 边缘无损/有损优化 | 是 — 为中国用户改善图片压缩 |
| **更多 WAF 规则** | 20 条自定义（vs 免费 5 条） | 否 — 5 条足够 |
| **Web Analytics** | 更长留存，隐私优先 | 可考虑 |
| **Argo Smart Routing** | 优化 CF 内部路由 | 否 — 仅 10-30ms 改善 |

**建议**：先免费。若 Polish/Image Resizing 对图片密集站点证明有价值，再考虑 Pro。

### 6.13 验证清单

- [ ] CF Analytics 缓存率趋向 99%+
- [ ] `curl -I https://www.kserks.org` → `cf-cache: HIT`
- [ ] `curl -I https://image-r2.kserks.org/<image>` → `cf-cache: HIT`，`alt-svc: h3=":443"`
- [ ] WAF Security 标签显示已拦截扫描器尝试
- [ ] `https://www.kserks.org/console` → 重定向到 CF Access 登录
- [ ] `https://status.kserks.org` → UptimeFlare 状态页（CF Pages）
- [ ] UptimeFlare 所有监控项绿色
- [ ] UptimeRobot 所有监控项绿色
- [ ] HTTP/3 验证：`curl --http3 -I https://www.kserks.org`

---

## 七、阶段 5：VPS 迁移（未来）+ 备份策略

**目标**：从纽约（~200-400ms）迁移到日本/新加坡（~80-200ms）为中国用户。Tunnel + R2 架构使迁移近乎无缝。

### 7.1 为什么迁移现在很简单

完成阶段 1-4 后，架构使迁移几乎无缝：

| 组件 | 需要迁移？ | 原因 |
|------|-----------|------|
| R2 图片 | **否** | 已在 R2，从 CF 边缘全球服务 |
| DNS 记录 | **否** | CNAME → tunnel 保持不变 |
| Tunnel 配置 | **复用 token** | 同一 tunnel ID，仅将 cloudflared 移到新 VPS |
| CF Dashboard 规则 | **否** | 所有规则基于域名，非 IP |
| Zero Trust Access | **否** | 基于域名，跟随 tunnel |

仅需迁移：**MySQL 数据、本地图片副本（供 imgproxy）、各应用 volume 数据**。

> 监控系统（UptimeFlare + UptimeRobot）无需迁移 — 它们不依赖 VPS。迁移期间监控持续运行，自动检测到短暂停机并告警。
>
> 备份无需迁移 — 已在 R2，全球可访问。

### 7.2 迁移顺序（约 2-4 小时计划停机）

1. ** provision 新 VPS**（推荐日本 — Vultr/Linode/DigitalOcean 东京）
2. **安装基础**：Docker、NGINX（复用当前配置）、cloudflared、UFW、SSH 加固（同阶段 1-2）
3. **在新 VPS 上恢复数据**：
   - MySQL：`mysql < dump.sql`（从 R2 备份恢复）
   - 配置文件：从 R2 备份恢复 NGINX 配置 + docker-compose + .env
   - 图片文件：`aws s3 sync` 从 R2 到 summerain_images volume
   - 各应用 volume：从旧 VPS `rsync` 或 `tar` 传输（cloudreve uploads、zfile data 等）
   - WebFont：从源仓库重新部署（静态文件）
4. **在新 VPS 上启动所有容器**，本地验证（`curl localhost:8084/health`）
5. **切换 Tunnel**：
   - 在旧 VPS 停止 cloudflared：`sudo systemctl stop cloudflared`
   - 在新 VPS 启动 cloudflared：`cloudflared service install <same-token>`
   - CF Dashboard 显示 tunnel `ny1`（重命名为 `jp1`）从新 IP 重连
   - **停机时间：约 10-30 秒**（Tunnel 重连时间）
6. **验证**所有服务经 CF 正常
7. **退役**旧 VPS（观察 48 小时后）

### 7.3 VPS 位置对比

| 位置 | 中国平均延迟 | VPS 价格（4GB） | 推荐 |
|------|-------------|----------------|------|
| 日本（东京） | 80-200ms | ~$24/月 | **最佳性价比** |
| 新加坡 | 80-200ms | ~$24/月 | 好的替代 |
| 香港 | 50-150ms | ~$40/月 | 最佳延迟，昂贵 |
| 美国（当前） | 200-400ms | ~$24/月 | 当前，对中国差 |

### 7.4 备份策略

**原则**：只备份无法重建的数据。大体积数据要么已在 R2（图片），要么可重新部署（WebFont 字体），要么无备份价值。MySQL 是唯一必须备份的数据 — 结构化数据，丢失即永久丢失。

**备份到 R2 `backups` bucket**（私有，30 天 lifecycle）：

| 数据 | 方法 | 频率 | 预估大小 | 备份理由 |
|------|------|------|---------|---------|
| MySQL | `mysqldump --single-transaction` → gzip → R2 | 每日 4AM | ~100MB | 结构化数据，无法重建 |
| 配置文件 | `tar czf`（NGINX 配置 + 所有 docker-compose + .env）→ R2 | 每周日 | ~5MB | 体积小，重建配置耗时 |

**不备份的数据及理由**：

| 数据 | 大小 | 不备份理由 |
|------|------|-----------|
| Redis | ~10MB | 缓存数据，重启自动重建 |
| Halo 数据 volume | 小 | 主要是配置，MySQL 已含博客内容 |
| zfile 数据 | 大 | 文件索引服务，实际文件在磁盘，配置在 MySQL |
| cloudreve 数据 | 大 | 用户上传内容，非核心业务数据 |
| WebFont 字体 | ~100MB | 静态文件，可从源仓库重新部署 |
| 图片 | 已在 R2 | R2 versioning 启用，自保护 |

**R2 存储估算**：
- 图片：~50GB
- 备份：100MB × 30 + 5MB × 4 ≈ 3GB
- 总计：~53GB，超出免费 10GB 部分 = 43GB × $0.015 = **$0.65/月**

**备份脚本**：cron job → `mysqldump` → gzip → `aws s3 cp` 到 R2 → lifecycle 自动清理 30 天前备份。

**恢复演练**：每月测试 MySQL 恢复以验证备份完整性。

### 7.5 验证清单（阶段 5）

- [ ] 新 VPS：所有容器运行，健康检查通过
- [ ] `curl --resolve www.kserks.org:443:127.0.0.1 https://www.kserks.org` 在新 VPS 上正常
- [ ] Tunnel 从新 IP 重连（CF Dashboard → HEALTHY）
- [ ] 所有服务从中国可访问（经 17ce.com 或 chinaz ping 测试）
- [ ] UptimeFlare / UptimeRobot 显示改善的响应时间
- [ ] 旧 VPS 观察 48 小时后退役
- [ ] MySQL 恢复演练完成（从 R2 备份恢复到测试环境）

---

## 八、完整计划汇总

| 阶段 | 重点 | 停机时间 | 关键交付物 | 预估时间 |
|------|------|---------|-----------|---------|
| 1 | 安全加固 | 无 | 容器加固、Redis 命令重命名、secrets、审计修复、内核、CVE 自动修复与监控 | ~2.5h |
| 2 | Tunnel 迁移 | 每子域名短暂 | cloudflared、mTLS、Web 0 公网端口（SSH 保留受限管理通道） | ~2.5h |
| 3 | R2 启用 | 无 | R2 bucket、缓存头修复（图片迁移由用户自行执行） | ~1h |
| 4 | CF 控制台优化 | 无 | 10 条 Cache Rules、5 条 WAF、Zero Trust、UptimeFlare + UptimeRobot、速度 | ~2h |
| 5 | VPS 迁移（未来） | ~30s（Tunnel 切换） | 日本 VPS、备份/恢复、数据迁移 | ~4h |

**最终状态**：Web 服务零公网入站端口（80/443 仅 127.0.0.1 + mTLS）、SSH 保留受限管理通道（UFW 仅管理员 IP）、R2 服务图片、99%+ 缓存率、WAF + Zero Trust 保护、HTTP/3 + Brotli + Early Hints 为中国用户、~80-200ms 延迟（阶段 5 后）。

---

## 九、风险与注意事项

### 9.1 阶段间依赖

- 阶段 2 依赖阶段 1 的安全加固（容器加固、审计修复）
- 阶段 3 可独立于阶段 2 执行（R2 启用不依赖 Tunnel）
- 阶段 4 依赖阶段 2 完成（Zero Trust Access 需要 Tunnel public hostname）
- 阶段 5 依赖阶段 1-4 全部完成

### 9.2 关键风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Tunnel 断连 | Web 服务不可用 | cloudflared 自动重连；Always Online 服务缓存内容；SSH 管理通道不受影响（独立于 Tunnel） |
| R2 配置错误 | 公开图 404 | 阶段 3 先在测试图片上验证，再全量迁移 |
| mTLS 证书过期 | CF 无法到源站 | CF Origin CA 证书 15 年有效期；设置日历提醒 |
| 迁移期间数据丢失 | MySQL 数据不一致 | 迁移前完整 `mysqldump` 到 R2；旧 VPS 保留 48 小时 |
| font.kserks.org 宿主机进程 | 非 Docker，迁移需额外处理 | 阶段 5 需单独迁移 python3 WebFont 服务 |

### 9.3 特殊注意事项（基于实测）

| 项 | 说明 |
|----|------|
| font.kserks.org | 宿主机 python3:8087（非 Docker），阶段 5 迁移时需单独处理，建议未来容器化 |
| 监控系统 | UptimeFlare（CF Workers）+ UptimeRobot（外部），均不依赖 VPS，迁移时无需处理 |
| NGINX 自编译 | 当前 1.30.2 含 HTTP/3 + Brotli + headers-more，迁移到新 VPS 需复用编译配置或重新编译。apt 安全更新不覆盖自编译版本，需每日版本检查脚本监控 CVE |
| CVE 自动修复 | 系统包自动修复（unattended-upgrades ✓ 已有）；Docker 镜像/NGINX/Go 依赖仅通知不自动修复，管理员手动处理 |
| Compose 文件分散 | 分布在 /data/compose/* 和 /home/kserks/summerain/，迁移时需全部覆盖 |
| image.kserks.org.conf | 直接在 sites-enabled（非软链接），与其他站点结构不同 |
| .env 明文密码 | /data/.env 含 MySQL/Redis/ZFileDB 明文密码，阶段 1 需改用 secrets |
| CSP 头 | 各站点已有详细 CSP 配置，阶段 4 的 Transform Rules 不应覆盖 NGINX 的 CSP |

### 9.4 不在本计划范围

- 前端代码修改（除 `public_handler.go` 两处小修复）
- Halo/zfile/cloudreve/summerain 应用本身的配置
- font.kserks.org WebFont 服务的容器化（建议未来单独处理）
- 国内 CDN 前置（未来可考虑，约 200-300 元/月，将延迟降至 < 20ms）
- Cloudflare China Network（需 Enterprise + ICP 备案）
- 优选 IP（违反 CF TOS §2.2.1(b)，明确不采用）

---

## 十、参考资源

- [Cloudflare Tunnel 文档](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
- [Cloudflare R2 文档](https://developers.cloudflare.com/r2/)
- [Cloudflare Cache Rules](https://developers.cloudflare.com/cache/how-to/cache-rules/)
- [Cloudflare WAF Custom Rules](https://developers.cloudflare.com/waf/custom-rules/)
- [Cloudflare Zero Trust Access](https://developers.cloudflare.com/cloudflare-one/policies/access/)
- [Cloudflare Authenticated Origin Pulls](https://developers.cloudflare.com/ssl/origin-configuration/authenticated-origin-pull/)
- [Cloudflare TOS §2.2.1](https://www.cloudflare.com/terms/) — 禁止优选 IP 条款
- [UptimeFlare](https://github.com/lyc8503/UptimeFlare)
- [UptimeRobot](https://uptimerobot.com)
