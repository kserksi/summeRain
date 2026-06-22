# Cloudflare 免费版最大化 + 服务器最小攻击面 优化设计

- **日期**：2026-06-22
- **状态**：设计已审批，待编写实施计划
- **范围**：面向中国用户的同人志网站（美国纽约 VPS / Ubuntu 24.04 / Halo 博客 + 自研 Go 图床 summerain + zfile 云盘 + MySQL + Redis），通过 5 个阶段实现 Cloudflare 免费版能力最大化与服务器攻击面最小化
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

### 1.2 当前架构

| 项 | 现状 |
|----|------|
| VPS 位置 | 美国纽约 |
| 操作系统 | Ubuntu 24.04 |
| IP | 仅 IPv4 |
| CDN | Cloudflare 免费版（传统代理模式，橙色云朵） |
| 域名 | 在 Cloudflare 注册 |
| 入站流量 | 80/443 端口公网开放，CF 反代到源站 |
| 服务 | Halo 博客、自研 Go 图床（summerain）、zfile 云盘、MySQL、Redis |
| 部署方式 | Docker，宿主机 NGINX 反代 |
| 监控 | 无 |
| 邮件 | 域名无邮件服务 |

### 1.3 已确认的关键决策

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 入站流量 | Cloudflare Tunnel | 公网 0 端口，最小攻击面 |
| 图片存储 | Cloudflare R2 | TOS 合规；~$0.60/月；出口流量 $0 |
| 图片展示 | R2 Custom Domain 直连 CDN | 不经源站，零 VPS 负载 |
| VPS 位置 | 未来迁至日本 | 中国用户延迟从 200-400ms 降至 80-200ms |
| 预算 | 免费为主 + R2 微付费 + 可考虑 Pro | 最大化免费版能力 |

### 1.4 现有代码资产

| 文件 | 内容 | 状态 |
|------|------|------|
| `backend/internal/service/r2_service.go` | R2 S3 SDK 集成（Upload/Download/Delete/MigrateLocalDir/Exists/PublicURL） | 已实现，默认禁用 |
| `backend/internal/handler/public_handler.go:109-120` | 公开图 R2 302 重定向逻辑 | 已实现，有条件 bug 需修复 |
| `backend/docker-compose.deploy.yml` | 生产部署 compose（127.0.0.1 绑定 + internal 网络） | 基础良好，需加固 |
| `docs/fixbug/security-audit.md` | 前后端安全审计报告 | 1 严重 + 1 高危 + 3 中危待修 |

---

## 二、目标架构

### 2.1 子域名规划

| 子域名 | 用途 | 路由 | 服务端口 |
|--------|------|------|---------|
| `blog.example.com` | Halo 博客 | Tunnel → NGINX → Halo | 127.0.0.1:8090 |
| `img.example.com` | 图床（API + 前端 + 私密图） | Tunnel → NGINX → summerain | 127.0.0.1:8080 |
| `cdn.example.com` | 公开图片展示（R2） | CNAME → R2 Custom Domain | — |
| `drive.example.com` | zfile 云盘 | Tunnel → NGINX → zfile | 127.0.0.1:8081 |
| `status.example.com` | Uptime Kuma 监控 | Tunnel → NGINX → Kuma | 127.0.0.1:3001 |
| `ssh.example.com` | SSH 经 Tunnel | Tunnel → ssh://localhost:22 | 127.0.0.1:22 |

### 2.2 流量流向

```
中国用户（HTTP/3 + Brotli via CF edge）
       │
       ├─ blog.example.com   ─┐
       ├─ img.example.com    ─┤  Cloudflare Tunnel（VPS 主动出站）
       ├─ drive.example.com  ─┤  → NGINX 127.0.0.1:443（mTLS）→ Docker
       ├─ status.example.com ─┤
       ├─ ssh.example.com    ─┘
       │
       └─ cdn.example.com → CNAME → R2 Custom Domain → CF CDN → bucket
              ↑
     img.example.com/i/<link>（公开图）→ 302 重定向 → cdn.example.com
     （私密图仍走 Tunnel → VPS，带 token 校验）
```

