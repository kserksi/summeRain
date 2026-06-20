# 水印功能修复部署文档

- **日期**：2026-06-20
- **状态**：待实施
- **方案**：SVG 水印图片 + imgproxy 原生合成 + 手动重启
- **影响范围**：后端 Go 代码、服务器 docker-compose 配置

---

## 一、背景与根因

### 问题现象

管理员在后台启用水印后，上传和浏览的图片均不显示水印。

### 根因分析（三层叠加 Bug）

| # | 问题 | 详情 |
|---|---|---|
| 1 | imgproxy URL 未签名 | 后端 `ImgproxyService` 使用 `/insecure/` 前缀，但 imgproxy 配置了 `IMGPROXY_KEY`/`IMGPROXY_SALT`，导致所有 imgproxy 请求返回 **403 Invalid signature**。缩略图和水印图均无法生成。 |
| 2 | 水印选项名与 imgproxy v4 不匹配 | 代码使用 `wm.o:` / `wm.g:` / `wm.fs:` / `wm.cl:`，这些选项在 imgproxy v4 中不存在。 |
| 3 | `wmt:` 文本水印是 PRO 功能 | imgproxy 社区版 v4.0.5 不支持 `wmt:`（文本水印），返回 **404 Unknown processing option wmt**。社区版仅支持通过 `IMGPROXY_WATERMARK_PATH` 配置一张全局水印图片，再用 `wm:` 选项应用。 |

### Bug 1 和 Bug 2 已修复状态

Bug 1（签名）和 Bug 2（选项名）已在当前部署版本中修复。`imgproxy_service.go` 已添加 `signPath()` 方法实现 HMAC-SHA256 签名。

### 本文档解决的方案

针对 Bug 3（PRO 功能限制），采用 **SVG 水印图片 + imgproxy 原生合成** 方案：

- 后端根据水印配置（文字、颜色、字号）生成一张 SVG 图片
- SVG 保存在 imgproxy 可读的共享 Docker 卷中
- imgproxy 通过 `IMGPROXY_WATERMARK_PATH` 加载 SVG 作为全局水印
- 上传和提供图片时，imgproxy URL 中包含 `wm:opacity:position` 启用水印
- 水印配置变更后，管理员手动重启 imgproxy 使新 SVG 生效

---

## 二、方案架构

```
管理员保存水印配置（前端 Configs 页面）
     │
     ▼
AdminService.UpdateConfigs()
     ├── 1. 保存配置到 MySQL (system_configs 表)
     ├── 2. 读取水印配置 → 生成 SVG 文字图片
     ├── 3. 写入 /data/images/watermark.svg（共享 Docker 卷 image_storage）
     ├── 4. 清除 Redis 缓存（key: "wm:config"）
     └── 5. 返回 { restart_needed: true }
                                   │
                    前端显示提示："请重启 imgproxy 生效"
                                   │
                    管理员 SSH → docker restart summerain-imgproxy
                                   │
                     imgproxy 重新加载 IMGPROXY_WATERMARK_PATH → 水印生效
                                   │
     ┌─────────────────────────────┴───────────────┐
     ▼                                              ▼
上传时 (image_service.go ProcessedURL)        提供时 (public_handler.go ServeImage)
imgproxy 签名 URL 包含 /wm:0.7:ce           imgproxy 签名 URL 包含 /wm:0.7:ce
     │                                              │
     ▼                                              ▼
     imgproxy 用全局 SVG 水印合成 → 输出 webp/avif（保持原格式）
```

### 关键设计决策

| 决策 | 原因 |
|---|---|
| SVG 格式而非 PNG | 纯文本生成，无需 Go 图像库；矢量缩放无损 |
| `IMGPROXY_WATERMARK_PATH` 而非 `wmt:` | 社区版支持，非 PRO 功能 |
| 手动重启而非自动重启 | 不挂载 Docker Socket，避免 root 级安全风险 |
| SVG 存储在 Docker 持久卷 | 容器重启后 SVG 不丢失，仅首次部署需手动重启一次 |
| 水印配置 Redis 缓存 60s | 避免每次图片请求都查 DB |

---

## 三、前置条件

### 3.1 已完成（当前部署版本）

- [x] Bug 1 修复：`imgproxy_service.go` 的 `signPath()` 签名方法
- [x] Bug 2 修复：`image_service.go:467-475` 的 `os.IsNotExist` 防护
- [x] 孤儿容器 `stoic_vaughan` 已清理

