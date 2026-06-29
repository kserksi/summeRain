# 08 · 编码规范

> 所属：[前端架构设计（索引）](./README.md)

> 结合用户生产要求（[07 生产规范](07-production-standards.md)）与 React / TypeScript / Tailwind 官方指南，作为实现期统一准则。官方依据：[React 文档](https://react.dev/learn/thinking-in-react) 与 [Hook 规则](https://react.dev/reference/rules)、[TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html)、[typescript-eslint](https://typescript-eslint.io)、[Tailwind CSS](https://tailwindcss.com/docs)。

## 总则

- 遵循三方官方指南；[07 生产规范](07-production-standards.md) 为生产硬性要求、本节为日常编码细则，二者叠加生效。
- 自描述代码优先；默认不写代码注释，仅在"为什么"非直观时写 `// why`，不写 `// what`。公开 Hook/工具写 TSDoc。

## 命名

- 变量/函数/普通文件：camelCase；组件/类型/接口：PascalCase（组件文件名与组件同名）
- 常量：`UPPER_SNAKE`，集中于 `config/constants.ts`；Hook：`use` 前缀；i18n key：点分命名空间
- 布尔：`is/has/can/should` 前缀；事件处理：`handleXxx` / `onXxx`
- CSS 自定义属性：kebab-case（语言强制）；其余一律驼峰

## TypeScript

- `"strict": true`，**禁 `any`**（确需须注释说明）；公开 API 显式标注类型，局部依赖推断
- `interface` 用于可扩展/合并的对象形状，`type` 用于联合与工具类型，二者不混用
- `const`/`let` 禁 `var`；优先 `async/await`；`catch` 变量按 `unknown` 处理，错误用类型化 `ApiError`
- 无魔法值：字面量收口常量模块

## React（遵循官方组件与 Hook 指南）

- 仅函数组件 + Hook，遵守 [Rules of Hooks](https://react.dev/reference/rules/rules-of-hooks)；一文件一主组件
- props 用 `interface`/`type`，必填优先；列表用稳定 `key`（不用数组索引）
- **派生状态优先于 `useEffect`**；effect 依赖准确、单一职责，避免链式 effect
- 组合优于继承；纯组件优先，副作用隔离；守卫早返回
- 表单用 React Hook Form + Zod（schema 即校验，单一真相）

## 状态与数据

- **服务端状态只走 TanStack Query**，不在 `useState`/zustand 缓存接口数据
- zustand 仅存真正客户端状态（主题、当前用户快照）；Query key 规范化集中
- mutation 用 `invalidateQueries` 失效，不手动改缓存（除非带回滚的乐观更新）

## 样式（Tailwind v4 官方约定）

- 工具类优先；组件内**禁裸 hex**，统一用 shadcn token 类（`bg-primary` 等）
- 复杂样式抽组件而非滥用 `@apply`；条件类用 `cn()`（clsx + tailwind-merge）
- 响应式**移动优先**（`sm:/md:/lg:`）；暗色用 `dark:` 变体（基于 `.dark` 类）

## 文件与导出

- feature-based（[02 架构](02-architecture.md)）；优先**命名导出**，页面/路由组件可默认导出
- `index.ts` barrel 谨慎使用，避免循环依赖与打包膨胀

## 格式化与 Lint（门禁）

- **Prettier**：2 空格、分号、双引号、尾逗号 `all`、行宽 100、`LF`、UTF-8 无 BOM
- **ESLint（flat config）**：`typescript-eslint` recommended + `react` recommended + `react-hooks` + `jsx-a11y`
- **分层门禁**（详见 [06 测试](06-testing.md)）：
  - pre-commit（快）：`prettier --check` + `eslint` + `tsc --noEmit`
  - CI（阻断合并/发布）：上述三项 + `vitest run`
- 禁 `console.*`、`debugger`、注释掉的死代码、未使用导入

## 安全与无障碍

- **前端非安全边界**：所有鉴权/越权由后端判定（`/admin/*` 走 `RequireAdmin`，普通用户 403）；前端代码与 chunk 视为公开，绝不靠"藏代码/藏路由"做权限。数据敏感操作的唯一护栏是后端。
- **切换用户清缓存**：登出/登录必须 `queryClient.clear()` + **硬刷新**（`window.location.assign`），杜绝上一会话的内存数据（尤其 TanStack Query 缓存）跨用户残留；`auth-store` 不持久化（详见 [02 会话切换的数据清理](02-architecture.md)）。
- 不打印/提交密钥；用户输入消毒；CSRF/凭据统一在 `lib/api.ts`
- 禁 `dangerouslySetInnerHTML`（除非经消毒）
- 沿用 shadcn 无障碍基线：语义化 HTML、`label`、`aria`、键盘可达、焦点可见；图片 `alt`；文本对比度 ≥ 4.5:1（WCAG AA）

## 性能

- 路由懒加载（[02 架构](02-architecture.md)）；图片 `loading="lazy"` 与适当尺寸
- Query 合理 `staleTime`/`gcTime`，避免过度请求；`memo` 先测后优

## 提交

- Conventional Commits（`feat:` / `fix:` …），小步单一职责
- 仅在用户明确要求时执行提交（与全局约定一致）

---

← [07 生产规范](07-production-standards.md) · [索引](./README.md) · 下一板块：[09 决策与范围](09-decisions-and-scope.md)
