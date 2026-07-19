# バックエンド変更計画（設計のみ、実装は含まない）

> [!WARNING]
> **アーカイブ済みの設計記録です。** この文書は、2026 年 6 月 18 日に作成された
> V2 実装前の計画を記録しています。ここに記載したトークンおよび CAPTCHA の作業は、
> その後実装されたか、別の仕組みに置き換えられました。現在の動作については、現行の
> ソース、API リファレンス、リリースノートを正式な情報として参照してください。

- **日付：** 2026-06-18
- **状態：** 設計。執筆時点では未実装
- **範囲：** `docs/design/frontend-architecture/` で定義された 2 つのフロント
  エンド規則をサポートするため、Go バックエンド（`backend/`）に必要な変更を列挙する。
- **原則：** 何を、どこで、なぜ変更するかだけを説明し、実装コードは含めない。

---

## 1. 非公開画像：統一トークンモデル

### 従来の状態（目標を満たしていなかった）

- `internal/service/image_service.go`：
  `GenerateAccessToken/ListTokens/RevokeAccessToken` は、期限付きの
  **複数トークンモデル**を実装していた。
- `internal/handler/public_handler.go::ServeImage`：トークンのない非公開画像
  リクエストは `4010`（401）を返し、**owner/admin セッションのバイパスはなかった**。
  失効済みトークンによるアクセスにも `404` の意味付けがなかった。
- `internal/handler/image_handler.go::Get`：`Image` レスポンスに現在のトークンが
  含まれていなかった。

### フロントエンド設計で採用した目標規則

- 非公開画像ごとに**統一トークンを 1 つ**だけ持ち、トークンの文字列は**不変**とする。
- **TTL：** 既定値は 1h（3,600,000 ms）で、バックエンドが**ミリ秒**単位で検証する。
  owner/admin は発行時に **600,000 ms から 259,200,000 ms** の範囲で指定できる。
- **失効：** owner/admin だけが実行できる。失効後にトークンを**自動再発行しない**。
  owner/admin が明示的に再申請するまで、第三者とは恒久的に共有できない。
- **同一オリジンセッションによる owner/admin の直接表示：** 自動的に認可し、`/i/`
  ではトークンを要求しない。API は表示用に現在の統一トークンを返す。
- 第三者：トークンなし、誤り、期限切れ -> **403**。失効済みトークン -> **404**。

### 必要な変更

**1）モデル / リポジトリ**（`model/image_access_token.go`、
`repository/image_access_token_repo.go`）

- 画像ごとに有効トークンを最大 1 つに制約する。有効トークンとは、失効も期限切れも
  していないトークンを指す。フィールドは文字列、`expires_at`、`revoked_at`、
  `status` とする。
- `FindActiveByImageID(imageID)`、`Issue(imageID, ttlMs)`（統一された有効トークンを
  1 つに保つため、既存の有効トークンを無効化してから発行する）、`Revoke(imageID)`、
  `Validate(imageID, token)` を追加または変更する。`Validate` は `valid`、
  `expired`、`revoked`、`not_found` のいずれかを返す。

**2）サービス層**（`service/image_service.go`）

- 複数トークン用の一連のメソッドを、
  `IssueAccessToken(userID, imageID, ttlMs)`（owner/admin 検証を含む）と
  `RevokeAccessToken(userID, imageID)` に置き換える。
- handler が TTL を渡し、サービス層で `[600000, 259200000]` に clamp する。
  既定値は `3600000` とする。
- 自動再発行の経路をすべて削除する。発行は明示的な操作とする。

**3）画像の直接 URL**（`handler/public_handler.go::ServeImage`）

- 非公開画像の分岐では、最初に**任意のセッション解析**を行う。`middleware` の検索
  ロジックを再利用して `__Host-session_token` / Bearer を読み取るが、認証は必須に
  しない。セッションが owner または admin のものであれば、そのまま許可する。
- それ以外はトークンを検証し、`Validate` の結果で分岐する。`valid` は `no-store`
  付きで許可し、`expired` / `not_found` は **403**、`revoked` は **404** を返す。
- トークンがない場合も **403** とし、従来の `4010` を置き換える。

**4）画像詳細**（`handler/image_handler.go::Get`）

- リクエスト元が owner/admin の場合、`access_token`（現在の統一トークンの平文。
  このエンドポイントだけが返す）と `token_expires_at` をレスポンスに追加する。

**5）エラーコード**（`internal/pkg/errcode/errcode.go`）

- `4037 Private image token is invalid or expired`（403）を追加する。
- `4042 Private image token has been revoked`（404）を追加する。
- `/i/` でトークンがない場合は、従来の `4010` ではなく `4037` を使用する。

**6）設定**（`model/system_config.go` + `/admin/configs`）

- `private_token_ttl_default_ms` を追加し、既定値を `3600000` とする。admin が設定
  できるが、サービス層でも固定境界内に再度 clamp する。
- `600000` / `259200000` の境界は、設定変更で規則を破れないよう**コード定数**とし、
  設定項目にはしない。

**7）`docs/API.md` の更新：** `/i/:link` の非公開アクセス動作（owner/admin バイパスと
403/404）を記載し、`POST /images/:id/tokens` の入力を `ttl_ms` に変更し、
`GET /images/:id` レスポンスに `access_token` を追加する。

---

## 2. CAPTCHA：差し替え可能な 3 Provider

### 従来の状態（目標を満たしていなかった）

- `internal/service/recaptcha.go`（reCAPTCHA v3）だけが存在し、
  `recaptchaVerifier` インターフェース経由で `auth_service` に注入されていた。