### 3.2 代码安全前置条件（详见 `docs/security-audit.md`）

水印部署前，以下 **代码安全问题应先修复**：

| 编号 | 问题 | 位置 | 修复方式 |
|---|---|---|---|
| SEC-C1 | `/public/wm-test` 调试端点无认证泄露全部配置 | `main.go:138-165` | 删除该端点 |
| SEC-H1 | 认证接口 `err.Error()` 泄露内部细节 | `auth_handler.go:31,51,113,153` | 替换为固定文案 |
| SEC-M1 | GIF 扩展名/MIME 校验不一致 | `image_service.go:192` vs `370` | 统一白名单 |

修复 SEC-C1 和 SEC-H1 可与水印代码变更合并在同一次部署中。

### 3.3 待确认

| 确认项 | 当前值 | 说明 |
|---|---|---|
| imgproxy 版本 | v4.0.5 | 社区版，不支持 `wmt:` |
| imgproxy KEY | `8ddf4ab3...` | 已配置，URL 必须签名 |
| imgproxy SALT | `f237c63b...` | 已配置，URL 必须签名 |
| Docker 卷 image_storage | 挂载于 backend (rw) 和 imgproxy (ro) | SVG 由 backend 写入，imgproxy 读取 |
| 后端部署目录 | `/opt/summerain/` | 含 docker-compose.yml、server 二进制 |
| SSH 别名 | `kserks` | 通过 ssh-skill 操作 |
| DB 水印配置 | enabled=true, text=kserks, opacity=0.7, position=ce, size=64, color=66ccff | 已在 system_configs 表中 |

### 3.4 验证 `wm:` 选项可用性（部署前必做）

在实施代码变更前，先手动验证 imgproxy 社区版支持 `wm:` 选项 + `IMGPROXY_WATERMARK_PATH`：

```bash
# 1. 在服务器上创建测试 SVG
ssh kserks 'cat > /tmp/test_wm.svg << '\''EOF'\''
<svg xmlns="http://www.w3.org/2000/svg" width="200" height="80">
  <text x="100" y="40" text-anchor="middle" dominant-baseline="central"
        font-family="sans-serif" font-size="32" fill="#66ccff">TEST</text>
</svg>
EOF'

# 2. 将 SVG 放入共享卷
ssh kserks 'docker cp /tmp/test_wm.svg summerain-backend:/data/images/watermark.svg'

# 3. 在 docker-compose.yml 中为 imgproxy 添加环境变量
#    IMGPROXY_WATERMARK_PATH=/data/images/watermark.svg
#    （编辑 /opt/summerain/docker-compose.yml）

# 4. 重启 imgproxy
ssh kserks 'cd /opt/summerain && docker compose up -d imgproxy'

# 5. 生成签名 URL 并测试
#    使用 Python 脚本生成 wm:0.7:ce 格式的签名 URL
#    确认 imgproxy 返回 200 而非 404

# 6. 确认无误后，清理测试文件
ssh kserks 'docker exec summerain-backend rm /data/images/watermark.svg'
```

如果 V2 验证失败（`wm:` 不被接受），需要回退到混合方案（Go 层水印）。

---

## 四、代码变更清单

### 4.1 新增文件

#### `internal/service/watermark.go`

SVG 水印生成器，纯字符串拼接，无外部依赖。

**功能**：
- `GenerateWatermarkSVG(text, color, size string) string` — 根据文字、颜色、字号生成 SVG XML
- `SaveWatermarkFile(svg, storageBasePath string) error` — 将 SVG 写入 `{storageBasePath}/watermark.svg`
- `RegenerateWatermark(cfgMap map[string]string, storageBasePath string) (changed bool, err error)` — 从 DB 配置 map 生成并保存，返回是否有变更

**SVG 模板**：
```xml
<svg xmlns="http://www.w3.org/2000/svg" width="{WIDTH}" height="{HEIGHT}">
  <text x="50%" y="50%" text-anchor="middle" dominant-baseline="central"
        font-family="sans-serif" font-size="{SIZE}"
        fill="#{COLOR}">{ESCAPED_TEXT}</text>
</svg>
```

