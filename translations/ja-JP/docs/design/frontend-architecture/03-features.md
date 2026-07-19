# 03 · 機能とページ

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

## 機能領域と Query/Mutation

| 機能 | 主な Query / Mutation | ルート / ページ |
|---|---|---|
| auth | `useMe` `useLogin` `useRegister` `useLogout` | `/login` `/register` |
| images | `useImages`（無限スクロール）`useImage` `useUpload` `useDeleteImage` `useToggleVisibility` `useImageAccessToken` `useRevokeAccessToken` | `/images` `/images/:id` `/upload` |
| captcha | `usePublicConfig` `useCaptcha` | 起動時の確認 + ログイン／登録への埋め込み |
| user | `useProfile` `useChangePassword` | `/profile` |
| notifications | `useNotifications` `useMarkRead` `useMarkAllRead` `useDeleteNotification` `useClearNotifications` | ヘッダーの `NotificationBell` ドロップダウン |
| admin | `useAdminUsers`（ページネーション）`useSetUserStatus` `useAdminStats` `useConfigs` `useUpdateConfigs` | `/admin` `/admin/users` `/admin/configs` |

## ランディングページとダッシュボードの遷移先

- **`/`（公開）:** 常にマーケティング用ランディングページ（ヒーロー、機能、CTA）とし、バックエンドに該当エンドポイントがないため公開ギャラリーは**設けません**。
- **ログイン済みユーザーが `/` を訪れた場合**、自動的に **`/dashboard`** へリダイレクトします。
- **`/dashboard`（保護されたコンソールの遷移先）:** ログイン後の遷移先で、`auth.user.role` に応じて条件付きで描画します。
  - 共通領域（全ユーザー）: 個人統計、ストレージ割り当て、最近の画像（`useProfile`、`useImages`）。
  - 管理者領域（`role==='admin'` のみ）: システム概要カード（`useAdminStats`）と `/admin` への「管理画面を開く」リンク。この領域は遅延読み込みし、一般ユーザーはダウンロードしません。
- **ロールの区別:** 遷移先を分けず、条件付き描画とルートガードで実現します。`/admin/*` は `AdminGuard`（フロントエンドの `role` チェック）とバックエンドの `RequireAdmin`（403 を返す）で保護し、一般ユーザーは URL を直接入力しても入れません。

## 主な操作フロー（成功後の動作）

- **登録 `useRegister`:** バックエンドはユーザーを**自動ログインしません**（API.md）。成功後は `/login` へ移動し、「登録が完了しました。ログインしてください」というトーストを表示します。ユーザー名は事前入力しても構いません。
- **パスワード変更 `useChangePassword`:** バックエンドは**すべてのセッションを消去し**（API.md）、現在のクッキーを直ちに無効化します。成功後は `queryClient.clear()` を明示的に呼び出し、`auth-store` を消去し、`/login` へ移動して「パスワードを変更しました。もう一度ログインしてください」と表示します。後続の 401 フォールバックを待って、無効なセッションで UI が停止する状態を避けます。
- **公開範囲の切り替え `useToggleVisibility`:** 画像を非公開にすると、その画像の**共有トークンが無効になります**（バックエンドは `tokens_revoked`/`warning` を返します）。成功後に `warning` が空でなければ Toast/Alert で表示し、`['images']` と `['images', id]` を無効化します。詳細レスポンスには `access_token` が含まれるため、トークンも更新されます。
- **アップロード／削除:** 成功後に `['images']` を無効化し、割り当てと件数が変わるため `refreshUser()` を呼び出します（[02](02-architecture.md)のスナップショット更新機構を参照）。

## 画像詳細 `/images/:id`

大きな画像、メタデータ、公開範囲の制御、非公開画像では共有トークン管理を表示します。レコードを取得できるのは所有者だけです（バックエンド認可）。

## 非公開画像（共有トークンモデル）

`visibility=private` のときの `/i/:link` **アクセス規則**:

| リクエスト元 | 動作 |
|---|---|
| 所有者または管理者（同一オリジンのセッション） | **自動的に認可**され直接表示（`<img src="/i/<link>">` にトークンは不要）。API は表示／共有用に画像の現在の共有トークンを返す |
| **有効かつ期限内**のトークンを持つ第三者 | `no-store` でアクセスを許可 |
| トークンが**ない、誤っている、または期限切れ**の第三者 | **403** を返す |
| **失効済み**トークンを持つ第三者 | **404** を返す |

**トークン規則:**
- 非公開画像ごとに共有トークンは**1 つ**です。存在中はトークンの**文字列を変更できません**。
- **TTL:** 既定は **1 時間（3,600,000 ms）**で、バックエンドが**ミリ秒**単位で検証します。`owner` / `admin` は生成時に **10 分（600,000 ms）から 72 時間（259,200,000 ms）**の範囲で期間を選べます。期限切れトークンは無効で、第三者には 403 を返します。
- **失効:** `owner` / `admin` だけが失効できます。**失効しても置き換えを自動発行しません**。`owner` / `admin` が**手動で再度申請**する必要があります。それまでは第三者へ共有できませんが、`owner` / `admin` セッションでは直接表示できます。
- 共有 URL: 公開画像 = `/i/<link>`、非公開画像 = `/i/<link>?token=<トークン>`。トークンを URL に含めることでログ／リファラーに露出する可能性がありますが、既定の設計として受け入れます。