**关键设计决策**：`img.example.com` 保留在 Tunnel 上（API + 前端 + 私密图使用同源相对路径，避免 CORS 问题）。公开图片展示通过 302 重定向到 `cdn.example.com`（R2）。此逻辑已在 `public_handler.go:109-120` 实现。

### 2.3 安全分层（纵深防御）

| 层 | 机制 | 范围 |
|----|------|------|
| 1. 网络 | UFW：拒绝所有入站，允许出站 | 零公网端口 |
| 2. 隧道 | cloudflared 仅出站连接 | 无入站攻击面 |
| 3. 传输 | mTLS（Authenticated Origin Pulls） | 仅 CF 可达 NGINX |
| 4. 应用 | Zero Trust Access（OAuth/OTP） | 保护后台面板 |
| 5. 边缘 | WAF + Bot Fight + Rate Limiting | 在 CF 边缘拦截攻击 |
| 6. 容器 | Docker internal 网络 + cap_drop + no-new-privileges | MySQL/Redis/imgproxy 隔离 |
| 7. 系统 | SSH 仅密钥 + ListenAddress 127.0.0.1 + fail2ban | SSH 仅经 Tunnel |

### 2.4 相比 manual/ 的简化

本设计**不复用** `manual/` 目录中的手册（该手册针对日本/Debian/Ghost/Seafile/MariaDB 的不同场景）。但参考其最佳实践模式。以下简化值得关注：

- **无需自编译 NGINX**：CF 边缘负责 HTTP/3 + Brotli 到用户。Tunnel→源站走 HTTP/2 over TCP。标准 `apt install nginx` 足够。
- **无需 UFW 白名单 CF IP**：Tunnel 做出站连接，无入站 CF IP 规则需求。UFW 直接拒绝所有入站。
- **无 CORS 问题**：`img.example.com` 保持单域名。R2 展示使用独立 `cdn.example.com`。

---

## 三、阶段 1：安全加固

**目标**：修复关键暴露、加固 Docker/系统，零停机。

### 3.1 Docker Compose 加固（关键）

`backend/docker-compose.deploy.yml` 基础良好，需补充：

| 问题 | 现状 | 修复 |
|------|------|------|
| MySQL/Redis 端口 | 开发 compose 暴露 0.0.0.0:3306/6379 | 无 `ports:` 段 — 仅 internal 网络 |
| Redis 密码 | 无 | 添加 `--requirepass` + secrets 文件 |
| Redis 危险命令 | 默认启用 | 重命名 FLUSHDB/FLUSHALL/CONFIG/DEBUG/SHUTDOWN |
| 容器权限 | 未 drop | `cap_drop: ALL` + 每服务最小 `cap_add` |
| 提权 | 允许 | `security_opt: no-new-privileges:true` |
| 内存限制 | 无 | 每服务 `mem_limit` |
| 网络隔离 | 默认非 internal | `internal: true` 用于 DB/缓存网络；单独 bridge 用于 backend→R2 |
| Secrets | 明文 env | Docker secrets 文件 |
| imgproxy key/salt | 开发为空 | 设置 `IMGPROXY_KEY`/`IMGPROXY_SALT` |

**网络拓扑**：
```
Docker 网络：
  app-internal（internal: true）    ← MySQL、Redis、imgproxy（无互联网访问）
  app-proxy（bridge）               ← Halo、summerain、zfile、Kuma（绑定 127.0.0.1）
```

summerain 连接两个网络（需要 app-internal 访问 DB/Redis/imgproxy，app-proxy 供 NGINX 反代）。

### 3.2 UFW 防火墙（过渡期 — Tunnel 前）

```bash
# 阶段 1：限制为 CF IP（仍为传统代理）
ufw default deny incoming
ufw default allow outgoing
ufw allow in on lo

# 仅允许 Cloudflare IP 访问 80/443
for ip in $(curl -s https://www.cloudflare.com/ips-v4); do
  ufw allow from $ip to any port 80,443 proto tcp
done

# SSH：仅你的 IP（临时，阶段 2 移除）
ufw allow from <YOUR_IP>/32 to any port 22 proto tcp

ufw enable
```