**尺寸估算**：
- 宽度 = `len(text) * fontSize * 0.6`（通用字符宽度系数）
- 高度 = `fontSize * 1.25`

**XML 转义**：文字中的 `<`、`>`、`&`、`"`、`'` 需转义。

#### `internal/service/watermark_test.go`

测试用例：
- `TestGenerateWatermarkSVG` — 验证 SVG 结构、尺寸、颜色、转义
- `TestSaveWatermarkFile` — 验证文件写入
- `TestRegenerateWatermark` — 验证启用/禁用/变更检测

### 4.2 修改文件

#### `internal/service/imgproxy_service.go`

**ProcessedURL 方法**：去掉 `wmt:`/Pango markup/Base64 编码，改为纯 `wm:` 选项。

```go
// 变更前（当前代码，社区版不支持 wmt）:
if text != "" {
    pangoText := buildWatermarkText(text, watermarkColor, watermarkSize)
    encoded := base64.RawURLEncoding.EncodeToString([]byte(pangoText))
    path += fmt.Sprintf("/wmt:%s", encoded)
    path += fmt.Sprintf("/wm:%s:%s:0:0:0", watermarkOpacity, watermarkPosition)
}

// 变更后:
path += fmt.Sprintf("/wm:%s:%s", watermarkOpacity, watermarkPosition)
```

**删除**：`buildWatermarkText` 函数、`encoding/base64` 和 `strconv` 中仅服务于 Pango 的引用（如果没有其他使用）。

**保留**：`signPath()` 方法（Bug 1 修复）、`sanitizeWatermarkText()` 函数（仍用于验证文字内容）、`validOpacity()` 函数。

#### `internal/service/public_config_service.go`

**新增**：

```go
type WatermarkConfig struct {
    Enabled  bool   `json:"enabled"`
    Opacity  string `json:"opacity"`
    Position string `json:"position"`
}
```

**新增方法**：`GetWatermark() (*WatermarkConfig, error)`
- 先查 Redis（key: `wm:config`，TTL 60s）
- miss 时从 `configReader.FindAll()` 提取 `watermark_enabled`、`watermark_opacity`、`watermark_position`
- 写入 Redis 并返回

**结构体变更**：`PublicConfigService` 添加 `rdb *redis.Client` 字段。
**构造函数变更**：`NewPublicConfigService` 添加 `rdb` 参数。

#### `internal/handler/public_handler.go`

**ServeImage 方法**（约 line 148-161，构造 imgproxy path 之后、签名之前）：

新增水印选项追加逻辑：

```go
// 在现有 path 构造之后:
if format != "" {  // format=="" 是原图下载，不加
    wm, _ := h.publicConfigService.GetWatermark()
    if wm != nil && wm.Enabled {
        path += fmt.Sprintf("/wm:%s:%s", wm.Opacity, wm.Position)
    }
}

// 然后是现有的签名逻辑:
signedPath := h.signer.SignPath(path)
```

#### `internal/service/admin_service.go`

**结构体变更**：添加 `storageCfg *config.StorageConfig` 和 `rdb *redis.Client` 字段。
**构造函数变更**：`NewAdminService` 添加对应参数。

**UpdateConfigs 方法变更**：

```go
// 返回值从 *errcode.AppError 改为 (*ConfigUpdateResult, *errcode.AppError)
type ConfigUpdateResult struct {
    RestartNeeded bool `json:"restart_needed"`
}

func (s *AdminService) UpdateConfigs(items []ConfigUpdateItem) (*ConfigUpdateResult, *errcode.AppError) {
    // ... 现有的 DB 保存逻辑 ...

    result := &ConfigUpdateResult{}

    // 检查是否包含水印配置
    for _, item := range items {
        if strings.HasPrefix(item.Key, "watermark_") {
            // 重新生成 SVG
            configs, _ := s.configRepo.FindAll()
            cfgMap := configListToMap(configs)
            watermark.RegenerateWatermark(cfgMap, s.storageCfg.BasePath)

            // 清除 Redis 缓存
            s.rdb.Del(context.Background(), "wm:config")

            result.RestartNeeded = true
            break
        }
    }
    return result, nil
}
```

#### `internal/handler/admin_handler.go`

**UpdateConfigs 方法**：将返回值传递给前端。

