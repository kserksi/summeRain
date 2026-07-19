# Design System - MASTER (summeRain)

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> This was the **single source of truth** for frontend UI/UX. Every page in
> [10-pages-ui-ux.md](../10-pages-ui-ux.md) was derived from it. Page-level
> exceptions belonged in `design-system/pages/<page>.md` and took precedence
> over this file. The system was validated in the `mockup/index.html` prototype.

## 1. Style Direction: Warm Soft Studio (maia)

- Start with the shadcn **maia** preset structure
  (`npx shadcn@latest apply --preset b3RZAU6YV`). **Ignore the preset colors,
  fonts, and icons** and replace them with the coffee tokens below.
- Favor **cards, visually independent elements, and generous rounding** while
  avoiding sharp elements. Use a **translucent navbar** with
  `inverted-translucent` and a subtle accent.
- Keep the interface calm, professional, and warm. Linear, Vercel, and Stripe
  are references, but warm coffee tones replace cool blue. Prioritize content,
  provide one primary CTA per screen, and keep decoration minimal.
- Component library: **shadcn/ui**; see Section 5.

## 2. Color Tokens (Coffee)

**Light (Mocha)**

| Token | Value | Purpose |
|---|---|---|
| `--bg` | `#F1E7DA` | Page background |
| `--bg2` | `#EADFCF` | Secondary surfaces, input backgrounds, table headers |
| `--card` | `#FFFCF8` | Cards |
| `--text` | `#33261B` | Body text |
| `--muted` | `#6E5C49` | Supporting text; 6.24:1 against cards |
| `--border` | `#DAC7AE` | Borders and separators |
| `--primary` | `#6F4E37` | Primary color (mocha) |
| `--primaryHover` | `#573D2B` | Primary hover |
| `--primarySoft` | `#EFE2D2` | Soft primary background |
| `--accent` | `#A9764F` | Supporting color and gradients |
| `--success/--danger/--info/--warn` | `#5C7A4A / #A8412F / #4A6E92 / #B9742A` | Softened status colors |

**Dark (Espresso)**

| Token | Value |
|---|---|
| `--bg / --bg2 / --card` | `#16100D / #1E1611 / #251B14` |
| `--text / --muted` | `#F2E7D6 / #C5B59E` |
| `--border` | `#4D3A29` |
| `--primary / --primaryHover / --primarySoft` | `#D4A57E / #B9895F / #33261829` |
| `--accent` | `#C39A72` |
| `success/danger/info/warn` | `#9CBF82 / #D88A78 / #86A8C9 / #E0AC64` |

- Body text has **11.98:1** contrast against the background; supporting text has
  **6.24:1** against cards. Both exceed AA at 4.5:1.
- **Never use raw hex values inside components.** Use semantic token classes
  such as `bg-primary` and `text-muted-foreground`. Dark mode adapts through
  tokens automatically; **do not write manual `dark:` overrides**.

## 3. Typography

- Use the **system font stack** with no webfont, satisfying
  [07 External Resource Localization](../07-production-standards.md).
- Type scale: 12 / 14 / 16 / 18 / 24 / 32 / 48. Body text is **16** with
  **1.65** line height.
- Font weights: 400 body / 500 labels / 600-700 headings.
- Use `tabular-nums` for numeric columns, prices, and timers to prevent layout
  shift.
- Headings use fluid `clamp()`, such as landing-page H1
  `clamp(32px,6vw,58px)`.

## 4. Radius / Shadow / Spacing

- **Radius (maia large):** cards 22-24px / controls 12-14px / large containers
  28-30px / pills 999px.
- **Shadow:** layered soft shadows with a warm brown base. In dark mode, replace
  heavy shadows with low-luminance outlines.
- **Spacing:** 4/8 baseline (4 / 8 / 12 / 16 / 24 / 32 / 48).

## 5. Components (shadcn/ui) and Composition Rules

**Inventory:** forms
`Field/FieldGroup/FieldLabel/Input/InputGroup/Select/Switch/ToggleGroup/Slider/Textarea`;
data `Card` (full composition) / `Table/Badge/Avatar/Progress/Chart`;
navigation `NavigationMenu/Tabs/Breadcrumb/Pagination`; overlays
`Dialog/AlertDialog/Sheet/Drawer/DropdownMenu/Tooltip/Popover`; feedback
`sonner/Alert/Skeleton/Spinner/Empty/Progress`; commands `Command`; layout
`Separator/ScrollArea`.

