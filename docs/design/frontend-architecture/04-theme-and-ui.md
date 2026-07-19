# 04 · 主题与 UI

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> 所属：[前端架构设计（索引）](./README.md)

## shadcn 预设策略

- 初始化后执行 `npx shadcn@latest apply --preset b3RZAU6YV` 拉取该预设
- **预设仅用于决定组件构成与默认结构**（组件集、radius、变体骨架等）
- **忽略预设的颜色、字体与图标**：
  - 颜色 → 由我们的**咖啡色板**覆盖（写入 `src/styles.css` 的 `@theme` 与 `:root`/`.dark` CSS 变量，Tailwind v4 CSS-first，无 `tailwind.config.js`）
  - 字体 → 沿用系统字体栈，不采用预设字体（geist）
  - 图标 → 采用 **Tabler Icons**，不采用预设图标库（phosphor）；`components.json` 的 `iconLibrary` 设 `tabler`

## 咖啡色板（Tailwind v4 `@theme` 接线）

> **单一真相源为 [design-system/MASTER.md](design-system/MASTER.md) §2**（含完整令牌表与对比度）。下方为摘要；色值经原型迭代**加深以提高对比度**（辅助对卡片 6.24:1、正文 11.98:1），**取代**早期 `css/style.css`（遗留原生前端，将被 `frontend/` 替换，不再作为色值来源）。

Tailwind v4 为 CSS-first：色板写入 `src/styles.css` 的 `@theme { --color-*: ...; }` 与 `:root`/`.dark` 变量，组件直接用 `bg-primary` / `text-foreground` 等 token 类。无 `tailwind.config.js`。

浅色（摩卡）`:root`：
- 背景 `#F1E7DA` / 卡片 `#FFFCF8` / 文字 `#33261B` / 辅助 `#6E5C49` / 边框 `#DAC7AE`
- 主色 `#6F4E37`（hover `#573D2B`，浅底 `#EFE2D2`）/ 辅助 `#A9764F`

深色（浓缩）`.dark`：
- 背景 `#16100D` / 卡片 `#251B14` / 文字 `#F2E7D6` / 辅助 `#C5B59E` / 边框 `#4D3A29`
- 主色 `#D4A57E`（hover `#B9895F`）/ 辅助 `#C39A72`

状态色（柔化）详见 MASTER §2。

状态色统一柔化（绿/蓝/红/紫对应柔化值），状态语义：成功/警告/危险/信息。

## 字体与 i18n

- **字体**：系统字体栈 `-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Roboto, ...`。不引入任何外部 webfont（满足 [07 生产规范 · 外部资源本地化](07-production-standards.md#外部资源本地化)）。
- **i18n**：英语是主要语言，默认语言为 `en-US`，同时提供 `zh-CN` 和 `ja-JP`。所有 UI 字符串按语言存放在 `src/i18n/locales/`，组件内一律通过 `t('key')` 取用，禁止硬编码面向用户的文案。

> 本条覆盖早期"暂不引入 i18n"的 YAGNI 决策（见 [09 决策记录](09-decisions-and-scope.md#决策记录)）。

---

← [03 特性](03-features.md) · [索引](./README.md) · 下一板块：[05 构建与部署](05-build-and-deploy.md)
