# 07 · 生产产物规范（硬性要求）

> 所属：[前端架构设计（索引）](./README.md)

> 以下为最终生产产物（`backend/web/`）的强制规范。**开发/测试阶段可豁免**（dev 走 Vite 未签名产物，不计 SRI/版本）。

## 模块化与原子化

- JS/CSS 一律按特性 + 路由拆分为最小原子模块（feature-based 目录 + React.lazy 路由分包，见 [02 架构](02-architecture.md)），杜绝单文件巨型 bundle。
- 组件、hook、工具均为单一职责的独立可复用单元。

## SRI 完整性（SHA512）+ SemVer 版本号

- 生产构建对**每一份** JS/CSS 产物计算 **SHA512** 完整性哈希，并生成 `backend/web/assets.manifest.json`：`{ "<文件名>": { "version": "<semver>", "integrity": "sha512-..." } }`。
- **版本号遵循 Semantic Versioning 2.0.0**。版本粒度：以**发布版本**（整个 build 的 SemVer，如 `1.0.0`，源自 `package.json` 的 `version`）统一标注该次产物的所有文件；每份文件各自记录独立的 SRI 哈希。（如未来需要模块级独立版本，可在 manifest 扩展 `moduleVersion` 字段。）
- **bump 规则**：发布时**人工**改 `package.json` 的 `version`——修复 `patch`、新增功能 `minor`、不兼容变更 `major`；CI 据此 `version` 打 Git tag 并写入 manifest。未 bump 不发布（构建产物的 `version` 即所打 tag）。当前不引入 changesets/standard-version 等工具（YAGNI），流程纯人工+CI tag。
- 跨域/外链资源（本项目已无，见下节）若引入，必须带 SRI，否则不引入。
- **覆盖范围（重要）**：
  - **入口与静态产物**（入口 JS/CSS 及 index.html 内 `<link rel="modulepreload">`）→ 由 `vite-plugin-sri3` 在 `index.html` 注入 `integrity="sha512-..."`，浏览器原生校验。
  - **动态分包**（React.lazy 经 `import()` 加载的 chunk）→ 浏览器**原生 `integrity` 机制不覆盖运行时 `import()`**，仅靠插件注入 index.html 静态标签无法保护。因此**必须额外实现运行时完整性守卫**：在动态 import 前以 `assets.manifest.json` 中的 SHA512 校验目标 chunk（fetch 后比对哈希、不匹配则拒绝执行并上报）。该项为实现期硬性子任务，不得省略。
  - 备选：若采用能改写 Vite modulepreload polyfill、对运行时创建的 preload link 注入 `integrity` 的插件方案，亦可，但须验证其对懒加载 chunk 确实生效。
  - 风险背景：本站所有产物同源（无 CDN，见下节），降低了 SRI 主要防范的第三方源篡改面；但既已声明"每一份校验"为硬性要求，动态 chunk 须以上述守卫补齐，而非豁免。

## 外部资源本地化

- **禁止运行时引用任何跨域/CDN 资源**：所有第三方依赖经 Vite 打包内联为本地产物；字体使用系统字体栈（不引 webfont）；无远程图片/图标。
- 构建后需校验产物中不存在 `https://`、`http://` 形态的外链（仅允许后端同源的 `/api`、`/i` 相对路径）。
- **唯一受控豁免：人机验证**。当且仅当管理员启用了某家 captcha（`captcha_provider ≠ none`，见 [03](03-features.md#人机验证可插拔管理员决定默认无)），**仅该 provider 的官方脚本**作为外链加载（reCAPTCHA/Turnstile/极验均无法自托管）。**默认 `none` → 零外链**，本节硬性要求完全成立；启用哪家就豁免哪家，且需在构建校验白名单中显式登记该 provider 域名。

## 变量与常量规范

- **无魔法值**：所有字面量（数值上限、超时、分页大小、存储单位、路由路径、Query key、存储键名等）一律收口于 `src/config/constants.ts`，导出常量统一 **UPPER_SNAKE_CASE**，遵循 MDN 常量命名规范（与 [08 编码规范](08-coding-standards.md) 一致），组件内不得出现裸数字/裸字符串。
- **JS/TS 变量**：camelCase（MDN 规范）。
- **CSS 自定义属性**：按 CSS 规范使用 kebab-case（`--coffee-bg` 等，此为语言强制，不与"驼峰"冲突）。
- **编码**：所有源文件与产物统一 **UTF-8**（无 BOM）；`index.html` 含 `<meta charset="UTF-8">`。

## HTML 精简

- `index.html` 仅保留**最少且必要**的内联 JS/CSS。唯一许可的内联脚本为主题防闪烁引导（pre-paint、几行，读取本地主题偏好加 `.dark` 类），其余全部走外部 bundle。
- 该内联脚本因须在 bundle 加载前执行、无法导入常量模块，其使用的本地存储键名作为**唯一受控例外**与 `config/constants.ts` 中的同名常量手工保持同步，并在代码注释中标注对应关系。主题键钉死为 **`ic_theme`**（取值 `light`/`dark`）。

## i18n（英语优先，多语言支持）

- 默认使用 `en-US`，并提供 `zh-CN` 与 `ja-JP`；通过 `react-i18next` 的 `t()` 取用文案，组件不硬编码面向用户的字符串（见 [04 主题与 UI](04-theme-and-ui.md)）。
- 结构预留多语言扩展；切换语言不需改动组件代码。

---

← [06 测试](06-testing.md) · [索引](./README.md) · 下一板块：[08 编码规范](08-coding-standards.md)