```go
// 变更前:
if appErr := h.adminService.UpdateConfigs(req.Items); appErr != nil {
    response.Error(c, appErr)
    return
}
response.Success(c, nil)

// 变更后:
result, appErr := h.adminService.UpdateConfigs(req.Items)
if appErr != nil {
    response.Error(c, appErr)
    return
}
response.Success(c, result)
```

#### `cmd/server/main.go`

**启动时生成 SVG**（在所有服务初始化之后、`r.Run()` 之前）：

```go
configs, _ := configRepo.FindAll()
cfgMap := configListToMap(configs)
if cfgMap["watermark_enabled"] == "true" {
    changed, err := watermark.RegenerateWatermark(cfgMap, cfg.Storage.BasePath)
    if err != nil {
        log.Printf("[WATERMARK] failed to generate SVG: %v", err)
    } else if changed {
        log.Println("[WATERMARK] SVG generated. Restart imgproxy to apply.")
    }
}
```

**依赖注入调整**：
- `NewAdminService` 调用添加 `&cfg.Storage` 和 `rdb`
- `NewPublicConfigService` 调用添加 `rdb`

### 4.3 不改动的文件

| 文件 | 原因 |
|---|---|
| 前端 `Configs.tsx` | UI 不变，管理员配置流程不变 |
| `docker-compose.yml`（backend 部分） | 不挂载 Docker Socket |
| imgproxy Dockerfile | 不变 |
| backend Dockerfile | 不变 |
| `internal/worker/cleanup.go` | 与水印无关 |

---

## 五、服务器配置变更

### 5.1 docker-compose.yml（imgproxy 服务）

编辑 `/opt/summerain/docker-compose.yml`，在 `imgproxy` 服务的 `environment` 中添加：

```yaml
imgproxy:
    image: darthsim/imgproxy:v4.0.5
    container_name: summerain-imgproxy
    restart: unless-stopped
    environment:
      - IMGPROXY_BIND=:8080
      - IMGPROXY_LOCAL_FILESYSTEM_ROOT=/data
      - IMGPROXY_MAX_SRC_RESOLUTION=50
      - IMGPROXY_MAX_ANIMATION_FRAMES=100
      - IMGPROXY_TTL=31536000
      - IMGPROXY_KEY=${IMGPROXY_KEY}
      - IMGPROXY_SALT=${IMGPROXY_SALT}
      - IMGPROXY_FONT=/fonts/GoogleSans-ASCII.ttf
      - IMGPROXY_WATERMARK_PATH=/data/images/watermark.svg        # ← 新增
    volumes:
      - image_storage:/data/images:ro
      - temp_storage:/data/temp:ro
      - /opt/summerain/fonts/GoogleSans-ASCII.ttf:/fonts/GoogleSans-ASCII.ttf:ro
    # ... 其余不变
```

### 5.2 .env 文件

无需变更。水印配置在 MySQL `system_configs` 表中，不在 `.env` 中。

### 5.3 前端提示文案（可选优化）

前端 `Configs.tsx` 的保存回调中，可根据 API 返回的 `restart_needed` 字段显示提示。此为可选优化，不阻塞部署。

---

## 六、构建与部署步骤

### 步骤 1：实施代码变更

按照第四节完成所有代码变更。确保：
- `go build ./...` 通过
- `go test ./internal/service/... ./internal/handler/...` 通过
- `go vet ./...` 无警告

### 步骤 2：本地交叉编译

```powershell
cd D:\book\backend
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o server_deploy ./cmd/server
```

验证：`server_deploy` 文件大小约 19MB。

### 步骤 3：备份当前版本

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker tag summerain:latest summerain:rollback && \
   cp /opt/summerain/server /opt/summerain/server.prev && \
   cp /opt/summerain/docker-compose.yml /opt/summerain/docker-compose.yml.prev && \
   echo 'BACKUP DONE'"
```

### 步骤 4：上传新二进制

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_upload.py kserks \
  "D:\book\backend\server_deploy" "/opt/summerain/server.new"
```

### 步骤 5：更新 docker-compose.yml

在服务器上编辑 `/opt/summerain/docker-compose.yml`，为 imgproxy 添加 `IMGPROXY_WATERMARK_PATH` 环境变量（见第五节 5.1）。

可通过 SSH 直接编辑，或上传修改后的文件。

