# summeRain バックエンド API リファレンス

> 本リファレンスは `backend/` のソース（Go + Gin + GORM/MySQL + Redis +
> imgproxy）とファイル単位で照合されており、フロントエンドとバックエンドの
> 連携仕様として扱われます。
>
> - **ベース URL:** `/api/v1`
> - **既定のポート:** `8080`（`SERVER_PORT`）
> - **画像直接配信ルート:** `GET /i/:link`（`/api/v1` の外部）
> - **検証基準:** `v2.0.0`。初期の V2 リリースは頻繁に変更される可能性があるため、
>   対象バージョンのソースとリリースノートを優先してください。

---

## 目次

1. [一般規約](#1-一般規約)
2. [認証と CSRF](#2-認証と-csrf)
3. [認証 API](#3-認証-api)
4. [画像](#4-画像)
5. [画像アクセストークン](#5-画像アクセストークン)
6. [ユーザー](#6-ユーザー)
7. [通知](#7-通知)
8. [管理](#8-管理)
9. [公開エンドポイント](#9-公開エンドポイント)
10. [エラーコード一覧](#10-エラーコード一覧)
11. [フロントエンド連携上の注意](#11-フロントエンド連携上の注意)

---

## 1. 一般規約

### レスポンスエンベロープ

アプリケーション API エンドポイントは、次の `response.Response` エンベロープを返します。

```json
{
  "code": 0,
  "message": "success",
  "data": { },
  "request_id": "任意。エラー時のみ返される"
}
```

| フィールド | 説明 |
|---|---|
| `code` | `0` は成功を示します。それ以外はアプリケーションエラーコードです。[セクション 10](#10-エラーコード一覧)を参照してください。 |
| `message` | 人が読めるメッセージ |
| `data` | アプリケーションデータ。成功時は存在し、追加データを伴うエラーを除いてエラー時は省略されます。 |
| `request_id` | リクエスト追跡 ID。**エラーレスポンスでのみ返されます。** |

- HTTP ステータスコードはアプリケーションの意味に従います。200 は成功、201 は作成完了、400 は不正なパラメーター、401 は未認証、403 はアクセス拒否、404 は未検出、413 はペイロード超過、429 はレート制限、500 はサーバーエラーです。

### データモデルのフィールド名

バックエンドの JSON は一貫して **snake_case** を使用します。例として
`storage_used`、`created_at`、`view_count`、`unique_link`、`user_id` があります。

### 実行時依存サービス

サービス起動時に MySQL と Redis へ接続できる必要があります。いずれかの `Ping` が失敗すると
`main.go` は `Fatal` を呼び出します。imgproxy は上限付きの V1 動的変換と、透かしが有効な場合の
V2 公開処理を担当します。既存の永続化済みバリアントは直接読み取れます。
インフラストラクチャは `docker-compose.yml` で起動できます。

ヘルスチェックエンドポイント：

- `GET /health` -> `{"status":"ok"}`
- `GET /ready` -> DB と Redis の接続を確認し、利用不可の場合は 503 を返す
- `GET /metrics` -> Prometheus メトリクス

---

## 2. 認証と CSRF

システムは 2 種類の認証方式をサポートします。

### 2.1 Web：Cookie 認証

ログインに成功すると、サーバーは 2 つの Cookie を設定します。

| Cookie | 用途 | HttpOnly | 有効期間 |
|---|---|---|---|
| `__Host-session_token` | セッション資格情報 | はい | 30 日（2592000 秒） |
| `__Host-csrf_token` | CSRF 防御 | いいえ。フロントエンドから読み取り可能 | Cookie Max-Age は 30 日、サーバー記録は 24 時間 |

- Cookie は `__Host-` 接頭辞を使用し、**HTTPS と同一サイトでのデプロイ**、`SameSite=Strict`、`Secure` が必要です。
- フロントエンドは `credentials: 'include'` を指定してリクエストを送信するだけで、ブラウザーが Cookie を自動的に付与します。
- サーバー側の CSRF 記録は、有効な書き込み操作後に更新されます。期限切れの場合は `POST /api/v1/auth/csrf/refresh` で復元できます。フロントエンドが自動で更新して再送するのは、明示的に冪等なリクエストだけです。

### 2.2 CSRF 防御

**Cookie で認証するすべての書き込み操作（POST、PUT、PATCH、DELETE）は、
次のリクエストヘッダーを含める必要があります。**

```text
X-CSRF-Token: <__Host-csrf_token Cookie の値>
```

`middleware/csrf.go` の規則：

- GET、HEAD、OPTIONS リクエストは検証しません。
- `Authorization: Bearer <token>` を使用するデバイスクライアントのリクエストは、CSRF 検証を**省略**します。
- Cookie 認証でヘッダーがない場合は `4035 CSRF token required`、値が一致しない場合は `4036 Invalid CSRF token` を返します。

> フロントエンド実装：`__Host-csrf_token` Cookie を読み取り、GET 以外のすべてのリクエストに
> `X-CSRF-Token` ヘッダーを追加します。

### 2.3 デバイスクライアント：Bearer Token 認証

`Authorization: Bearer <session_token>` を、`X-Platform: android|windows` および
`X-Client-Version` ヘッダーとともに送信します。device-login、bootstrap、heartbeat の各フローは
Web 連携には関係しません。

---

## 3. 認証 API

### 3.1 登録

`POST /api/v1/auth/register`

> [!WARNING]
> Web 専用です。`X-Platform` が存在し、値が `web` でない場合は
> `4034 Registration is restricted to Web clients` を返します。ログイン用レート制限が適用されます。

**リクエスト本文**

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "8 文字以上",
  "captcha": {
    "provider": "recaptcha|turnstile|geetest_v4",
    "token": "recaptcha / turnstile token",
    "action": "register",
    "lot_number": "GeeTest v4",
    "captcha_output": "GeeTest v4",
    "pass_token": "GeeTest v4",
    "gen_time": "GeeTest v4"
  }
}
```

検証条件：`username` は 3～50 文字、`email` は 100 文字以内の有効なメールアドレス、
`password` は 8～72 文字である必要があります。

> `captcha` ペイロードは現在の `captcha_provider` によって異なります。
> [セクション 9.1](#91-公開設定)を参照してください。`provider=none` の場合は省略できます。
> reCAPTCHA には `token` と `action`、Turnstile には `token`、GeeTest v4 には
> `lot_number`、`captcha_output`、`pass_token`、`gen_time` が必要です。
> provider が一致しない場合は拒否されます。既定の `CROSS_ORIGIN_ISOLATION=true` では
> GeeTest v4 を利用できません。[セクション 9.1](#91-公開設定)を参照してください。

**成功：201**

```json
{
  "code": 0,
  "message": "created",
  "data": { "id": 12, "username": "alice", "email": "alice@example.com" }
}
```

> 登録ではログインセッションが**作成されません**。ログインエンドポイントを別途呼び出してください。

### 3.2 ログイン

`POST /api/v1/auth/login`

**リクエスト本文**

```json
{ "username": "alice", "password": "xxxxxx",
  "captcha": { "provider": "recaptcha", "token": "...", "action": "login" } }
```

> `username` にはユーザー名を指定できます。IP とユーザー名のレート制限が適用され、
> 失敗が続くと `2008` を返します。`captcha` ペイロードは[登録](#31-登録)と同じです。

**成功：200** - `__Host-session_token` と `__Host-csrf_token` も設定します。

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "user": { "id": 12, "username": "alice", "role": "user" }
  }
}
```

> `UserSummary` に含まれるのは `id`、`username`、`role` だけです。

### 3.3 現在のユーザー

`GET /api/v1/auth/me`（認証必須）

**成功：200**

```json
{
  "code": 0,
  "data": {
    "id": 12, "username": "alice", "email": "alice@example.com",
    "role": "user", "status": "active", "avatar_url": null,
    "storage_used": 1048576, "storage_quota": 1073741824,
    "image_count": 3, "created_at": "2026-01-01T00:00:00Z", "updated_at": "..."
  }
}
```

> 完全な `model.User` を返します。`password_hash` は `json:"-"` により非表示です。

### 3.4 ログアウト

`POST /api/v1/auth/logout`（認証と CSRF が必須）

両方の Cookie を消去し、サーバー側のセッションを削除します。
`{"code":0,"data":null}` を返します。

### 3.5 CSRF Token の更新

`POST /api/v1/auth/csrf/refresh`（認証必須）

このエンドポイントは、長時間のアップロード中に期限切れとなった CSRF token を復元します。
古い `X-CSRF-Token` は不要ですが、同一オリジンの Web リクエストでなければなりません。
サーバーは `Origin` を検証し、ブラウザーから提供される場合は
`Sec-Fetch-Site: same-origin` も検証します。成功すると `__Host-csrf_token` を再設定します。
更新後に自動再送できるのは、冪等な意味を持つリクエストだけです。

### 3.6 デバイスエンドポイント

Web 連携では、これらのエンドポイントを無視できます。

| メソッド | パス | 説明 |
|---|---|---|
| POST | `/auth/device-login` | デバイスへログインし、identity_token を返す |
| POST | `/auth/device-bootstrap` | identity_token を session_token に交換する |
| POST | `/auth/device-heartbeat` | heartbeat でセッションを維持する |
| DELETE | `/auth/device-shutdown` | デバイスセッションを終了する |
| GET | `/auth/device-identities` | デバイス ID を一覧表示する |
| DELETE | `/auth/device-identities/:id` | ID を失効させる。CSRF 必須 |
| GET | `/auth/sessions` | 有効なセッションを一覧表示する |
| DELETE | `/auth/sessions/:id` | セッションを失効させる。CSRF 必須 |

---

## 4. 画像

> このセクションのすべてのエンドポイントで認証が必要です。画像は所有者に紐付けられます。
> `GET /images/:id` は `image.user_id == 現在のユーザー` を検証し、
> 一致しない場合は `4031 Forbidden` を返します。

### 4.1 カーソルページネーション付き画像一覧

`GET /api/v1/images/`

**クエリパラメーター**

| パラメーター | 既定値 | 説明 |
|---|---|---|
| `cursor` | 空 | 前のレスポンスの `next_cursor` から得たページネーションカーソル |
| `limit` | `20` | 1 ページあたりの項目数 |
| `sort` | `-created_at` | 並び順。`-` 接頭辞は降順を指定する |
| `visibility` | 空 | `public` または `private` で絞り込む |
| `search` | 空 | キーワード検索 |

**成功：200**

```json
{
  "code": 0,
  "data": {
    "images": [ { "id": 1, "user_id": 12, "unique_link": "abc...", "filename": "a.jpg",
                  "title": "", "visibility": "public", "view_count": 5,
                  "width": 800, "height": 600, "file_size": 123456,
                  "created_at": "2026-06-18T10:00:00Z", "updated_at": "..." } ],
    "next_cursor": "次ページのカーソル。続きがない場合は空",
    "has_more": true
  }
}
```

`model/image.go` の `Image` フィールド：

| フィールド | 型 | 説明 |
|---|---|---|
| `id` | uint64 | 画像 ID |
| `user_id` | uint64 | 所有者 |
| `image_file_id` | uint64 | 基礎となるファイルレコード |
| `unique_link` | string | `/i/<unique_link>` の構築に使う一意の短縮リンク |
| `title` / `filename` / `description` | string | メタデータ |
| `visibility` | string | `public` または `private` |
| `pipeline_version` | uint16 | `1` は従来のパイプライン、`2` はクライアント前処理 |
| `processing_status` | string | `pending`、`processing`、`completed`、`failed` |
| `asset_link` | string? | 現在の V2 オリジンエイリアス。非公開画像は `<unique_link>S` を使用する |
| `view_count` | uint64 | 表示回数 |
| `width` / `height` | int | 寸法 |
| `file_size` | int64 | バイト数 |
| `created_at` / `updated_at` | time | タイムスタンプ |

> [!WARNING]
> バックエンドの `Image` モデルには `category` と `tags` フィールドがありません。
> カテゴリーやタグには、バックエンドの拡張またはフロントエンドでのローカル保存が必要です。

### 4.2 V2 クライアント前処理アップロード

V2 は既定で有効です。ブラウザーは静止画の JPG/JPEG、PNG、BMP、WebP、AVIF を受け入れ、
アニメーションを拒否し、4 つの WebP パートを生成します。`master` は元の解像度で Q80、
`gallery` は 400x400 の cover クロップで Q60、`admin` は 120x160 の cover クロップで Q60
（Image Management では 2 倍のピクセル密度を持つ 60x80 CSS ピクセルで表示）、
`publish_source` は長辺 2048 で Q80 です。サーバーはストリーミング中に完全な WebP コンテナーを
受信して検証し、バックグラウンドで最終 `publish` アセットだけに透かしを適用します。

1. `GET /api/v1/uploads/recipe`（認証必須）：現在のレシピ、パート上限、ピクセル上限、セッション TTL を返します。`v2_enabled` は新規アップロードの機能スイッチです。`false` の場合、Web クライアントはローカル前処理を省略して V1 multipart アップロードを使用します。
2. `POST /api/v1/uploads/`（認証と CSRF が必須）：セッションを作成します。64 文字以内の `Idempotency-Key` を送信してください。同じキーで再送できるのは、完全に同一のマニフェストだけです。
3. `PUT /api/v1/uploads/:uploadID/parts/:kind`（認証と CSRF が必須）：レスポンスの `put_url` へ未加工の `image/webp` リクエスト本文をアップロードします。`Content-Length`、SHA-256、寸法、完全な RIFF コンテナーがマニフェストと一致する必要があります。
4. `POST /api/v1/uploads/:uploadID/complete`（認証と CSRF が必須）：`master`、`gallery`、`admin` をアトミックに昇格させ、`publish_source` から `publish` を生成する永続的な公開ジョブを作成します。
5. `POST /api/v1/uploads/status`（認証と CSRF が必須）：1～100 個の `upload_ids` を一括照会します。存在しない、または権限のない ID は部分結果を返さず、一律 404 になります。
6. `GET /api/v1/uploads/:uploadID`（認証必須）：1 件の状態を照会します。同じパスへの `DELETE` で、処理段階に入っていないセッションをキャンセルできます。

**マニフェスト例**

```json
{
  "filename": "photo.jpg",
  "visibility": "public",
  "processor_version": "wasm-vips-0.0.18",
  "recipe_version": "2.0.0",
  "source": { "mime_type": "image/jpeg", "width": 8000, "height": 6000, "animated": false },
  "parts": [
    { "kind": "master", "size": 8200000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 8000, "height": 6000, "quality": 80 },
    { "kind": "gallery", "size": 32000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 400, "height": 400, "quality": 60 },
    { "kind": "admin", "size": 9000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 120, "height": 160, "quality": 60 },
    { "kind": "publish_source", "size": 410000, "sha256": "<64 lowercase hex>", "mime_type": "image/webp", "width": 2048, "height": 1536, "quality": 80 }
  ]
}
```

**セッションレスポンス**

```json
{
  "upload_id": "32-character-url-safe-id",
  "status": "initiated",
  "expires_at": "2026-07-16T12:30:00Z",
  "parts": [
    { "kind": "master", "status": "pending", "put_url": "/api/v1/uploads/.../parts/master", "size": 8200000, "sha256": "...", "width": 8000, "height": 6000 }
  ]
}
```

通常の状態遷移は `initiated`、`uploading`、`processing`、`completed` の順です。
`failed` と `cancelled` は終了状態です。公開後に永続化される 4 つのアクセス用バリアントは
`master`、`gallery`、`admin`、`publish` で、中間の `publish_source` は削除されます。
`image_id`、`unique_link`、`asset_link` を含む `cleanup_pending` レスポンスは、画像が公開済みで
中間ファイルのクリーンアップだけが残っていることを示します。それ以外は失敗した終了状態として扱います。
クライアントのポーリング上限は 10 分です。

`POST /api/v1/images/` は `V2_UPLOAD_ENABLED=false` の場合にのみ V1 multipart 互換エンドポイントとして
残り、V2 が有効な場合は `4262` を返します。レシピエンドポイントは常に利用でき、V2 が無効な場合は
`"v2_enabled": false` を返します。これにより、クライアントは元画像を処理する前に互換パイプラインを選択できます。

### 4.3 画像詳細

`GET /api/v1/images/:id`（認証必須。owner または admin）

一覧項目と同じ構造の `Image` を 1 件返します。**非公開画像**について owner または admin が
リクエストした場合、レスポンスには現在の統一トークンも含まれます。

```json
{ "code": 0, "data": {
    "...Image のフィールド...": "...",
    "access_token": "現在の平文統一トークン。有効なトークンがない場合は省略",
    "token_expires_at": "2026-06-18T11:00:00Z"
}}
```

### 4.4 画像の削除

`DELETE /api/v1/images/:id`（認証と CSRF が必須）

**成功：200**（`DeleteResult`）

```json
{ "code": 0, "data": {
    "image_id": 50,
    "storage_freed_bytes": 123456,
    "storage_used": 1876544,
    "storage_quota": 1073741824
}}
```

### 4.5 公開範囲の変更

`PATCH /api/v1/images/:id/visibility`（認証と CSRF が必須）

**リクエスト本文：** `{ "visibility": "public" }`（`public` または `private` のみ）

**成功：200**（`VisibilityResult`）

```json
{ "code": 0, "data": {
    "image_id": 50,
    "visibility": "public",
    "tokens_revoked": 2,
    "warning": "private から public への変更により、この画像のすべてのアクセストークンが失効しました",
    "asset_link": "現在の V2 公開リンク。private 状態ではファイル名に S が付加される"
}}
```

`tokens_revoked` が 0 より大きくなる可能性があるのは、`private` から `public` への変更時だけです。
`public` から `private` への変更では、V2 `asset_link` が直ちに `S` 接尾辞付きエイリアスへ切り替わります。
CDN が古い公開 URL を保持するのは最長 10 分で、purge イベントも outbox に追加されます。

---

## 5. 画像アクセストークン

各非公開画像が持つ統一トークンは**最大 1 つ**です。トークン値は**変更できません**。
失効後は owner または admin が明示的に新しいトークンを発行する必要があり、それまでは第三者と共有できません。

### 5.1 トークンの発行または再発行

`POST /api/v1/images/:id/tokens`（認証と CSRF が必須。owner または admin）

**リクエスト本文：** `{ "ttl_ms": 3600000 }`（任意）。既定値は
`private_token_ttl_default_ms` から取得し、`[600000, 259200000]` ms、すなわち 10 分～3 日に制限されます。

> トークンを発行すると、画像の既存の有効なトークンが**自動的に無効化**され、
> 1 つだけを有効とする規則を保ったまま、新しい平文トークンが返されます。

**成功：200**（`AccessTokenResult`）

```json
{ "code": 0, "data": {
    "token_id": 7,
    "token": "平文トークン",
    "expires_at": "2026-06-18T11:00:00Z",
    "warning": "このトークンを今すぐ保存してください。値は変更できず、失効後は再発行が必要です。"
}}
```

### 5.2 トークンの失効

`DELETE /api/v1/images/:id/tokens`（認証と CSRF が必須。owner または admin）-> `{ "image_id": 7, "revoked": true }`

> `revoked=false` は、有効なトークンが存在しなかったことを示します。失効後は、
> 新しいトークンを発行するまで第三者と画像を共有できません。

### 5.3 現在のトークン

専用の一覧エンドポイントはありません。owner または admin が `GET /api/v1/images/:id` を呼び出すと、
有効なトークンがある非公開画像のレスポンスに `access_token` と `token_expires_at` が含まれます。
[セクション 4.3](#43-画像詳細)を参照してください。

### 5.4 アップロードキューの状態

`GET /api/v1/upload/queue/:id`（認証必須）は、非同期の `upload_queue` レコードを照会します。

---

## 6. ユーザー

### 6.1 プロフィール

`GET /api/v1/user/profile`（認証必須）

**成功：200**（`model.User` に算出済みの `storage_percent` を追加した `UserProfile`）

```json
{
  "code": 0,
  "data": {
    "id": 12, "username": "alice", "email": "alice@example.com",
    "role": "user", "status": "active", "avatar_url": null,
    "storage_used": 1048576, "storage_quota": 1073741824,
    "storage_percent": 0.09,
    "image_count": 3,
    "created_at": "2026-01-01T00:00:00Z"
  }
}
```

### 6.2 パスワードの変更

`PATCH /api/v1/user/password`（認証と CSRF が必須）

**リクエスト本文：** `{ "old_password": "現在のパスワード", "new_password": "8 文字以上の新しいパスワード" }`

> パスワードの変更に成功すると、**そのユーザーのすべてのセッションが削除**され、
> 再ログインが必要になり、通知が送信されます。現在のパスワードが誤っている場合は `2001` を返します。

---

## 7. 通知

> すべてのエンドポイントで認証が必要で、書き込み操作には CSRF が必要です。
> `model/notification.go` を参照してください。

| メソッド | パス | 説明 |
|---|---|---|
| GET | `/notifications/` | 通知一覧 |
| PATCH | `/notifications/:id/read` | 1 件を既読にする。CSRF 必須 |
| PATCH | `/notifications/batch-read` | すべてを既読にする。CSRF 必須 |
| DELETE | `/notifications/:id` | 1 件を削除する。CSRF 必須 |
| DELETE | `/notifications/clear` | すべてを削除する。CSRF 必須 |

`Notification` のフィールドは `id`、`user_id`、`type`、`title`、`message`、`is_read`、
JSON 文字列の `metadata`、`created_at` です。

---

## 8. 管理

> すべてのエンドポイントで認証、`platform == "web"`、`role == "admin"` が必要です。
> `RequireAdmin` が 3 条件すべてを確認します。admin ルートグループ全体に CSRF ミドルウェアが適用されます。

### 8.1 ページネーション付きユーザー一覧

`GET /api/v1/admin/users?page=1&page_size=20`

**成功：200**（`UserListResult`）

```json
{ "code": 0, "data": {
    "items": [ { /* model.User の全フィールド */ } ],
    "total": 42, "page": 1
}}
```

> `page_size` の上限は 100 です。`items` は `id ASC` 順です。各項目には
> `storage_used`、`storage_quota`、`image_count`、`status` およびその他のユーザーフィールドが含まれます。

### 8.2 ユーザー状態の変更

`PATCH /api/v1/admin/users/:id/status`（CSRF 必須）

**リクエスト本文：** `{ "status": "active" }`

`status` に指定できるのは `active` または `suspended` だけです。`pending_deletion` と
`deleting` はアカウント削除ステートマシンが管理するため、このエンドポイントから直接設定できません。

> `suspended` に設定すると、以後の認証ではそのユーザーの既存セッションが拒否され、
> 実質的に強制ログアウトとなり、「アカウントが無効化されました」という通知が送信されます。
> バックエンドに `banned` 状態はなく、対応する概念は `suspended` です。ユーザーが存在しない場合は `4041` を返します。

### 8.3 ユーザー削除の申請または取消

| メソッド | パス | リクエスト | 説明 |
|---|---|---|---|
| POST | `/admin/users/:id/request-deletion?admin=<管理者のユーザー名>` | `{ "username": "削除対象のユーザー名" }` | `active` の一般ユーザーだけを受け付け、`pending_deletion` に移行して 24 時間後の削除を予定する |
| POST | `/admin/users/:id/cancel-deletion` | なし | `pending_deletion` だけを受け付け、`active` に戻す |

削除を申請すると、対象ユーザーのすべてのセッションが消去されます。ロック期間中、ユーザーは再ログインして
データを一括ダウンロードできますが、アップロード、画像削除、画像変更は `4038` を返します。
期限処理 worker がタスクを取得すると、アカウントは内部の `deleting` 段階に入ります。
以後は認証と業務アクセスが fail-closed となり、取消はできず、物理ファイルの削除は永続 outbox を通じて再試行されます。

重複申請、許可されない移行元状態、同時の状態変更は `4095`、ユーザー名の不一致は `3000` を返します。
管理者アカウントは削除できません。両エンドポイントは成功時に `{"code":0,"data":null}` を返します。

### 8.4 システム統計

`GET /api/v1/admin/stats`

**成功：200**（`SystemStats`）

```json
{ "code": 0, "data": {
    "total_users": 42,
    "total_images": 1024,
    "storage_used": 5368709120,
    "active_users": 38,
    "total_sessions": 17
}}
```

### 8.5 システム設定

| メソッド | パス | 説明 |
|---|---|---|
| GET | `/admin/configs` | すべての設定項目（`config_key`、`config_value`、関連フィールド）を返す |
| PATCH | `/admin/configs` | `{ "items": [ { "key": "...", "value": "..." } ] }` で一括更新する |

---

## 9. 公開エンドポイント

### 9.1 公開設定

`GET /api/v1/public/config`（認証不要）

```json
{ "code": 0, "data": {
    "captcha_provider": "none",
    "captcha_site_key": "",
    "site_language": "en-US"
}}
```

| フィールド | 説明 |
|---|---|
| `captcha_provider` | `none`、`recaptcha`、`turnstile`、`geetest_v4`。admin は `/admin/configs` からキー `captcha_provider` で上書きできる |
| `captcha_site_key` | 現在の provider のクライアント公開鍵。reCAPTCHA/Turnstile の site key、GeeTest の `captcha_id`、または `none` の場合は空 |
| `site_language` | `en-US` や `zh-CN` などのサイト言語。フロントエンドが起動時に言語を選択する |

既定の `CROSS_ORIGIN_ISOLATION=true` では COOP/COEP ヘッダーが送信され、wasm-vips の 50 MP 処理経路が
有効になります。GeeTest v4 の外部スクリプトリソースはこの分離ポリシーを満たさないため、分離が有効な間は
サーバーが `captcha_provider=geetest_v4` を拒否します。`none`、`recaptcha`、`turnstile` を使用してください。
クロスオリジン分離を明示的に無効にすると GeeTest を利用できますが、V2 の大画像処理に必要な分離実行経路が
失われるため、50 MP アップロード環境では推奨しません。

### 9.2 画像直接配信サービス

`GET /i/:link`

- V2 公開画像：`GET /i/<asset_link>.webp`。
- V2 固定バリアント：`GET /i/<asset_link>/master.webp`、`gallery.webp`、`admin.webp`、`publish.webp`。クエリパラメーターから追加サイズは生成されません。`master` と `admin` にアクセスできるのは owner と admin だけです。
- V1 `link` は引き続き `<unique_link>` または `<unique_link>.<ext>` を使用でき、`ext` は webp、avif、jpg、jpeg、png、gif のいずれかです。拡張子なしのリンクは元画像を返します。既存のサイズ指定なし WebP とバックグラウンド AVIF バリアントは直接読み取られ、その他の形式、または 4096 以下の `w`/`h` と `q` を伴うリクエストは上限付き imgproxy 互換経路を使用します。任意サイズの動的結果はリクエスト中だけ一時ファイルを使用します。同一の同時リクエストは集約され、最後のレスポンスが解放した後にファイルを削除します。
- **非公開画像：**
  - `__Host-session_token` または `Bearer` による同一オリジンセッションを持つ owner または admin は、画像トークンなしで直接許可されます。
  - 第三者はクエリパラメーター `?token=xxx`、ヘッダー `X-Image-Token`、または `Authorization: Bearer xxx` を使用できます。
    - 有効なトークンは `no-store` 付きで許可されます。
    - トークンがない、誤っている、または期限切れの場合は **`4037`（403）** を返します。
    - 失効済みの場合は **`4042`（404）** を返します。
- アクセスごとに Redis の `views:<id>` カウンターを非同期に増加させ、`view_flusher` worker が永続化します。
- キャッシュヘッダー：非公開画像は `no-store` を使用します。公開オリジンレスポンスのキャッシュは最長 10 分です。公開範囲の変更時には、永続的な CDN purge イベントも outbox に書き込みます。

### 9.3 公開統計

`GET /api/v1/public/stats`（認証不要）

```json
{ "code": 0, "data": {
    "images": 1024,
    "users": 42,
    "views": 53821,
    "storage_used": 5368709120
}}
```

| フィールド | 説明 |
|---|---|
| `images` | ホストしている画像の総数 |
| `users` | `status=active` の有効な登録ユーザー数 |
| `views` | `SUM(view_count)` による累積表示回数。約 60 秒遅れて永続化される |
| `storage_used` | サイト全体のストレージ使用量（バイト） |

---

## 10. エラーコード一覧

| code | HTTP | 意味 |
|---|---|---|
| 1000 | 500 | 内部サーバーエラー |
| 1001 | 500 | データベースエラー |
| 1002 | 500 | キャッシュサービスエラー |
| 1003 | 500 | 画像処理サービスエラー（imgproxy） |
| 1004 | 503 | reCAPTCHA サービスを利用できない |
| 2001 | 401 | ユーザー名またはパスワードが誤っている |
| 2008 | 429 | ログイン試行回数が多すぎる |
| 2009 | 403 | reCAPTCHA 検証に失敗した |
| 2090 | 429 | bootstrap リクエストが多すぎる |
| 3000 | 400 | パラメーター検証エラー |
| 3001 | 400 | ファイルがない、または ID が無効 |
| 3002 | 413 | ファイルがサイズ上限を超えている |
| 3003 | 415 | 未対応のファイル形式 |
| 3004 | 400 | ファイル数が上限を超えている |
| 3005 | 400/404 | V2 アップロードマニフェスト、upload ID、または part パラメーターが無効 |
| 3006 | 400 | アップロードストリームの読み取り失敗、または R2 URL 設定が無効 |
| 3007 | 422 | アップロード part の SHA-256 検証に失敗した |
| 3008 | 422 | アップロード part の寸法がマニフェストと一致しない |
| 3010 | 400 | 画像寸法が上限を超えている |
| 4010 | 401 | 未認証、またはトークンが無効 |
| 4011 | 401 | セッションの期限切れ |
| 4012 | 403 | ストレージクォータを使い切った |
| 4029 | 429 | アップロードのレート制限を超えた |
| 4030 | 403 | 権限不足、またはアカウントが停止中 |
| 4031 | 403 | デバイス上限に到達、または画像へのアクセス権がない |
| 4032 | 403 | admin エンドポイントは Web クライアント専用 |
| 4033 | 403 | identity_token は API アクセスに使用できない |
| 4034 | 403 | 登録は Web クライアント専用 |
| 4035 | 403 | CSRF token required |
| 4036 | 403 | Invalid CSRF token |
| 4037 | 403 | 非公開画像トークンが無効、または期限切れ |
| 4038 | 403 | アカウント削除ロック期間中の画像書き込みは禁止 |
| 4039 | 403 | 削除ロック期間中の一括ダウンロード許可回数を使い切った |
| 4040 | 404 | 通知が見つからない |
| 4041 | 404 | ユーザーまたはファイルが見つからない |
| 4042 | 404 | 非公開画像トークンが失効済み |
| 4043 | 404 | アップロードセッションが存在しない、期限切れ、またはアクセス不可 |
| 4090 | 409 | Nonce の再利用 |
| 4091 | 409 | アップロードセッションの状態競合 |
| 4092 | 409 | アップロード part が未完了 |
| 4093 | 409 | 画像を処理中、またはクリーンアップ中 |
| 4094 | 409 | R2 ストレージターゲットが過去のファイルまたは保留中のクリーンアップから参照されているため変更不可 |
| 4095 | 409 | 現在のユーザー状態では要求された遷移を実行できない |
| 4261 | 426 | クライアント画像レシピのバージョンが未対応 |
| 4262 | 426 | このデプロイでは V2 クライアント前処理アップロードが必要 |
| 4260 | 426 | クライアントのバージョンが古すぎる |
| 4291 | 429 | アップロード同時実行数、または有効セッション容量を使い切った |
| 5030 | 503 | サーバーストレージの負荷が高すぎる |
| 5031 | 503 | V2 アップロードが一時的に無効 |

---

## 11. フロントエンド連携上の注意

### 11.1 リクエストラッパーの要件

1. **資格情報：** すべてのリクエストに `credentials: 'include'` を設定し、ブラウザーが Cookie を自動送信するようにします。
2. **CSRF：** `__Host-csrf_token` Cookie を読み取り、GET 以外のすべてのリクエストに `X-CSRF-Token` を追加します。
3. **レスポンス処理：** `body.code === 0` を成功とみなし、それ以外は `message` を表示します。401 レスポンスではログイン画面へ移動します。
4. **`__Host-` Cookie の制約：** HTTPS と同一オリジンでのデプロイが必須です。ローカルの `http://localhost` ではブラウザーが Cookie を拒否する場合があるため、同一オリジンプロキシまたは自己署名証明書を使用します。

### 11.2 既存フロントエンド mock のフィールド対応

| フロントエンド mock フィールド | バックエンドフィールド | 備考 |
|---|---|---|
| `userId` | `user_id` | snake_case |
| `uploadedAt` | `created_at` | ISO 8601 文字列 |
| `views` | `view_count` | |
| `size` | `file_size` | |
| `isPublic: boolean` | `visibility: "public"|"private"` | Boolean から文字列へ |
| `id`（string） | `id`（uint64） | 数値 |
| `url`/`thumb` | `/i/<unique_link>` | フロントエンドで直接 URL を構築する必要がある |
| `banned` 状態 | `suspended` | バックエンドに `banned` 状態はない |

### 11.3 バックエンドで未対応のフロントエンド機能

- **公開ギャラリーまたはディスカバリーページ：** バックエンドに公開画像一覧エンドポイントはなく、`images` は現在のユーザーの画像だけを返します。`GET /images/public` を追加するか、フロントエンドからこの機能を削除してください。
- **カテゴリーまたはタグ：** `Image` モデルに `category` と `tags` フィールドはありません。

現在のフロントエンドは、管理者向け画像一覧と削除、ユーザー削除の申請と取消、
`pending_deletion` と `deleting` の表示にすでに対応しており、これらは機能不足ではありません。

---

*本ドキュメントの生成に使用したソース：`cmd/server/main.go`、
`internal/handler/*`, `internal/service/*`, `internal/model/*`,
`internal/middleware/{auth,csrf}.go`、
`internal/pkg/{response,errcode}/*`。*
