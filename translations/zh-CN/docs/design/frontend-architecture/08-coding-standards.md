# 08 · 编码规范

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

> 本规范将项目生产要求（[07 生产规范](07-production-standards.md)）与 React、TypeScript、Tailwind 官方指南结合，形成统一的实现准则。权威依据包括：[React 文档](https://react.dev/learn/thinking-in-react)与 [Hook 规则](https://react.dev/reference/rules)、[TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html)、[typescript-eslint](https://typescript-eslint.io)及 [Tailwind CSS](https://tailwindcss.com/docs)。

## 总则

- 遵循三个生态的官方指南。[07 生产规范](07-production-standards.md) 定义生产环境硬性要求，本节定义日常编码细则，两者共同生效。
- 优先编写自描述代码。默认不添加注释；只有原因不直观时才写 `// why`，不得写 `// what`。公开 Hook 与工具使用 TSDoc。

## 命名

- 变量、函数与普通文件使用 camelCase；组件、类型与接口使用 PascalCase，组件文件名与组件同名。
- 常量使用 `UPPER_SNAKE_CASE` 并集中在 `config/constants.ts`；Hook 使用 `use` 前缀；国际化键使用点分命名空间。
- 布尔值使用 `is`、`has`、`can` 或 `should` 前缀；事件处理器使用 `handleXxx` 或 `onXxx`。
- CSS 自定义属性按语言要求使用 kebab-case；其余命名遵循上述 camelCase 规则。

## TypeScript

- 设置 `"strict": true`，并**禁止使用 `any`**。确实无法避免时，必须用注释说明原因。公开 API 显式声明类型，局部代码依赖类型推断。
- 使用 `interface` 表示可扩展或可合并的对象形状，使用 `type` 表示联合类型与工具类型；不要混用两套约定。
- 使用 `const` 或 `let`，禁止 `var`。优先使用 `async/await`。将 `catch` 变量按 `unknown` 处理，错误使用类型化的 `ApiError`。
- 消除魔法值，将字面量集中到常量模块。

## React（遵循官方组件与 Hook 指南）

- 只使用函数组件与 Hook，并遵守 [Rules of Hooks](https://react.dev/reference/rules/rules-of-hooks)。每个文件只包含一个主组件。
- 使用 `interface` 或 `type` 定义 props，并优先使用必填字段。列表使用稳定的 `key`，不得使用数组索引。
- **优先使用派生状态，而不是 `useEffect`。** effect 依赖必须准确，每个 effect 只承担一项职责，避免链式 effect。
- 组合优于继承，纯组件优先；隔离副作用，守卫使用提前返回。
- 表单使用 React Hook Form + Zod；schema 是校验的单一事实来源。

## 状态与数据

- **所有服务端状态都通过 TanStack Query 管理。** 不得在 `useState` 或 zustand 中缓存 API 数据。
- zustand 只保存真正的客户端状态：主题与当前用户快照。Query key 必须规范化并集中管理。
- mutation 通过 `invalidateQueries` 使数据失效；除非实现带回滚的乐观更新，否则不得手工修改缓存。

## 样式（Tailwind v4 官方约定）

- 优先使用工具类。**组件内禁止裸 hex 色值**，统一使用 `bg-primary` 等 shadcn 令牌类。
- 将复杂样式抽成组件，不要滥用 `@apply`。条件类使用 `cn()`（clsx + tailwind-merge）。
- 响应式布局采用**移动优先**策略，使用 `sm:`、`md:` 与 `lg:`；暗色模式使用基于 `.dark` 类的 `dark:` 变体。

## 文件与导出

- 按特性组织代码（参见 [02 架构](02-architecture.md)）。优先使用**命名导出**，页面与路由组件可以使用默认导出。
- 谨慎使用 `index.ts` barrel 文件，避免循环依赖与 bundle 膨胀。

## 格式化与 Lint（质量门禁）

- **Prettier：** 2 空格缩进、分号、双引号、尾逗号设为 `all`、行宽 100、`LF`、无 BOM 的 UTF-8。
- **ESLint（flat config）：** `typescript-eslint` recommended + `react` recommended + `react-hooks` + `jsx-a11y`。
- **分层门禁**（详见 [06 测试](06-testing.md)）：
  - pre-commit（快速）：`prettier --check` + `eslint` + `tsc --noEmit`
  - CI（阻断合并/发布）：上述三项检查 + `vitest run`
- 禁止 `console.*`、`debugger`、被注释掉的死代码与未使用的导入。

## 安全与无障碍

- **前端不是安全边界。** 所有鉴权与授权结果由后端判定（`/admin/*` 通过 `RequireAdmin`，普通用户会收到 403）。前端代码与 chunk 均应视为公开内容，绝不能依赖隐藏代码或路由实现访问控制。敏感数据操作的唯一防线是后端强制校验。
- **切换用户时清理缓存。** 登录与登出必须调用 `queryClient.clear()`，并通过 `window.location.assign` 执行**硬刷新**，防止上一会话的内存数据，尤其是 TanStack Query 缓存，跨用户残留。`auth-store` 不持久化（详见 [02 会话切换的数据清理](02-architecture.md)）。
- 不得打印或提交密钥；消毒用户输入；在 `lib/api.ts` 中集中处理 CSRF 与凭据。
- 禁止使用 `dangerouslySetInnerHTML`，除非输入已经消毒。
- 保持 shadcn 的无障碍基线：语义化 HTML、`label`、`aria`、键盘可达、焦点可见、图片 `alt` 文字，以及至少 4.5:1 的文字对比度（WCAG AA）。

## 性能

- 路由采用懒加载（参见 [02 架构](02-architecture.md)）；图片使用 `loading="lazy"` 与合适尺寸。
- 为 Query 设置合理的 `staleTime` 与 `gcTime`，避免过度请求；使用 `memo` 前先进行测量。

## 提交

- 使用 Conventional Commits（`feat:`、`fix:` 等），每个提交保持小而单一。
- 仅在用户明确要求时提交，与全局约定保持一致。

---

<- [07 生产规范](07-production-standards.md) · [索引](./README.md) · 下一章节：[09 决策与范围](09-decisions-and-scope.md)