**Required composition rules:**

- Always compose forms with `FieldGroup` + `Field`; do not use a raw div with
  `space-y`. Validation uses `data-invalid` on Field and `aria-invalid` on the
  control.
- Use `gap-*` for spacing, never `space-x/y-*`; use `size-*` for equal width and
  height; use `cn()` for conditional classes.
- Use `Empty` for empty states, `Alert` for messages, `sonner` for toasts,
  `Separator` for dividers, `Skeleton` for placeholders, and `Badge` for status.
  **Do not invent styled div replacements.**
- Confirm destructive operations with `AlertDialog`. Every `Dialog/Sheet` must
  include a `Title`; use `sr-only` when it is visually hidden.
- Use the complete `CardHeader/Title/Description/Content/Footer` composition for
  `Card`.
- **Do not hand-write z-index values** for overlay components; they manage their
  own stacking.

**Icons:** use **Tabler Icons** (`@tabler/icons-react`) with one outline style.
Inside `Button`, use `data-icon="inline-start|end"` and **do not add a size
class**. Icon buttons are at least 44x44 and include `aria-label`. Set
`components.json` `iconLibrary` to **tabler**, replacing the preset's phosphor
icons.

**No native controls when appearance must be unified:** replace native
`<select>` with a custom dropdown, use a custom scrollbar, and convert tables to
cards on mobile.

## 6. Unified State Standards

`hover` (brighten token) / `active` (scale .97-.98) / `disabled` (opacity .5 +
cursor) / `focus-visible` (2-4px ring) / `loading` (spinner + disabled) / `empty`
(`Empty` + copy + CTA) / `error` (copy + retry) / `skeleton` (>300ms).

## 7. Responsive Behavior

- **Mobile first**; breakpoints 375 / 768 / 1024 / 1440.
- At widths <=820: navbar -> **hamburger + sliding drawer**; tables -> **cards**;
  every grid stacks to one column; type and spacing become tighter.
- Use `clamp()` for headings and allow **no horizontal scrolling**.
- Touch targets are at least 44x44; desktop containers use `max-w-7xl`.

## 8. Motion

- Micro-interactions last 150-300ms. Use ease-out when entering and ease-in when
  leaving. **Honor `prefers-reduced-motion`** by disabling animation and
  transitions globally. **Do not animate width/height.**
- Entrance uses `fadeUp` plus staggered cards; cards and buttons lift on hover;
  progress bars shimmer; loading icons spin; the hero dot pattern breathes.
- Fade pages in on navigation.

## 9. Theme Switching (View Transitions First, Mask Fallback)

- Provide an **explicit** top-bar switch with ☀/🌙, not hidden in a menu.
  `light` / `dark` adapt through tokens.
- **When View Transitions are supported** (Chrome/Edge), animate
  `::view-transition-new(root)` with `clip-path: circle()` expanding from the
  switch center from **0 to 150%**. This creates a circular wipe of the **real
  new-theme content** and gives the best result.
- **Without View Transitions:** use `.theme-mask`, a 300vmax solid circle animated
  with `transform:scale(0 to 1)` through the Web Animations API.
- With **`prefers-reduced-motion`**, switch immediately without animation.
- Persist in localStorage. On first paint, follow the system
  `prefers-color-scheme`; an inline pre-paint script prevents flashing.

## 10. Lightbox (Detail Image)

The detail image is clickable and includes a "Click to enlarge" hint. Clicking
opens a full-screen lightbox with the enlarged image, close button, and backdrop.
Close it through the button, a backdrop click, or Esc. Use a scale transition on
entry and exit.

## 11. Accessibility and Performance

- Contrast is at least 4.5:1 (AA); focus rings are visible; Tab order matches
  visual order; icon buttons have `aria-label`; images have `alt`; heading levels
  are semantic.
- Images use `loading="lazy"` plus dimensional placeholders to prevent CLS;
  routes are lazy-loaded; virtualize lists containing at least 50 items.

---

<- [Index](../README.md) - Page specifications:
[10-pages-ui-ux.md](../10-pages-ui-ux.md)