### 步骤 6：替换二进制 + 重建镜像 + 重启

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "mv /opt/summerain/server.new /opt/summerain/server && \
   chmod +x /opt/summerain/server && \
   cd /opt/summerain && \
   docker compose build backend && \
   docker compose up -d && \
   echo 'DEPLOY DONE'"
```

注意：`docker compose up -d` 会重启所有有变更的服务（backend 和 imgproxy 都会重启）。

### 步骤 7：验证启动

```bash
# 等待 10 秒让服务启动
sleep 10

# 检查所有容器状态
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker ps --format 'table {{.Names}}\t{{.Status}}' | grep summerain"

# 检查 backend 日志（确认 SVG 生成）
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker logs --tail 20 summerain-backend 2>&1 | grep -i watermark"

# 检查 imgproxy 日志（确认无启动错误）
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker logs --tail 10 summerain-imgproxy 2>&1"

# 健康检查
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker exec summerain-backend wget -q -O- http://localhost:8080/health"
```

预期结果：
- 所有容器 Up (healthy)
- backend 日志包含 `[WATERMARK] SVG generated`
- imgproxy 日志无错误
- health 返回 `{"status":"ok"}`

### 步骤 8：验证 SVG 文件存在

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker exec summerain-backend cat /data/images/watermark.svg"
```

预期：输出包含 `<svg` 标签和配置的文字内容。

### 步骤 9：清理本地编译产物

```powershell
Remove-Item D:\book\backend\server_deploy -Force
```

---

## 七、部署后验证

### 7.1 水印端到端测试

通过前端上传一张测试图片，验证水印可见：

1. 打开前端 `https://localhost:5173/`
2. 登录管理员账户
3. 上传一张测试图片
4. 查看图片详情页
5. 确认图片上显示水印文字 "kserks"，颜色 #66ccff，位置居中

### 7.2 imgproxy 日志验证

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker logs --tail 20 summerain-imgproxy 2>&1 | grep wm:"
```

预期：日志中包含 `/wm:0.7:ce` 路径，状态码 200。

### 7.3 缩略图验证

缩略图不应包含水印（设计决策）。验证：

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "docker logs --tail 30 summerain-imgproxy 2>&1 | grep 'rs:fill'"
```

确认缩略图请求（含 `rs:fill:300:300`）不包含 `wm:`。

### 7.4 水印关闭回归测试

1. 在管理员配置页关闭水印
2. 上传新图片
3. 确认新图片无水印
4. 重新开启水印

### 7.5 原图下载验证

直接访问原图（format 为空）应无水印：

```
访问 /i/{link}（不带 ?f=webp 参数）→ 返回原始文件，无水印
访问 /i/{link}?f=webp → 返回 imgproxy 处理的 webp，有水印
```

---

## 八、回滚方案

### 场景 A：代码问题需回滚

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "cd /opt/summerain && \
   docker tag summerain:rollback summerain:latest && \
   cp server.prev server && \
   docker compose build backend && \
   docker compose up -d backend"
```

同时恢复 docker-compose.yml：
```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "cp /opt/summerain/docker-compose.yml.prev /opt/summerain/docker-compose.yml && \
   cd /opt/summerain && docker compose up -d imgproxy"
```

### 场景 B：仅 imgproxy 水印配置问题

移除 docker-compose.yml 中的 `IMGPROXY_WATERMARK_PATH`，重启 imgproxy：

```bash
python ~/.claude/skills/ssh-skill/scripts/ssh_execute.py kserks \
  "cd /opt/summerain && \
   sed -i '/IMGPROXY_WATERMARK_PATH/d' docker-compose.yml && \
   docker compose up -d imgproxy"
```

此操作后水印不生效，但图片正常显示（无水印）。

---

## 九、运维操作手册

### 9.1 修改水印配置

管理员在前端 **管理后台 → 配置** 页面修改水印设置后：

1. 后端自动生成新 SVG 并保存
2. 前端显示提示：*"水印配置已更新，请重启 imgproxy 生效"*
3. 管理员 SSH 到服务器执行：

```bash
ssh kserks 'docker restart summerain-imgproxy'
```

4. 等待约 5 秒，新水印配置生效

**生效范围**：
- ✅ 后续所有图片请求（提供时）的水印更新
- ✅ 后续上传图片（上传时处理）的水印更新
- ❌ 已上传图片的已生成缩略图/处理图不受影响（但提供时会动态应用新水印）

### 9.2 关闭水印

1. 管理员在配置页关闭水印开关
2. 后端清除 Redis 缓存
3. **无需重启 imgproxy**（URL 中不含 `wm:` 即可）
4. 后续图片请求无水印

### 9.3 重新开启水印

1. 管理员在配置页开启水印
2. 后端重新生成 SVG
3. 管理员重启 imgproxy（如果 SVG 内容有变化）
4. 后续图片请求有水印

### 9.4 检查水印状态

```bash
# 检查 SVG 文件是否存在
ssh kserks 'docker exec summerain-backend ls -la /data/images/watermark.svg'

