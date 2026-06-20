# 前后端代码安全审查报告

- **日期**：2026-06-20
- **审查范围**：前端 `frontend/src/` + 后端 `backend/` 源代码（不含服务器部署配置）
- **审查方法**：源码静态分析 + 模式匹配 + 数据流追踪

---

## 一、发现汇总

| 严重级别 | 数量 | 说明 |
|---|---|---|
| **严重** | 1 | 调试端点泄露全部配置 |
| **高危** | 1 | 认证错误信息泄露 |
| **中危** | 3 | GIF 不一致、LIKE 注入、大小限制不一致 |
| **低危** | 1 | SQL 字符串拼接 |
| **已具备** | 20+ | 现有安全措施确认有效 |

---

## 二、严重（Critical）

### SEC-C1：调试端点泄露全部系统配置

**位置**：`cmd/server/main.go:138-165`

```go
api.GET("/public/wm-test", func(c *gin.Context) {
    configs, _ := configRepo.FindAll()       // ← 读取全部配置
    cfgMap := make(map[string]string)
    for _, cfg := range configs {
        cfgMap[cfg.ConfigKey] = cfg.ConfigValue
    }
    // ...
    result := gin.H{
        "config_from_db": gin.H{
            "all_configs": cfgMap,            // ← 全量返回给客户端
            // ...
        },
        "imgproxy_base_url": cfg.Imgproxy.BaseURL,
        "generated_url":     processedURL,     // ← 含签名的 imgproxy URL
    }
    c.JSON(200, gin.H{"code": 0, "message": "success", "data": result})
})
```

**风险**：
- 路由 `/api/v1/public/wm-test` 无需认证（public 路由组）
- `configRepo.FindAll()` 返回 `system_configs` 表全部记录，包含所有水印配置、captcha 密钥等
- 响应中直接暴露 `all_configs` 全量字段
- 同时暴露 imgproxy 签名 URL，泄露签名算法的实际输出格式
- 任何未认证用户可访问 `https://dev.kserks.org/api/v1/public/wm-test`

**修复**：删除此调试端点，或添加管理员认证 + 移除 `all_configs` 字段。

---

## 三、高危（High）

### SEC-H1：认证接口泄露内部错误细节

**位置**：`internal/handler/auth_handler.go:31,51,113,153`

```go
// 4 处相同模式：
if err := c.ShouldBindJSON(&input); err != nil {
    response.Error(c, errcode.New(3000, err.Error(), 400))  // ← err.Error() 直接返回
    return
}
```

**风险**：
- `err.Error()` 包含 Go reflect 的字段名、类型信息（如 `"username" failed`、`"time.Time" failed`）
- 帮助攻击者了解请求体内部结构（字段名、验证规则）
- 虽然当前仅用于 JSON binding 错误（风险有限），但模式不安全

**修复**：
```go
if err := c.ShouldBindJSON(&input); err != nil {
    response.Error(c, errcode.New(3000, "请求参数无效", 400))
    return
}
```

---

## 四、中危（Medium）

### SEC-M1：GIF 扩展名与 MIME 校验不一致

**位置**：
- `image_service.go:192` — 扩展名白名单包含 `.gif`
- `image_service.go:370-377` — MIME 白名单 **不包含** `image/gif`

```go
// 扩展名检查 — 包含 gif
allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}

// MIME 检查 — 不包含 gif
func allowedImageMIME(mimeType string) bool {
    allowedMIME := map[string]bool{
        "image/png":  true,
        "image/jpeg": true,
        "image/webp": true,
    }
    return allowedMIME[mimeType]
}
```

**影响**：用户上传 `.gif` 文件时，扩展名检查通过（line 192），但 MIME 检查失败（line 215），返回 "文件内容与扩展名不匹配"。行为上 GIF 被拒绝，但错误信息误导用户。

**修复**：在 `allowedImageMIME` 中添加 `"image/gif": true`，或从扩展名白名单中移除 `.gif`。

### SEC-M2：搜索 LIKE 通配符注入

**位置**：`internal/repository/image_repo.go:47-48`

```go
if search != "" {
    query = query.Where("filename LIKE ? OR description LIKE ?", "%"+search+"%", "%"+search+"%")
}
```

