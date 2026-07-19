# 10 - Page-by-Page UI/UX Specifications

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of [Frontend Architecture Design (Index)](./README.md). See
> [design-system/MASTER.md](./design-system/MASTER.md) for design tokens and
> component rules. Every layout below was validated in `mockup/index.html`.
> All components are composed with shadcn/ui.

## Public Pages

### `/` Landing Page

- **Visibility:** available to everyone; authenticated visitors are redirected
  automatically to `/dashboard`.
- **Layout:** (1) Hero with a coffee gradient, a `clamp()` headline, and two CTAs
  ("Upload now" / "Register free"); (2) four-card feature **Bento**; (3) a
  three-step usage section; (4) a CTA band.
- **Components:** `Card` (Bento) + `Button` + `Badge` + `Separator`.
- **Motion:** staggered `fadeUp` for hero copy, `breathe` for the dot pattern, and
  smooth scrolling for anchors.
- **Responsive behavior:** one column on mobile; CTAs span the full width.

### `/login` and `/register`

- **Layout:** a centered `Card` (`max-w` 440) on a warm gradient background;
  field **labels above controls** (`FieldGroup` + `Field`, not placeholders);
  password visibility (`InputGroup` + `InputGroupAddon`); full-width primary
  button with loading state (`Spinner` + `disabled`).