### 3.3 SSH 加固

```ini
Port 22
ListenAddress 0.0.0.0          # → 127.0.0.1 在阶段 2
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
MaxAuthTries 3
LoginGraceTime 30
AllowUsers <your_user>
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
ClientAliveInterval 300
ClientAliveCountMax 2
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
- `unattended-upgrades`：自动安装安全更新，不自动重启
- `logrotate`：NGINX + Docker 容器日志
- AppArmor：确认 enforced

### 3.6 NGINX 加固（Tunnel 前）

- `server_tokens off`
- 安全头：HSTS、X-Content-Type-Options、X-Frame-Options、Referrer-Policy、Permissions-Policy
- `set_real_ip_from` CF IP 段 + `real_ip_header CF-Connecting-IP`
- 速率限制 zone：`login`（5r/m）、`api`（30r/s）
- 阶段 2 将切换为 `listen 127.0.0.1:443 ssl` + mTLS

### 3.7 验证清单

- [ ] `docker compose ps` — MySQL/Redis 无发布端口
- [ ] `ss -tlnp` — 3306/6379 不监听 0.0.0.0
- [ ] `ufw status` — 仅 CF IP 在 80/443，你的 IP 在 22
- [ ] `curl https://img.example.com/api/v1/public/wm-test` → 404
- [ ] `sysctl net.ipv4.tcp_congestion_control` → bbr
- [ ] SSH 密码登录被拒绝

---

## 四、阶段 2：Cloudflare Tunnel 迁移

**目标**：零公网入站端口。所有流量经 Tunnel。mTLS 确保仅 CF 可达 NGINX。

### 4.1 安装 cloudflared + 创建 Tunnel

```bash
# 添加 CF 包仓库 + 安装
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg
echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main" | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update && sudo apt install -y cloudflared

# 通过 Dashboard 创建 Tunnel（推荐）：
# CF Dashboard → Zero Trust → Networks → Tunnels → Create tunnel
# 名称：ny1 → 复制安装命令（含 token）→ 在服务器执行
```

### 4.2 配置 Public Hostnames

在 CF Dashboard → Tunnels → `ny1` → Public Hostnames 逐条添加：

| # | Subdomain | Domain | Service | URL | 备注 |
|---|-----------|--------|---------|-----|------|
| 1 | blog | example.com | HTTPS | localhost:443 | TLS: Origin Server Name = blog.example.com |
| 2 | img | example.com | HTTPS | localhost:443 | 同上 |
| 3 | drive | example.com | HTTPS | localhost:443 | 同上 |
| 4 | status | example.com | HTTPS | localhost:443 | 同上 |
| 5 | ssh | example.com | SSH | localhost:22 | — |

每条 hostname 自动创建 CNAME → `<tunnel-id>.cfargotunnel.com`（橙色云朵）。**按子域名逐个切换**，CF DNS 即时更新，验证后再切换下一个。

### 4.3 Origin CA 证书 + mTLS

**生成 Origin CA 证书**（CF Dashboard → SSL/TLS → Origin Server → Create Certificate）：
- 类型：RSA（2048）
- Hostnames：`*.example.com`（通配符覆盖所有子域名）
- 有效期：15 年
- 将 PEM（证书 + 私钥）保存到服务器：`/opt/app/nginx/ssl/origin.pem`、`origin.key`

**启用 Authenticated Origin Pulls（mTLS）**：
- CF Dashboard → SSL/TLS → Origin Server → Authenticated Origin Pulls → **On**
- 下载 CF Origin Pull CA：`https://developers.cloudflare.com/ssl/static/authenticated_origin_pull_ca.pem`
- 保存到 `/opt/app/nginx/ssl/cloudflare-ca.pem`

### 4.4 NGINX 配置（Tunnel 后）