**风险**：
- 使用参数化查询（`?` 占位符），SQL 注入风险已消除 ✅
- 但用户输入中的 `%` 和 `_` 是 LIKE 特殊字符，未转义
- 攻击者可输入 `_%` 匹配所有记录，或构造特定模式进行数据探测
- 影响较低（仅影响搜索结果准确性，不泄露额外数据）

**修复**：转义 LIKE 特殊字符：
```go
if search != "" {
    escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(search)
    query = query.Where("filename LIKE ? ESCAPE '\\\\' OR description LIKE ? ESCAPE '\\\\'",
        "%"+escaped+"%", "%"+escaped+"%")
}
```

### SEC-M3：文件大小限制不一致

**位置**：
- `image_service.go:185` — 应用层限制 10MB
- nginx 配置 — `client_max_body_size 20m`

**影响**：nginx 允许 20MB 请求到达后端，后端拒绝 >10MB 的文件。多余的 10MB 带宽被浪费在传输最终被拒绝的请求上。

**修复**：统一为同一值（建议 nginx 也设为 10MB，或后端提升至 20MB）。

---

## 五、低危（Low）

### SEC-L1：migrate.go SQL 列名字符串拼接

**位置**：`internal/repository/migrate.go:26`

```go
db.Exec("ALTER TABLE image_access_tokens DROP COLUMN " + col)
```

**风险**：`col` 来源于硬编码的列名列表（line 15-20），非用户输入。SQL 注入风险极低。但字符串拼接 SQL 的模式不佳，若未来维护者误改为用户输入则有风险。

---

## 六、已具备的安全措施（确认有效）

### 认证与会话管理

| 措施 | 实现位置 | 说明 |
|---|---|---|
| 会话 token SHA256 哈希存储 | `auth_service.go:199`、`token.go` | 数据库中不存明文 |
| 会话过期校验 | `auth.go:78` | 过期会话拒绝访问 |
| 封禁用户拦截 | `auth.go:84` | `status != "active"` → 403 |
| 平台不匹配检测 + 审计日志 | `auth.go:89-106` | 自动吊销 + 记录 |
| Identity token 拒绝 API | `auth.go:73-76` | 仅 Web 会话可用于 API |
| 密码 bcrypt 哈希 | `user_service.go:82` | 非明文存储 |
| 密码长度校验 | `auth_service.go:67` | `min=8,max=72` |
| 会话 token 不出现在响应体 | `auth_service.go:111` | `json:"-"` + 测试验证（`auth_service_test.go:87`） |

### CSRF 防护

| 措施 | 实现位置 | 说明 |
|---|---|---|
| Double-submit cookie 模式 | `csrf.go` + `auth_handler.go:68` | 完整实现 |
| CSRF token 哈希存储 | `auth_service.go:206` | 数据库中不存明文 |
| 全部写操作验证 CSRF | `main.go:171-214` | POST/DELETE/PATCH 全覆盖 |
| CSRF token 自动过期清理 | `cleanup.go:62-69` | Worker 定期执行 |
| 前端自动附带 CSRF header | `api.ts:22-24` | 写操作自动添加 `X-CSRF-Token` |

### Cookie 安全

| 措施 | 实现位置 | 说明 |
|---|---|---|
| `__Host-` 前缀 | `auth_handler.go:67-68` | 要求 Secure + Path=/ + 无 Domain |
| `Secure=true` | `auth_handler.go:67-68` | 仅 HTTPS 传输 |
| `HttpOnly=true`（会话 cookie） | `auth_handler.go:67` | JS 不可读取 |
| `SameSite=Strict` | `auth_handler.go:66` | 阻止跨站 CSRF |
| CSRF cookie `HttpOnly=false` | `auth_handler.go:68` | 合理：JS 需读取放入 header |

### 授权与访问控制

| 措施 | 实现位置 | 说明 |
|---|---|---|
| 图片操作验证 ownership | `image_service.go:417` | `image.UserID != userID → 403` |
| 私密图片需 token/owner/admin | `public_handler.go:84-98` | 三重校验 |
| 管理员接口验证 role | `admin_handler.go` `RequireAdmin()` | 中间件拦截 |
| ID 均为 ParseUint | 全部 handler | 无字符串 ID 注入 |
| Cursor 分页（非 offset） | `image_repo.go:41` | 防止 offset 篡改 |

