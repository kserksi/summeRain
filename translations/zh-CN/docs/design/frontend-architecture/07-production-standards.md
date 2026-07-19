# 07 · 生产产物规范（硬性要求）

> [!WARNING]
> **已归档的设计记录。** 本页面早于已经完成的 V2 前端，
> 其中的版本、路径或实现状态可能已经过时。

> 所属：[前端架构设计（索引）](./README.md)

> 以下规则是最终生产产物（`backend/web/`）的硬性规范。**开发与测试阶段可以豁免**，因为开发环境使用未签名的 Vite 产物，不受 SRI 与发布版本要求约束。

## 模块化与原子化

- 按特性与路由将所有 JavaScript/CSS 拆分为可行范围内最小的原子模块（基于特性的目录 + React.lazy 路由级分包，参见 [02 架构](02-architecture.md)），不得产生单文件巨型 bundle。
- 每个组件、hook 与工具都必须是职责单一、可独立复用的单元。

## SRI 完整性（SHA512）+ SemVer 版本

- 生产构建必须为**每一份** JavaScript/CSS 产物计算 **SHA512** 完整性哈希，并以 `{ "<文件名>": { "version": "<semver>", "integrity": "sha512-..." } }` 格式生成 `backend/web/assets.manifest.json`。
- **版本遵循 Semantic Versioning 2.0.0。** 版本粒度为**发布版本**：整个构建的 SemVer（例如 `1.0.0`，取自 `package.json` 的 `version`）统一标记该次构建中的所有产物，每个文件则记录自己的 SRI 哈希。若未来需要模块级版本，可在 manifest 中增加 `moduleVersion` 字段。
- **版本递增规则：** 发布前人工修改 `package.json` 中的 `version`：修复使用 `patch`，新功能使用 `minor`，不兼容变更使用 `major`。CI 使用该 `version` 创建 Git tag 并写入 manifest。未递增版本不得发布，产物 `version` 必须与 tag 一致。当前不引入 changesets、standard-version 等工具（YAGNI），采用人工修改 + CI 打 tag 的流程。
- 未来如引入跨域或外部托管资源，必须携带 SRI，否则不得使用。当前项目不存在此类资源，详见下一节。
- **覆盖范围（重要）：**
  - **入口与静态产物**（入口 JavaScript/CSS 以及 `index.html` 中的 `<link rel="modulepreload">`）-> `vite-plugin-sri3` 在 `index.html` 中注入 `integrity="sha512-..."`，由浏览器原生校验。
  - **动态分包**（通过 React.lazy 与 `import()` 加载的 chunk）-> 浏览器的**原生 `integrity` 机制不覆盖运行时 `import()`**，仅向 `index.html` 静态元素注入完整性属性并不充分。因此，实现时**必须增加运行时完整性守卫**：动态导入前先获取目标 chunk，与 `assets.manifest.json` 中的 SHA512 比对；不匹配时拒绝执行并上报。这是不得省略的实现子任务。
  - 也可以使用插件重写 Vite modulepreload polyfill，并为运行时创建的 preload link 注入 `integrity`，但必须实际验证其确实覆盖懒加载 chunk。
  - 风险背景：所有产物均为同源且不使用 CDN（见下一节），因此 SRI 主要防范的第三方来源篡改风险较低。但既然“校验每一份产物”是硬性要求，动态 chunk 就必须由上述守卫覆盖，不能豁免。

<a id="local-external-resources"></a>
## 外部资源本地化

- **禁止在运行时引用任何跨域/CDN 资源。** 所有第三方依赖必须由 Vite 打包为本地产物；字体使用系统字体栈，不加载 Web 字体；不得使用远程图片或图标。
- 构建后必须校验产物中不存在 `https://` 或 `http://` 形式的外链。仅允许后端同源的 `/api` 与 `/i` 相对路径。
- **唯一受控例外是人机验证。** 仅当管理员启用某个 provider（`captcha_provider ≠ none`，参见 [03](03-features.md#pluggable-captcha-administrator-selected-default-none)）时，才可以外部加载该 provider 的官方脚本。reCAPTCHA、Turnstile 与极验均无法自托管。默认值 `none` 意味着**零外链**，完全满足本节的硬性要求。启用某个 provider 时只豁免该 provider，且必须在构建校验白名单中显式登记其域名。

## 变量与常量

- **禁止魔法值：** 将数值上限、超时、分页大小、存储单位、路由路径、Query key、存储键等所有字面量集中在 `src/config/constants.ts` 中。导出的常量统一使用 **UPPER_SNAKE_CASE**，遵循 MDN 命名指南（与 [08 编码规范](08-coding-standards.md) 一致）。组件中不得出现裸数字或裸字符串。
- **JavaScript/TypeScript 变量：** 遵循 MDN 指南，使用 camelCase。
- **CSS 自定义属性：** 按 CSS 语言要求使用 kebab-case（例如 `--coffee-bg`）；该要求与 camelCase 规则不冲突。
- **编码：** 所有源文件与产物统一使用无 BOM 的 **UTF-8**；`index.html` 包含 `<meta charset="UTF-8">`。

## HTML 精简

- `index.html` 只保留**最低限度且必要**的内联 JavaScript/CSS。唯一允许的内联脚本是简短的 pre-paint 引导脚本，用于读取已保存的主题偏好并应用 `.dark`，以避免闪烁；其余内容全部放入外部 bundle。
- 该脚本必须在 bundle 加载前执行，无法导入常量模块，因此其 localStorage 键是**唯一受控例外**。必须与 `config/constants.ts` 中对应常量手工保持同步，并在代码注释中说明对应关系。主题键固定为 **`ic_theme`**，取值为 `light` 或 `dark`。

## 国际化（英语优先，多语言支持）

- 默认使用 `en-US`，并提供 `zh-CN` 与 `ja-JP`。组件通过 `react-i18next` 的 `t()` 获取文案，不得硬编码面向用户的字符串（参见 [04 主题与 UI](04-theme-and-ui.md)）。
- 结构应便于扩展，切换语言时无需修改组件代码。

---

<- [06 测试](06-testing.md) · [索引](./README.md) · 下一章节：[08 编码规范](08-coding-standards.md)