```nginx
# 00-default.conf — 拒绝非 CF 连接
server {
    listen 127.0.0.1:80 default_server;
    listen 127.0.0.1:443 ssl default_server;
    http2 on;

    ssl_certificate /opt/app/nginx/ssl/origin.pem;
    ssl_certificate_key /opt/app/nginx/ssl/origin.key;
    ssl_client_certificate /opt/app/nginx/ssl/cloudflare-ca.pem;
    ssl_verify_client on;              # mTLS：拒绝非 CF 连接

    server_name _;
    return 444;
}

# 每个子域名 server 块：
server {
    listen 127.0.0.1:443 ssl;
    http2 on;
    server_name blog.example.com;      # 按子域名

    ssl_certificate /opt/app/nginx/ssl/origin.pem;
    ssl_certificate_key /opt/app/nginx/ssl/origin.key;
    ssl_client_certificate /opt/app/nginx/ssl/cloudflare-ca.pem;
    ssl_verify_client on;

    # ... proxy_pass 到后端 ...
}
```

**关键变更**：`listen` 绑定 `127.0.0.1`。`ssl_verify_client on` 强制 mTLS。非 CF 连接无法到达 NGINX。

### 4.5 SSH 经 Tunnel

**服务器端**：SSH 保持端口 22，Tunnel 验证通过后：
```ini
ListenAddress 127.0.0.1    # 仅经 Tunnel 可达
```

**客户端**（本机 `~/.ssh/config`）：
```
Host ny-server
  HostName ssh.example.com
  User <your_user>
  ProxyCommand cloudflared access ssh --hostname %h
  IdentityFile ~/.ssh/ny_vps_ed25519
```

在本机安装 `cloudflared`（Windows/macOS）。首次 `ssh ny-server` 打开浏览器进行 CF Access OAuth/OTP 验证，之后 SSH 经 Tunnel 连接。

### 4.6 关闭所有公网端口（最终切换）

验证所有服务经 Tunnel 正常后：

```bash
# NGINX：以 127.0.0.1-only 配置重启
sudo nginx -t && sudo systemctl restart nginx

# SSH：切换为仅本地
sudo sed -i 's/^ListenAddress.*/ListenAddress 127.0.0.1/' /etc/ssh/sshd_config
sudo systemctl restart sshd

# UFW：移除所有入站规则
sudo ufw delete allow from <YOUR_IP>/32 to any port 22
# 移除所有 CF IP 规则（循环）
sudo ufw default deny incoming
sudo ufw reload

# 验证：零公网端口
sudo ss -tlnp | grep -v 127.0.0.1
# 预期：仅 cloudflared 出站连接，无 0.0.0.0 监听
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

### 4.8 迁移顺序（每子域名约 5 分钟）

1. 验证当前服务正常（经旧 A 记录）
2. 添加 Tunnel public hostname（自动创建 CNAME，替换旧 A 记录）
3. 通过新 CNAME 测试（经 Tunnel）
4. 若故障：删除 Tunnel hostname，手动恢复旧 A 记录
5. 若正常：切换下一个子域名
6. 全部 5 个完成后：执行 4.6（关闭公网端口）

### 4.9 验证清单

- [ ] `systemctl status cloudflared` → active (running)
- [ ] CF Dashboard Tunnel `ny1` → HEALTHY
- [ ] 所有 5 个子域名解析到 CF IP（`dig blog.example.com +short` → 104.x/172.x）
- [ ] `ssh ny-server` 经 Tunnel 正常
- [ ] `curl -k https://localhost:443 -H "Host: blog.example.com"` 正常（mTLS）
- [ ] 直接 `curl https://<源站IP>` → 连接被拒绝（端口关闭）
- [ ] `ss -tlnp` → 无 0.0.0.0 监听
- [ ] `ufw status` → default deny incoming，零规则

---

## 五、阶段 3：R2 启用

**目标**：公开图片从 R2 → CDN → 用户（展示零源站负载）。零停机。

