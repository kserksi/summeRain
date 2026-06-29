# 06 · 测试策略

> 所属：[前端架构设计（索引）](./README.md)

## 范围

**Vitest + React Testing Library**：

- 单测 `lib/api.ts`（信封拆解 / CSRF 注入 / **401 登出 / 4030 停用 / 429 限流 / 错误码 i18n 映射**）
- 各 feature 的 `hooks.ts`（用 MSW mock 接口，验证 query/mutation 行为）

关键交互（上传、无限滚动、表单校验、可见性切换、改密重登、注册跳转）写组件测试。

## 取舍

不追求全量覆盖，优先 **api 层 + 鉴权/查询钩子**——这两处是系统正确性的核心，且最易回归。

## 门禁分层

- **pre-commit（本地，求快）**：`prettier --check` + `eslint` + `tsc --noEmit`（与 [08](08-coding-standards.md) 一致）
- **CI（阻断合并/发布）**：上述三项 **+ `vitest run`**。测试不通过不得合并；发布构建同样跑测试。

---

← [05 构建与部署](05-build-and-deploy.md) · [索引](./README.md) · 下一板块：[07 生产产物规范](07-production-standards.md)