### 输入验证与文件上传

| 措施 | 实现位置 | 说明 |
|---|---|---|
| 文件扩展名白名单 | `image_service.go:191-197` | png/jpg/jpeg/webp/gif |
| 文件内容嗅探 | `image_service.go:214` | `http.DetectContentType` 检测真实类型 |
| MIME 类型白名单 | `image_service.go:370-377` | png/jpeg/webp |
| 文件大小限制 | `image_service.go:185` | 10MB |
| 存储路径由 SHA256 派生 | `image_service.go:221-222,245` | 用户不可控文件名/路径 |
| 请求体 binding 校验 | 全部 handler | `ShouldBindJSON` + 结构体验证标签 |
| Visibility 枚举校验 | `image_handler.go:167-170` | 仅 `public`/`private` |

### SQL 注入防护

| 措施 | 说明 |
|---|---|
| GORM 参数化查询 | 全项目使用 `?` 占位符 |
| 无原始 SQL 字符串拼接 | 仅 `migrate.go:26` 例外（硬编码值） |
| 搜索功能使用 LIKE `?` | `image_repo.go:48`（参数化） |

### 前端安全

| 措施 | 实现位置 | 说明 |
|---|---|---|
| 无 `dangerouslySetInnerHTML` | `frontend/src/` 全量搜索 | 无 XSS 向量 |
| 无硬编码密钥/token | `frontend/src/` 全量搜索 | 无 `process.env`、`VITE_` |
| API base 为相对路径 | `constants.ts:1` | `/api/v1`（同源，无 CORS 问题） |
| 第三方脚本均为 HTTPS | `captcha/hooks.ts:41-43` | reCAPTCHA/Turnstile/GeeTest |
| 401 自动重定向登录 | `api.ts:40-43` | 过期会话自动处理 |
| 敏感配置不泄露验证 | `public_handler_test.go:38-39` | 测试验证 `recaptcha_secret_key` 不在公开配置中 |

### 速率限制

| 措施 | 限制 |
|---|---|
| 登录 IP 级 | 5 次/15 分钟 |
| 登录用户名级 | 3 次/15 分钟 |
| Bootstrap | 10 次/分钟 |
| 上传 | 存在（errcode 4029） |

---

## 七、修复优先级

### 立即修复

| 编号 | 问题 | 修复方式 | 工作量 |
|---|---|---|---|
| SEC-C1 | 调试端点泄露配置 | 删除 `/public/wm-test` 端点 | 1 分钟 |
| SEC-H1 | 认证错误泄露细节 | `err.Error()` → 固定文案 | 5 分钟 |

### 计划修复

| 编号 | 问题 | 修复方式 |
|---|---|---|
| SEC-M1 | GIF 不一致 | `allowedImageMIME` 添加 `image/gif` |
| SEC-M2 | LIKE 通配符 | 转义 `%` 和 `_` |
| SEC-M3 | 大小限制不一致 | 统一 nginx 与后端的限制值 |
| SEC-L1 | SQL 拼接 | 改为参数化或常量（非紧急） |

---

## 八、审查覆盖范围

### 已覆盖

| 维度 | 检查内容 |
|---|---|
| 认证与会话 | token 生成、存储、校验、过期、吊销 |
| 授权 | ownership 校验、角色检查、IDOR |
| CSRF | 双提交 cookie、token 校验、写操作覆盖 |
| XSS | dangerouslySetInnerHTML、用户输入渲染 |
| SQL 注入 | 原始 SQL、字符串拼接、参数化 |
| 文件上传 | 扩展名、MIME、大小、路径构造 |
| 密钥管理 | 硬编码密钥、token 暴露、配置泄露 |
| 错误处理 | 内部信息泄露、堆栈暴露 |
| 输入验证 | 请求体校验、枚举值、边界值 |
| 速率限制 | 登录、注册、上传 |

### 未覆盖（后续可补充）

- 依赖项漏洞扫描（`go.mod` 第三方库 CVE 检查）
- 前端构建产物安全（Source Map 泄露、SRI 完整性）
- 穿透测试（实际攻击模拟）
- Go race condition 检测（`go test -race`）