### 5.1 创建 R2 Bucket

CF Dashboard → R2 → Create bucket：
- 名称：`doujin-images`
- Public access：**Enabled**（Public bucket）
- Custom Domain：`cdn.example.com`（自动在 CF DNS 创建 CNAME）
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
    {"key": "r2_public_url", "value": "https://cdn.example.com"}
  ]
}
```

R2 服务热重载配置（调用 `reload()`），无需重启。

### 5.4 迁移现有图片（零停机）

`r2_service.go:165` 已实现 `MigrateLocalDir(basePath, subdir)`：
- 递归遍历 `/data/images/`
- 将每个文件上传到 R2，保持相同相对路径
- 逐文件记录日志
- 通过 admin 端点或一次性脚本执行
- **无服务中断** — 图片同时存在于本地磁盘和 R2

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
- `/i/abc123`（原图）→ 302 到 `cdn.example.com/abc123`（R2，边缘缓存 1 年）
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

最大化 CF 缓存命中率 — 首次请求后，已缓存图片零源站接触。

### 5.7 imgproxy + R2 共存

**过渡期**：原图保留在本地磁盘（imgproxy 读取用于格式转换）。R2 服务原图用于展示。无需修改 imgproxy 配置。

**未来优化**（不在本计划范围）：配置 imgproxy 使用 S3 源（`IMGPROXY_S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com`），使其直接从 R2 读取，之后可清理本地磁盘。

### 5.8 R2 费用估算

| 项目 | 用量 | 免费额度 | 计费量 | 月费 |
|------|------|---------|--------|------|
| 存储（Standard） | ~50GB | 10GB | 40GB | $0.60 |
| Class A 操作（上传） | <1万次/月 | 100万次 | 0 | $0 |
| Class B 操作（读取/未命中回源） | ~96万次/月 | 1000万次 | 0 | $0 |
| 出口流量 | ∞ | ∞ | 0 | $0 |
| **合计** | | | | **~$0.60/月** |

### 5.9 CF TOS 合规

R2 属于 Cloudflare Developer Platform。通过 R2 Custom Domain 经 CDN 服务图片是条款明确允许的方式（不同于免费版直接通过 CDN 代理服务大文件）。

### 5.10 验证清单

- [ ] `dig cdn.example.com +short` → CF IP
- [ ] `curl -I https://cdn.example.com/<测试图片>` → 200 from R2
- [ ] Admin 配置显示 `r2_enabled=true`
- [ ] `curl -I https://img.example.com/i/<公开链接>` → 302 → `cdn.example.com`
- [ ] `curl -I https://img.example.com/i/<公开链接>.webp` → 200（imgproxy，非 R2）
- [ ] `curl -I https://img.example.com/i/<私密链接>` → 无 token 时 403
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
| Rocket Loader | **Off** | 破坏 Halo/zfile 前端 JS |
| Mirage | **Off** | 可能干扰同人志图片显示 |

### 6.2 Cache Rules（免费 10 条）

| # | 规则名 | 条件 | Edge TTL | Browser TTL |
|---|--------|------|----------|-------------|
| 1 | Cache-CDN-R2 | `Hostname eq cdn.example.com` | 1 year | 1 year |
| 2 | Cache-Img-Display | `Hostname eq img.example.com AND URI Path starts with /i/` | 1 year | 1 year |
| 3 | Cache-Blog-Assets | `Hostname eq blog.example.com AND (URI Path ends with .css OR .js OR .woff2 OR .svg OR .png OR .jpg)` | 30 days | 30 days |
| 4 | Cache-Blog-HTML | `Hostname eq blog.example.com AND NOT URI Path starts with /console` | 1 hour | — |
| 5 | Cache-Drive-Static | `Hostname eq drive.example.com AND (URI Path ends with .css OR .js OR .woff2)` | 30 days | 30 days |
| 6 | Cache-ImgApp-Static | `Hostname eq img.example.com AND (URI Path ends with .css OR .js OR .woff2 OR .svg)` | 30 days | 30 days |
| 7 | Bypass-Img-API | `Hostname eq img.example.com AND URI Path starts with /api/` | Bypass | — |
| 8 | Bypass-Admin | `URI Path starts with /console OR /admin OR /dashboard` | Bypass | — |
| 9 | Bypass-Status | `Hostname eq status.example.com` | Bypass | — |
| 10 | Bypass-Drive-API | `Hostname eq drive.example.com AND (URI Path starts with /api OR /webdav)` | Bypass | — |