**フロントエンドの動作:**
- `owner` / `admin` の詳細ページには、コピー可能な現在のトークンと共有 URL、TTL セレクター（既定 1h、範囲 10min–72h、値は `constants.ts` に集約）、明示的な「トークンを生成／再発行」「トークンを失効」ボタンを表示します。自動処理は行いません。
- 第三者が共有 URL を開くと、バックエンドが `?token=` を直接検証し、フロントエンドのセッションは関与しません。
- 一覧／詳細では、所有者自身の非公開画像を同一オリジンのセッション認可で描画し、トークンは不要です。
- UI 文言は 403（権限なし、トークン無効／期限切れ）と 404（トークン失効済み）を区別します。
- フック: `useImageAccessToken(id)` は現在のトークンを取得／表示し、選択した TTL を `POST .../tokens` で送って生成／再発行します。`useRevokeAccessToken(id)` は失効処理を行います。

<a id="pluggable-captcha-administrator-selected-default-none"></a>
## CAPTCHA（差し替え可能、管理者が選択、既定は `none`）

- `captcha_provider` は `none`（既定）、`recaptcha`、`turnstile`、`geetest_v4` のいずれかで、管理者が `/admin/configs` から設定します。
- 起動時に公開 `GET /public/config` を確認し、`captcha_provider` とそのプロバイダーのクライアントキーを取得します。`provider=none` では**外部スクリプトを一切読み込みません**（[07 外部リソースのローカル化](07-production-standards.md#local-external-resources)を満たします）。
- 有効な場合、ログイン／登録ページに `<Captcha>` を埋め込み、プロバイダーごとの分岐で描画します。`useCaptcha()` はフォーム送信用にそのプロバイダーの検証ペイロードを返します。
- エラーは `2009`（検証失敗）と `1004`（サービス利用不可）に統一し、文言は i18n から提供します。

**プロバイダー比較:**

| 観点 | reCAPTCHA v3 | Cloudflare Turnstile | Geetest CAPTCHA v4 |
|---|---|---|---|
| クライアントスクリプト | `google.com/recaptcha/api.js?render=<site_key>`（または recaptcha.net ミラー） | `challenges.cloudflare.com/turnstile/v0/api.js` | `static.geetest.com` 配下の `gcaptcha4.js` |
| クライアント公開キー | `site_key` | `site_key` | `captcha_id` |
| 証跡取得 | `grecaptcha.execute(siteKey,{action})` -> トークン | `turnstile.render(el,{sitekey,action,callback})` -> トークン | `initGeetest4({captcha_id,product})` -> `{lot_number,captcha_output,pass_token,gen_time}` |
| 送信ペイロード | `token` + `action` | `token` | `lot_number` + `captcha_output` + `pass_token` + `gen_time` |
| サーバー検証 | POST `…/api/siteverify`（`secret` + `response` + `remoteip`） | POST `challenges.cloudflare.com/turnstile/v0/siteverify`（`secret` + `response`） | POST `gcaptcha4.geetest.com/verify`（`captcha_id` + 4 パラメーター、`captcha_key` による HMAC 署名） |
| 判定 | `success` & `score≥しきい値` & `action` & `hostname` | `success` | `result=='success'` |
| 中国での利用性 | 低い | 良好 | 良好（国内プロバイダー） |

> **ペイロードエンベロープ（バックエンドの `CaptchaPayload` と整合。[API.md](../../API.md) §3 を参照）:** ログイン／登録リクエスト本文に `captcha: { provider, token, action, lot_number, captcha_output, pass_token, gen_time }` を埋め込みます。現在の `provider` に対応するフィールドだけを設定します。`recaptcha` -> `token` + `action`、`turnstile` -> `token`、`geetest_v4` -> 4 パラメーターです。プロバイダーが一致しなければ拒否されます。`recaptcha` の `action` は、サーバーの `ExpectedAction` と一致する **`"login"` または `"register"`** でなければなりません。`auth_service.go` がログインと登録にそれぞれの値を注入します。エラーコード `2009` と `1004` はプロバイダーに依存しません。`ErrRecaptcha*` という名前は維持しますが、数値コードは正しいものです。

## ダッシュボードのデータソース（明示）

`/dashboard` には専用エンドポイントがなく、2 つの Query を組み合わせます。
- **個人統計 + 割り当て** -> `useProfile`（`GET /user/profile`。`storage_percent` と `image_count` を含む）。
- **最近の画像** -> `useImages` の最初のページ（先頭の数件に限定）。

`auth-store` と `/auth/me` は**本人情報と `role`**（ガード、ヘッダーのユーザー名）にのみ使用し、表示データの参照元にはしません。`storage_used` は 3 か所に現れるため、曖昧さを避けて `useProfile` を一貫して信頼できる情報源とします。

## バックエンド機能との整合

すべての機能は `docs/API.md` のエンドポイント、フィールド、ページネーション、エラーコードに従います。公開ギャラリー、カテゴリー／タグ、管理者による画像審査、ユーザー削除など、バックエンドが対応しない機能は本フロントエンドの対象外です。[09 明示的な対象外](09-decisions-and-scope.md#explicit-exclusions-yagni)を参照してください。

**既知のバックエンド制限:**
- **通知にはページネーションも上限もありません**（`GET /notifications` はページネーションパラメーターを受け取りません。API.md §7 を参照）。返された一覧を直接描画し、件数が多い場合はフロントエンドでスクロール／省略表示します。将来バックエンドにページネーションが追加されたら `useInfiniteQuery` へ切り替えます。

---

<- [02 アーキテクチャ](02-architecture.md) · [索引](./README.md) · 次: [04 テーマと UI](04-theme-and-ui.md)