# 检查 SVG 内容
ssh kserks 'docker exec summerain-backend cat /data/images/watermark.svg'

# 检查 imgproxy 是否加载了水印
ssh kserks 'docker logs --tail 5 summerain-imgproxy 2>&1'

# 检查 DB 水印配置
ssh kserks 'docker exec summerain-mysql mysql -u root -proot123456 image_gallery -e "SELECT * FROM system_configs WHERE config_key LIKE \"%watermark%\";"'
```

### 9.5 首次部署注意事项

由于 docker-compose 中 backend `depends_on` imgproxy（healthy），首次部署时：

1. imgproxy 先启动 → 此时 SVG 尚不存在 → imgproxy 正常运行但无水印
2. backend 启动 → 生成 SVG → 日志打印 `[WATERMARK] SVG generated`
3. **需要手动重启 imgproxy 一次**使其加载 SVG：

```bash
ssh kserks 'docker restart summerain-imgproxy'
```

后续容器重启不需要此操作（SVG 持久化在 Docker 卷中）。

---

## 十、已知限制

| 限制 | 说明 | 缓解方案 |
|---|---|---|
| SVG `<text>` 字体取决于容器环境 | imgproxy 使用 librsvg 渲染 SVG，字体为容器内 `sans-serif` | 可接受；如需特定字体，可将 TTF 安装到 imgproxy 容器的 `/usr/share/fonts/` |
| 水印配置变更需手动重启 imgproxy | `IMGPROXY_WATERMARK_PATH` 在启动时读取 | 管理员操作；后续可考虑添加自动重启（Docker Socket 或 webhook） |
| 水印文字宽度为估算 | `len(text) * fontSize * 0.6` 是近似值 | 对水印场景足够精确；如需精确可改为固定宽度 |
| 所有图片共享同一水印 | `IMGPROXY_WATERMARK_PATH` 是全局配置 | 符合系统设计预期 |
| 水印不应用于缩略图 | 缩略图 URL 不含 `wm:` | 设计决策：缩略图太小不适合水印 |
| 水印不应用于原图下载 | format=="" 时直接返回文件，不经 imgproxy | 设计决策：原图保留无水印版本 |

---

## 附录 A：相关文件索引

| 文件 | 路径 | 说明 |
|---|---|---|
| imgproxy 服务 | `internal/service/imgproxy_service.go` | URL 构造与签名 |
| 图片上传处理 | `internal/service/image_service.go` | 上传时生成缩略图和处理图 |
| 图片提供服务 | `internal/handler/public_handler.go` | ServeImage 动态处理 |
| 水印生成 | `internal/service/watermark.go`（新增） | SVG 生成 |
| 管理员配置 | `internal/service/admin_service.go` | UpdateConfigs |
| 公共配置 | `internal/service/public_config_service.go` | GetWatermark |
| 后端入口 | `cmd/server/main.go` | 启动初始化 |
| 服务器配置 | `/opt/summerain/docker-compose.yml` | imgproxy 环境变量 |
| 前端配置页 | `frontend/src/features/admin/pages/Configs.tsx` | 管理员 UI |

## 附录 B：DB 水印配置键值

| config_key | 示例值 | 说明 |
|---|---|---|
| `watermark_enabled` | `true` / `false` | 是否启用水印 |
| `watermark_text` | `kserks` | 水印文字内容 |
| `watermark_opacity` | `0.7` | 不透明度（0.0-1.0） |
| `watermark_position` | `ce` | 位置：ce/soea/sowe/noea/nowe/no/so/ea/we |
| `watermark_size` | `64` | 字号（px，用于 SVG font-size） |
| `watermark_color` | `66ccff` | 颜色（十六进制，不含 #） |