**Cache Key 优化**（规则 1-2）：对不使用 `?w=` `?h=` `?q=` 的图片忽略 query string，跨变体共享缓存。

### 6.3 Tiered Cache + Always Online

- **Tiered Cache**：**On**（免费）— 上层 CF 节点在回源前先查缓存，减少回源请求
- **Always Online**：**On**（免费）— 源站宕机时从 CF 缓存服务，用户看到旧但可用的内容

### 6.4 WAF Custom Rules（免费 5 条）

| # | 规则名 | 表达式 | 动作 |
|---|--------|--------|------|
| 1 | Block-Scanner-Paths | `(uri.path eq "/xmlrpc.php") or (uri.path eq "/wp-login.php") or (uri.path eq "/.env") or (uri.path eq "/.git/config") or (uri.path eq "/wp-admin/")` | Block |
| 2 | Block-Bad-Methods | `(method eq "PUT" or method eq "DELETE" or method eq "TRACE") and not uri.path starts with "/api/"` | Block |
| 3 | Block-Bad-UA | `(ua contains "semrush") or (ua contains "ahrefs") or (ua contains "mj12bot") or (ua contains "dotbot")` | Block |
| 4 | Challenge-High-Threat | `(cf.threat_score gt 10)` | Managed Challenge |
| 5 | Limit-Upload-Spam | `(http.host eq "img.example.com") and (uri.path starts with "/api/v1/images/") and (method eq "POST") and (cf.threat_score gt 5)` | Challenge |

**同时启用**：
- Bot Fight Mode：**On**（免费）
- Browser Integrity Check：**On**
- Cloudflare Managed Ruleset：**On**（免费部分）
- Exposed Credentials Check：**On**

### 6.5 Rate Limiting（1 条自定义 + 1 条 Dashboard 基础）

| 规则 | 表达式 | 速率 | 动作 |
|------|--------|------|------|
| Limit-Upload（自定义） | `Hostname eq img.example.com AND URI Path starts with /api/v1/images/` | 10 req/min | Block |
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

用 OAuth/OTP 保护后台面板，在到达源站前完成身份验证：

| Application | Domain | Session Duration |
|-------------|--------|------------------|
| Halo Admin | `blog.example.com/console/*` | 24h |
| summerain Admin | `img.example.com/admin/*` | 24h |
| zfile Admin | `drive.example.com/admin/*` | 24h |
| Kuma Dashboard | `status.example.com/dashboard/*` | 24h |

**Identity Provider**：One-time PIN（邮箱，零配置）或 GitHub OAuth（推荐）。

**Policy**：`Allow` 当 `Emails` 匹配你的邮箱。

### 6.8 Page Rules（免费 3 条，旧版 — 谨慎使用）

| # | URL pattern | 设置 |
|---|-------------|------|
| 1 | `*example.com/*console*` | Security Level: High |
| 2 | `cdn.example.com/*` | Cache Level: Cache Everything |
| 3 | 备用 | — |

> Cache Rules（6.2）在缓存方面取代 Page Rules。Page Rules 仅用于 Security Level。

### 6.9 Workers（免费 10万请求/天 — 有限使用）

站点约 26.9万请求/天。**不要全站用 Workers**（超免费额度）。

仅限以下场景：
- 自定义错误页（如 R2 404 → 品牌化"图片不存在"页面）
- 未来需要的缩略图 URL 重写

### 6.10 监控：Uptime Kuma（新增 Docker 容器）

