# 05 · 构建与部署

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

## 构建

`vite build` -> `build.outDir: '../backend/web'`、`build.emptyOutDir: true`、`base: '/'`。

> **`base` 必须为 `'/'`，不能使用相对值 `'./'`：** BrowserRouter 使用 HTML5 history 路由。刷新 `/images/123` 等深层链接时，后端 `NoRoute` 处理器会回退到 `index.html`，浏览器则根据**当前文档 URL** 解析资源路径。若 base 为相对的 `./`，`./assets/index.js` 会被解析为 `/images/assets/index.js`，命中 HTML 回退后无法作为脚本加载。Vite 官方文档也明确警告，相对 base 与 history 路由刷新不兼容。本项目前端部署在**域名根路径**（Go 通过 `./web/` 提供同源服务），因此 `base: '/'` 正确。若未来改为子路径部署，必须同时修改 `base` 与 React Router 的 `basename`（当前无需）。

生产构建启用 `vite-plugin-sri3` 注入 SHA512 `integrity` 属性，并输出 `assets.manifest.json`（每份产物对应一个 `{ version, integrity }` 条目；详见 [07 生产规范](07-production-standards.md)）。

## 生产部署

单个 Go 进程在同一来源提供 `backend/web/`、`/api` 与 `/i`，因此 cookie 天然同源。

## 开发工作流

Vite 通过 HTTPS 运行在 `:5173`。`server.proxy` 将 `/api` 和 `/i` 转发到 `http://localhost:8080` 上的 Go 服务，保证开发期 cookie 同源。

**`__Host-` cookie 的代理要求**（否则浏览器会拒收，登录状态无法保持）：
- 设置 `changeOrigin: true`，并且**不得**设置 `cookieDomainRewrite` 或 `cookiePathRewrite`（`__Host-` 要求不含 Domain 属性且 Path=/，任意改写都会破坏前缀规则）。
- 原样转发响应中的 `Set-Cookie` 头。`:5173` 的 HTTPS 满足 `Secure` 标志；虽然上游 Go 使用 HTTP，但 `c.SetCookie` 仍会写出 `Secure`，浏览器通过 HTTPS 代理视图接收时可以接受。
- cookie 存储在 `localhost:5173`（代理来源）下，后续 `/api` 与 `/i` 请求会自动携带。
- 此配置须与下方 HTTPS 前置条件配合使用。

## ⚠️ 必须使用 HTTPS（开发环境硬性前置条件）

带 `__Host-` 前缀的 cookie 要求 `Secure` 与 HTTPS。本地 HTTP 环境下，浏览器会拒绝写入 `__Host-session_token`，导致登录状态无法保持。开发环境**必须**为 Vite 启用 HTTPS（使用 `@vitejs/plugin-basic-ssl` 或 mkcert），否则无法正常进行登录联调。

---

<- [04 主题与 UI](04-theme-and-ui.md) · [索引](./README.md) · 下一章节：[06 测试策略](06-testing.md)
