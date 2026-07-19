# 05 · 构建与部署

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> 所属：[前端架构设计（索引）](./README.md)

## 构建

`vite build` → `build.outDir: '../backend/web'`，`build.emptyOutDir: true`，`base: '/'`。

> **base 必须为 `'/'`（不可用相对的 `'./'`）**：BrowserRouter 使用 HTML5 history 路由，深链（如 `/images/123`）刷新时后端 `NoRoute` 回退到 `index.html`，浏览器以**当前文档 URL** 解析资源路径——若 base 为相对的 `./`，`./assets/index.js` 会被解析成 `/images/assets/index.js` 而命中回退返回 HTML，脚本加载失败。Vite 官方亦明确警告相对 base 不兼容 history 路由刷新。本项目前端部署在**域名根**（Go 以 `./web/` 提供同源服务），故 `base: '/'` 成立。若将来改为子路径部署，须同时改 `base` 与 React Router 的 `basename`（当前无需）。

生产构建启用 `vite-plugin-sri3` 注入 SHA512 `integrity` 属性，并输出 `assets.manifest.json`（每份产物 → `{ version, integrity }`，详见 [07 生产规范](07-production-standards.md)）。

## 生产部署

Go 单进程同源提供 `backend/web/` + `/api` + `/i`，cookie 天然同源。

## 开发工作流

Vite `:5173`（HTTPS），`server.proxy` 将 `/api`、`/i` 转发到 `http://localhost:8080`（Go），保证开发期 cookie 同源。

**`__Host-` cookie 代理注意事项**（否则浏览器拒收，登录态存不住）：
- 代理须 `changeOrigin: true`，且**不得**设置 `cookieDomainRewrite` / `cookiePathRewrite`（`__Host-` 要求无 Domain、Path=/，任何改写都会破坏前缀规则）
- 保留响应 `Set-Cookie` 头原样转发（`Secure` 标志由 :5173 的 HTTPS 满足；上游 Go 虽是 http，但 `c.SetCookie` 仍写出 `Secure`，浏览器经 HTTPS 视图接收即可接受）
- cookie 落在 `localhost:5173`（代理源），后续 `/api`、`/i` 请求自动携带
- 配合下方 HTTPS 前置一起生效

## ⚠️ HTTPS 必需（硬性 dev 前置条件）

`__Host-` 前缀 cookie 要求 `Secure` + HTTPS，本地 http 下浏览器会拒绝写入 `__Host-session_token`，导致登录态存不住。开发环境**必须**为 Vite 启用 HTTPS（`@vitejs/plugin-basic-ssl` 或 mkcert），否则无法正常登录联调。

---

← [04 主题与 UI](04-theme-and-ui.md) · [索引](./README.md) · 下一板块：[06 测试策略](06-testing.md)