```yaml
uptime-kuma:
  image: louislam/uptime-kuma:1
  container_name: uptime-kuma
  restart: unless-stopped
  ports:
    - "127.0.0.1:3001:3001"
  volumes:
    - kuma_data:/app/data
  networks:
    - app-proxy
  security_opt:
    - no-new-privileges:true
  cap_drop: ['ALL']
  cap_add: [CHOWN, SETGID, SETUID, DAC_OVERRIDE]
  mem_limit: 256m
```

**监控项**：
- `blog.example.com` — HTTP(s)，60s 间隔
- `img.example.com/health` — HTTP(s)，30s 间隔
- `img.example.com/api/v1/public/stats` — HTTP(s) JSON 校验，60s
- `cdn.example.com` — HTTP(s)，60s
- `drive.example.com` — HTTP(s)，60s
- `status.example.com` — HTTP(s)，60s
- Push monitor：后端健康（脚本推送到 Kuma）

**公开状态页**：`status.example.com`（Kuma 内置）
**通知渠道**：Telegram + 邮箱

### 6.11 Notifications

CF Dashboard → Notifications：
- Security Events → 邮箱
- Tunnel Disconnect → 邮箱 + Telegram
- Origin 5xx 激增 → 邮箱
- R2 操作错误 → 邮箱
- Weekly Analytics Summary → 邮箱

### 6.12 Pro Plan（$25/月）— 可选附加

以下是 Pro 解锁的与本站相关的功能：

| 功能 | 收益 | 是否值得 |
|------|------|---------|
| **Image Resizing** | CF 边缘转换图片（imgproxy 替代） | 可考虑 — 消除 imgproxy 容器 |
| **Polish** | 边缘无损/有损优化 | 是 — 为中国用户改善图片压缩 |
| **更多 WAF 规则** | 20 条自定义（vs 免费 5 条） | 否 — 5 条足够 |
| **Web Analytics** | 更长留存，隐私优先 | 可考虑 — 更好的可见性 |
| **Argo Smart Routing** | 优化 CF 内部路由 | 否 — 仅 10-30ms 改善，额外 $5/月 |

**建议**：先免费。若 Polish/Image Resizing 对图片密集站点证明有价值，再考虑 Pro。

### 6.13 验证清单

- [ ] CF Analytics 缓存率趋向 99%+
- [ ] `curl -I https://blog.example.com` → `cf-cache: HIT`
- [ ] `curl -I https://cdn.example.com/<image>` → `cf-cache: HIT`，`alt-svc: h3=":443"`
- [ ] WAF Security 标签显示已拦截扫描器尝试
- [ ] `https://blog.example.com/console` → 重定向到 CF Access 登录
- [ ] `https://status.example.com` → Uptime Kuma 面板
- [ ] 所有监控项在 Kuma 中为绿色
- [ ] HTTP/3 验证：`curl --http3 -I https://blog.example.com`

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

仅需迁移：**MySQL 数据、Redis 数据、Halo volumes、zfile 数据、本地图片副本（供 imgproxy）**。

### 7.2 迁移顺序（约 2-4 小时计划停机）

1. ** provision 新 VPS**（推荐日本 — Vultr/Linode/DigitalOcean 东京）
2. **安装基础**：Docker、NGINX、cloudflared、UFW、SSH 加固（同阶段 1-2）
3. **在新 VPS 上恢复数据**：
   - MySQL：`mysql < dump.sql`（从 R2 备份）
   - Redis：恢复 RDB 快照
   - Docker volumes：`tar xf halo.tar -C /var/lib/docker/volumes/`
   - 图片文件：`r2_service.Download()` 或 `aws s3 sync` 从 R2 到本地磁盘
4. **在新 VPS 上启动所有容器**，本地验证（`curl localhost:8080/health`）
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

### 7.4 备份策略（阶段 5 前置 + 通用灾难恢复）

**每日自动备份到 R2 `backups` bucket**（私有，30 天 lifecycle）：