- `config.go::RecaptchaConfig` には reCAPTCHA のフィールドしかなく、
  `public_config_service.go` は `recaptcha_enabled` / `recaptcha_site_key` だけを
  返していた。
- `LoginInput` / `RegisterInput` には `recaptcha_token` / `recaptcha_action`
  しかなかった。

### 目標規則

- `captcha_provider` は `none`（既定）、`recaptcha`、`turnstile`、`geetest_v4`
  のいずれかとし、管理者が `/admin/configs` から設定する。
- `none` の場合、バックエンドは**検証を行わず**、フロントエンドもスクリプトを
  読み込まない。
- ログインと登録では選択した provider の payload を受け取り、バックエンドは対応する
  provider の検証処理を呼び出す。

### 必要な変更

**1）設定**（`config.go`）

- `RecaptchaConfig` を `CaptchaConfig` に置き換える。reCAPTCHA のフィールドを
  残したまま、`Provider string`、Turnstile（`SiteKey` / `Secret`）、GeeTest
  （`CaptchaID` / `CaptchaKey`）を追加する。
- 既定値を `Provider=none` とする。
- 環境変数 `CAPTCHA_PROVIDER`、`TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET`、
  `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY` を追加し、従来の
  `RECAPTCHA_*` との互換性を維持する。

**2）検証の抽象化と実装**（`service/`）

- `CaptchaVerifier` インターフェースを抽出する：
  `Verify(ctx, payload, remoteIP, requestHost) *errcode.AppError`。
- `recaptcha.go` の動作を変えず、このインターフェースを実装させる。
- `turnstile.go` を追加する。フォームフィールド `secret` + `response`
  （+ `remoteip`）を `https://challenges.cloudflare.com/turnstile/v0/siteverify`
  へ POST し、`success` を確認する。タイムアウトと失敗は `FailClosed` に従って扱う。
- `geetest_v4.go` を追加する。`lot_number` + `captcha_output` + `pass_token` +
  `gen_time` + `captcha_id` を使用し、`captcha_key` で HMAC-SHA256 署名して
  `https://gcaptcha4.geetest.com/verify` へ POST し、`result=="success"` を要求する。
- provider に応じた実装を返し、`none` では nil を返す `NewCaptchaVerifier(cfg)`
  factory を追加する。

**3）Auth サービス / Handler**（`service/auth_service.go`、
`handler/auth_handler.go`）

- `recaptchaVerifier` の代わりに `CaptchaVerifier` を注入する。
- `LoginInput` / `RegisterInput` を汎用 CAPTCHA payload に拡張する。
  `captcha_provider` に加え、`{recaptcha_token, recaptcha_action}`、
  `{turnstile_token}`、または `{lot_number, captcha_output, pass_token, gen_time}`
  を受け取る。provider に応じたネスト済み `captcha` オブジェクトを推奨する。
- `Login` / `Register` は**設定された provider**のフィールドを検証し、
  `provider=none` では検証を省略する。フロントエンドから送られた provider が設定と
  一致しない場合は拒否する。

**4）公開設定**（`service/public_config_service.go`、
`handler/public_handler.go::GetConfig`）

- `captcha_provider` と、その provider のクライアント公開キーを返す。reCAPTCHA /
  Turnstile は `site_key`、GeeTest は `captcha_id` を使用する。`provider=none` の場合、
  公開キーは空にする。

**5）レート制限とエラー**

- 既存のログインレート制限を再利用する。`2009`（検証失敗）と `1004`（サービス利用不可）
  は変更しない。
- brute-force 攻撃を抑えるため、検証失敗もレート制限のカウントに含める。

**6）`docs/API.md` の更新：** `/public/config` の新フィールド、ログイン / 登録の
多形態 CAPTCHA payload、`/admin/configs` の新しい CAPTCHA 設定を記載する。

---

## 3. データベース移行（AutoMigrate は構造変更を処理するが、データには注意が必要）

- `image_access_tokens`：`revoked_at` を追加し、一意性制約を画像ごとに 1 つの有効
  トークンへ変更する。既存の複数トークンデータには、最新の有効トークンだけを残すか、
  すべてを失効させる cleanup script が必要となる。
- `system_configs`：`private_token_ttl_default_ms`、`captcha_provider`、各 provider
  の key レコードを追加する。
- `cmd/server/main.go::AutoMigrate` に加えて、既存トークンを整理し、既定の設定値を
  書き込む**データ移行スクリプト**を用意する。

## 4. 対象外

- 実装コード、unit test、CI の変更。これらは設計承認後に別途計画する想定だった。
- フロントエンド実装。`frontend-architecture/` の各項目で定義済み。

## 5. リスクとトレードオフ

- **非公開画像の owner/admin バイパス：** 公開ルート `/i/:link` には任意のセッション
  解析が必要となる。このルートを必須認証の配下に置くと第三者のトークンアクセスが
  失敗するため、セッションの解析を試み、owner/admin の場合だけトークン検証を
  バイパスする。
- **ミリ秒 TTL 検証：** Go は内部でナノ秒を使用する。精度誤差を避けるため、保存と
  比較では一貫して `UnixMilli` を使用する。
- **GeeTest 署名アルゴリズム：** 署名文字列の結合順序と HMAC-SHA256 を含め、公式 v4
  文書に厳密に従う。使用前に公式 demo と照合して実装を検証する。
- **中国での可用性：** `recaptcha.net` mirror を使用しても、中国本土では reCAPTCHA
  が不安定である。主要な利用者向けには `turnstile` または `geetest_v4` を優先する。