- **CAPTCHA slot:** embedded in the form when `provider != none`; see
  [03](03-features.md#pluggable-captcha-administrator-selected-default-none).
- **Components:** `Card` + `Field` + `Input` + `Button` + `Alert` (errors) + a
  custom dropdown (no native select).
- **Error handling:** `2001` invalid credentials -> red field-level message;
  `2008/429` rate limit -> `Alert` + countdown; `4030` disabled account ->
  `Alert`; `2009/1004` CAPTCHA errors; all copy uses i18n.
- **Flow:** successful login calls `queryClient.clear()` and navigates to
  `/dashboard`; successful registration does not log in automatically and
  navigates to `/login` with a Toast.

## Protected Pages (`AuthGuard`)

### `/dashboard` Dashboard (Role-Segmented)

- **Layout:** top Bento statistics cards, followed by a "Recent images" grid and
  a sidebar containing storage progress plus an administrator system-overview
  card.
- **Data:** `useProfile` for statistics/quota, including `storage_percent`, plus
  `useImages` for recent images. Administrators also load `useAdminStats`
  lazily. `storage_used` always comes from `useProfile`.
- **Components:** the full `Card` composition + `Progress` + `Avatar` + `Chart`
  (admin area) + `Empty` (no images).
- **Role segmentation:** a shared area for everyone, a system-overview card only
  when `role==='admin'`, and an "Open admin" button. Use `React.lazy` so regular
  users do not download administrator code.
- **Empty state:** no images -> `Empty` + "Upload your first image".

### `/images` My Images

- **Layout:** toolbar with search, visibility filter, and grid/list toggle;
  image grid/list; infinite scrolling.
- **Components:** `Input` (search) + custom dropdown / `ToggleGroup` (filter and
  view) + grid `Card` + `Skeleton` + `Empty` + `DropdownMenu` (row actions) +
  `AlertDialog` (delete confirmation).
- **Interaction:** on hover, lift the card and reveal a filename overlay plus
  view/delete actions. Write operations include CSRF.
- **Pagination:** `useInfiniteQuery`; `has_more` determines completion; use a
  bottom sentinel and skeletons, with retry on error.
- **Side effects:** upload/delete/visibility change invalidates `['images']` and
  calls `refreshUser()`.

### `/images/:id` Image Details (Owner Only)

- **Layout:** two columns: a large image on the left, opening a lightbox when
  clicked, and an information panel on the right.
- **Components:** `Card` (direct link, metadata, private token) + `Table` /
  definition list (metadata) + `InputGroup` (direct link with copy
  `InputGroupAddon`) + `Switch` / custom dropdown (visibility) + private-token
  controls: `Badge` (status) + custom dropdown (TTL) + `Button`
  (issue/reissue/revoke) + `Alert` (warning).
- **Lightbox:** clicking the large image opens the full-screen lightbox described
  in [MASTER Section 10](./design-system/MASTER.md).
- **Private-token panel:** current token (masked, with copy), share URL
  `/i/<link>?token=`, TTL selector (1h default, range 10min-72h), and explicit
  issue/reissue/revoke actions with no automatic action. Switching to private
  emits a `warning/tokens_revoked` Toast.
- **Responsive behavior:** one column on mobile, with the image first.

### `/upload` Upload

- **Layout:** large dashed Dropzone for click/drag input with hover bounce,
  options for visibility and tags, and a queue.
- **Components:** Dropzone (token styling) + `ToggleGroup` / custom dropdown
  (visibility) + `Field` + queue items using `Card` + `Progress` + `Badge`
  (status).
- **Validation:** per-file size/type/count; `4012` quota-full `Alert`;
  `4029/429` rate limit; success Toast + `refreshUser`.
- **States:** each item is uploading (`Spinner` + shimmering progress), complete
  (`Badge` success), or failed (retry).

### `/profile` Profile

- **Layout:** account-information card, quota usage, and password-change card.
- **Components:** `Card` + `Badge` (role/status) + `FieldGroup` / `Field`
  (password change) + `Separator`.
- **Flow:** after a successful password change, the backend clears all sessions;
  call `queryClient.clear()`, clear auth-store, navigate to `/login`, and show a
  Toast.
- **Scope:** no avatar or profile-editing endpoint.

## Administrator Pages (`AdminGuard` with Backend `RequireAdmin` 403 Fallback)

### `/admin` Administration Overview

- **Components:** statistics wall built with `Card` + `Chart` (bar chart, lazy
  loaded, honors reduced motion) + `Empty`.
- **Self-healing:** on `4030/4032`, call `refreshUser()` and return to
  `/dashboard`.

### `/admin/users` User Management

- **Layout:** search plus a table on desktop or cards on mobile.
- **Components:** `Table` (desktop, with `ScrollArea` / overflow) + `Badge`
  (role/status, color plus text) + `Avatar` + `Pagination` + `DropdownMenu`
  (suspend/restore) + `AlertDialog` (confirmation) + `Input` (search).
- **Interaction:** setting `suspended` makes the backend clear the user's
  sessions and send a notification. Include CSRF. **Users cannot be deleted.**
- **Responsive behavior:** at widths <=820, replace the table with single-column
  cards. Treat the first cell as the header, expose `data-label` through
  `td::before`, and use a custom scrollbar.

### `/admin/configs` System Configuration

- **Layout:** sectioned card forms for CAPTCHA, private tokens, and other
  settings, plus a sticky save bar.
- **Components:** `Card` + `Field` + custom dropdown (CAPTCHA provider) + `Input`
  (key) + `Slider` / `Input` (TTL) + `Button` (save) + `Sonner` (success).
- **Content:** CAPTCHA `provider` (`none/recaptcha/turnstile/geetest_v4`) + key;
  default private-token TTL within the 600000-259200000 ms bounds; changing the
  provider warns that the corresponding external resource will be enabled.

## Global Components (Not Standalone Pages)

- **Navbar:** sticky and translucently blurred; brand on the left; role-aware
  links; `ThemeToggle` with explicit ☀/🌙 and VT + mask; `NotificationBell` with
  an unread dot anchored inside the button; user menu with `Avatar` +
  `AvatarFallback` + role `Badge` + logout.
  - **Responsive behavior:** at widths <=820, hide the tab bar and use a
    hamburger plus sliding `Sheet` with complete navigation, backdrop, Esc close,
    and close-on-selection. Keep the brand left and actions right.
- **NotificationBell dropdown:** `DropdownMenu` list + unread `Badge` + mark all
  read / clear all actions with CSRF; `Empty` for no notifications.
- **404:** `Empty` + `Button` to return home.
- **Toaster:** `sonner` for success, error, rate-limit, and disabled-account
  messages; dismiss automatically after 3-5s; `aria-live=polite`.
- **Custom dropdown:** replaces native `<select>` with a custom arrow, selected
  checkmark, open/close animation, and outside-click handling.
- **Custom scrollbar:** a thin theme-colored bar instead of the native gray bar.

---

## Landing Behavior and Role Differences

All three roles land on `/dashboard`. Differences are limited to whether the
administrator area is rendered on `/dashboard` and whether `/admin/*` is
allowed. The backend provides the final 403 protection for all unauthorized
access. See [02](02-architecture.md) and
[09](09-decisions-and-scope.md).

<- [09 Decisions and Scope](09-decisions-and-scope.md) - [Index](./README.md) -
Design system: [MASTER](./design-system/MASTER.md)