| 数据 | 方法 | 频率 | 预估大小 |
|------|------|------|---------|
| MySQL | `mysqldump --single-transaction` → gzip → R2 | 每日 4AM | ~100MB |
| Redis | `redis-cli BGSAVE` → 复制 RDB → R2 | 每日 4AM | ~10MB |
| Halo volume | `tar czf` → R2 | 每日 4AM | ~50MB |
| zfile 数据 | `tar czf` → R2 | 每日 4AM | ~500MB |
| 本地图片 | 已在 R2（阶段 3 迁移） | 持续 | — |
| R2 图片 | R2 versioning 启用（自保护） | — | — |

**备份脚本**：cron job → dump → gzip → `rclone` 或 `aws s3 cp` 到 R2 → 清理旧备份。

**恢复演练**：每月测试恢复以验证备份完整性。

### 7.5 迁移后优化（日本 VPS）

- `net.ipv4.tcp_congestion_control = bbr`（BBR 在日本→中国路由上表现好）
- 无需降低 TTL（DNS 经 Tunnel，无基于 IP 的记录）
- 通过 Uptime Kuma 监控延迟改善（对比 NY vs JP 响应时间）

### 7.6 验证清单（阶段 5）

- [ ] 新 VPS：所有容器运行，健康检查通过
- [ ] `curl --resolve blog.example.com:443:127.0.0.1 https://blog.example.com` 在新 VPS 上正常
- [ ] Tunnel 从新 IP 重连（CF Dashboard → HEALTHY）
- [ ] 所有服务从中国可访问（经 17ce.com 或 chinaz ping 测试）
- [ ] Uptime Kuma 显示改善的响应时间
- [ ] 旧 VPS 观察 48 小时后退役
- [ ] 备份恢复演练完成

---

## 八、完整计划汇总

| 阶段 | 重点 | 停机时间 | 关键交付物 | 预估时间 |
|------|------|---------|-----------|---------|
| 1 | 安全加固 | 无 | Docker 隔离、UFW、SSH、审计修复、内核 | ~2h |
| 2 | Tunnel 迁移 | 每子域名短暂 | cloudflared、mTLS、SSH 经 Tunnel、0 公网端口 | ~3h |
| 3 | R2 启用 | 无 | R2 bucket、图片迁移、缓存头修复 | ~2h |
| 4 | CF 控制台优化 | 无 | 10 条 Cache Rules、5 条 WAF、Zero Trust、Uptime Kuma、速度 | ~2h |
| 5 | VPS 迁移（未来） | ~30s（Tunnel 切换） | 日本 VPS、备份/恢复、数据迁移 | ~4h |

**最终状态**：零公网入站端口、mTLS 强制源站、R2 服务图片、99%+ 缓存率、WAF + Zero Trust 保护、HTTP/3 + Brotli + Early Hints 为中国用户、~80-200ms 延迟（阶段 5 后）。

---

## 九、风险与注意事项

### 9.1 阶段间依赖

- 阶段 2 依赖阶段 1 的 UFW 配置（需先限制 CF IP，再切换到 Tunnel）
- 阶段 3 可独立于阶段 2 执行（R2 启用不依赖 Tunnel）
- 阶段 4 依赖阶段 2 完成（Zero Trust Access 需要 Tunnel public hostname）
- 阶段 5 依赖阶段 1-4 全部完成

### 9.2 关键风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Tunnel 断连 | 所有服务不可用 | cloudflared 自动重连；Always Online 服务缓存内容 |
| R2 配置错误 | 公开图 404 | 阶段 3 先在测试图片上验证，再全量迁移 |
| mTLS 证书过期 | CF 无法到源站 | Origin CA 证书 15 年有效期；设置日历提醒 |
| 迁移期间数据丢失 | MySQL/Redis 数据不一致 | 迁移前完整备份；旧 VPS 保留 48 小时 |

### 9.3 不在本计划范围

- 前端代码修改（除 `public_handler.go` 两处小修复）
- Halo/zfile/summerain 应用本身的配置
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
- [Uptime Kuma](https://github.com/louislam/uptime-kuma)
