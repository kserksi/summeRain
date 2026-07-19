# 04 · Theme and UI

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

## shadcn Preset Strategy

- After initialization, run `npx shadcn@latest apply --preset b3RZAU6YV` to retrieve the preset.
- **Use the preset only to determine the component selection and default structure** (component set, radius, variant scaffolding, and so on).
- **Ignore the preset's colors, typography, and icons:**
  - Colors -> override them with **our coffee palette** (define it in the `@theme` block and the `:root`/`.dark` CSS variables in `src/styles.css`; Tailwind v4 is CSS-first, with no `tailwind.config.js`).
  - Typography -> retain the system font stack instead of the preset font (Geist).
  - Icons -> use **Tabler Icons**, not the preset icon library (Phosphor); set `iconLibrary` in `components.json` to `tabler`.

## Coffee Palette (Tailwind v4 `@theme` Wiring)

> **[design-system/MASTER.md](design-system/MASTER.md) §2 is the single source of truth** (including the complete token table and contrast ratios). The following is a summary. The colors were **darkened during prototype iteration to improve contrast** (secondary text on cards: 6.24:1; body text: 11.98:1) and **supersede** the earlier `css/style.css` (the legacy native frontend will be replaced by `frontend/` and is no longer a color source).

Tailwind v4 is CSS-first: define the palette through `@theme { --color-*: ...; }` and the `:root`/`.dark` variables in `src/styles.css`. Components use token classes such as `bg-primary` and `text-foreground` directly. There is no `tailwind.config.js`.

Light theme (Mocha), `:root`:
- Background `#F1E7DA` / card `#FFFCF8` / text `#33261B` / muted text `#6E5C49` / border `#DAC7AE`
- Primary `#6F4E37` (hover `#573D2B`, subtle background `#EFE2D2`) / secondary `#A9764F`

Dark theme (Espresso), `.dark`:
- Background `#16100D` / card `#251B14` / text `#F2E7D6` / muted text `#C5B59E` / border `#4D3A29`
- Primary `#D4A57E` (hover `#B9895F`) / secondary `#C39A72`

See MASTER §2 for the softened status colors.

All status colors are softened. Green, blue, red, and purple represent success, warning, danger, and information respectively.

## Typography and i18n

- **Typography:** use the system font stack `-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Roboto, ...`. Do not load any external webfont (satisfying [07 Production Standards · Local External Resources](07-production-standards.md#local-external-resources)).
- **i18n:** English is the primary language and `en-US` is the default locale; `zh-CN` and `ja-JP` are also provided. Store all UI strings by locale in `src/i18n/locales/`. Components must retrieve them through `t('key')` and must not hard-code user-facing copy.

> This requirement supersedes the earlier YAGNI decision to defer i18n (see [09 Decision Record](09-decisions-and-scope.md#decision-record)).

---

<- [03 Features](03-features.md) · [Index](./README.md) · Next: [05 Build and Deployment](05-build-and-deploy.md)
