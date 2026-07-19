# summeRain デプロイ・利用ガイド

> このガイドは `backend/` のソースコードとデプロイ設定に基づき、**デプロイ、設定、
> 運用、日常的な利用方法**を説明します。完全な API 契約については
> [`API.md`](./API.md) を参照してください。

---

## 目次

1. [プロジェクト概要](#1-プロジェクト概要)
2. [クイックスタート](#2-クイックスタート)
3. [設定リファレンス（環境変数）](#3-設定リファレンス環境変数)
4. [アーキテクチャとデプロイ](#4-アーキテクチャとデプロイ)
5. [運用ランブック](#5-運用ランブック)
6. [利用ガイド](#6-利用ガイド)
7. [制限値としきい値](#7-制限値としきい値)
8. [セキュリティ要点](#8-セキュリティ要点)

---

## 1. プロジェクト概要

summeRain はセルフホスト型の画像ホスティング・フォトアルバムサービスです。

- **バックエンド：** Go 1.26 + Gin + GORM（MySQL）+ Redis。imgproxy は V1
  互換パスを提供し、V2 公開時のウォーターマークを適用します。
- **フロントエンド：** React + Vite。ビルド成果物は Go サービスから同一オリジンで
  配信されます。
- **主な機能：** ユーザー登録とログイン、画像のアップロードと管理、公開/非公開の
  可視性、非公開画像用アクセストークン、マルチデバイスセッション（Web の Cookie +
  デバイストークン）、通知、管理画面、閲覧数統計、サムネイル、形式変換。

### コンポーネントの役割

| コンポーネント | 役割 |
|---|---|
| `backend`（Go） | 業務 API、`/i/:link` の画像直接配信、SPA フォールバック |
| MySQL | ユーザー、画像、セッション、通知、設定などの永続化 |
| Redis | レート制限カウンター、nonce 再利用防止、閲覧数のバッファリング |
| imgproxy | ローカルファイルシステムをソースとする V1 動的互換処理と V2 公開ウォーターマーク |
| 前段 nginx | TLS 終端、リバースプロキシ、レート制限、実 IP の転送 |

---

## 2. クイックスタート

### 2.1 ローカル開発

```bash
# ターミナル 1：固定バージョンの MySQL / Redis / imgproxy だけを Compose で起動
./scripts/dev-wsl.sh deps-up
./scripts/dev-wsl.sh backend

# ターミナル 2：初回起動前に frontend/ で npm ci を実行
./scripts/dev-wsl.sh frontend
```

- バックエンドは既定で `127.0.0.1:18080` を待ち受けます。ヘルスチェックは
  `GET http://127.0.0.1:18080/health` -> `{"status":"ok"}` です。
- フロントエンドは既定で `https://127.0.0.1:5173` を待ち受け、同一オリジンから
  `/api/` と `/i/` をバックエンドへプロキシします。
- 初回起動時に、チェックサム付きデータベースマイグレーションと互換モデルマイグレーションが
  自動実行されます。

> ローカルの `http://localhost` では、`__Host-` プレフィックスの Cookie に必要な HTTPS と
> 同一オリジンの条件を満たさないため、ブラウザーは Cookie の設定を拒否します。
> ローカル結合テストでは自己署名証明書または同一オリジンプロキシを使用してください。

### 2.2 本番デプロイ（GitHub Actions イメージ）

```bash
# GitHub Actions が公開した正確なバージョンを使用する。このファイルはコミットしない
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# backend/.env を編集し、少なくともイメージバージョン、データベースパスワード、
# Cookie シークレット、imgproxy キーを置き換える

docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build
```

アプリケーションイメージをビルドし、Docker Hub / GHCR へ同期するのは GitHub Actions
だけです。デプロイ先ホストではアプリケーションイメージをビルドしません。
`backend/docker-compose.deploy.yml` は `DOCKER_IMAGE` がない場合に拒否します。
本番環境では正確な SemVer タグまたは OCI マルチプラットフォームインデックス
ダイジェストを使用してください。

本番環境では前段 nginx が `127.0.0.1:8080` へリバースプロキシし、Cloudflare を
介してポート 443 を公開します。セクション 4 を参照してください。

---

## 3. 設定リファレンス（環境変数）

参照元：`internal/config/config.go`。**既定値**がある項目は明示的に設定しなくても
構いません。

### 3.1 サービス

| 変数 | 既定値 | 説明 |
|---|---|---|
| `SERVER_PORT` | `8080` | HTTP 待受ポート |
| `GIN_MODE` | `debug` | `debug` / `release`。本番では `release` を使用 |
| `COOKIE_SECRET` | `change-me-in-production` | 予約済み。現在のセッションは不透明なランダム文字列を使い、実際には署名していないが、強い値を推奨 |
| `CROSS_ORIGIN_ISOLATION` | `true` | COOP/COEP を送信し、wasm-vips の大画像処理パスを有効化。無効にするとブラウザー固有の安全しきい値を超える画像を処理できない |
| `GOMEMLIMIT` | `512MiB`（Compose） | 640 MiB のコンテナ制限内に Go ヒープの目標値を抑え、スタック、ネイティブメモリ、ランタイム用の余裕を確保 |

`CROSS_ORIGIN_ISOLATION=true` の場合、サードパーティのスクリプト、フォント、画像は CORS
または `Cross-Origin-Resource-Policy` で埋め込みを明示的に許可する必要があります。
許可がなければブラウザーは COEP に従って遮断します。50MP アップロードという目標は、
このモードで有効になる分離済み wasm-vips パスに依存します。

### 3.2 データベース（MySQL）

| 変数 | 既定値 | 説明 |
|---|---|---|
| `DB_HOST` | `mysql` | ホスト |
| `DB_PORT` | `3306` | ポート |
| `DB_USER` | `root` | **本番では `summerain.*` だけに権限を持つ制限付きアカウントを使用** |
| `DB_PASSWORD` | *（空）* | 必須 |
| `DB_NAME` | `summerain` | データベース名 |
| `DB_MAX_OPEN_CONNS` | `8` | データベースの最大オープン接続数 |
| `DB_MAX_IDLE_CONNS` | `4` | データベースの最大アイドル接続数。オープン接続数の上限を超えてはならない |
| `DB_CONN_MAX_LIFETIME` | `30m` | 再利用する接続の最大寿命 |

> DSN：`user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local`

### 3.3 キャッシュ（Redis）

| 変数 | 既定値 | 説明 |
|---|---|---|
| `REDIS_ADDR` | `redis:6379` | アドレス |
| `REDIS_PASSWORD` | *（空）* | プライベートネットワーク内では空でもよい。ネットワークをまたぐ場合は設定を推奨 |
| `REDIS_DB` | `0` | データベース番号 |
| `REDIS_POOL_SIZE` | `8` | Redis クライアント接続プールの上限 |

Compose は Redis のデータ上限を `128mb`、コンテナ上限を `192m` に設定し、
`noeviction` を使用します。上限に達すると書き込みが明示的に失敗するため、レート制限や
リプレイ防止状態の暗黙的なエビクションと、コンテナの OOM を防止できます。

### 3.4 画像処理（imgproxy）

| 変数 | 既定値 | 説明 |
|---|---|---|
| `IMGPROXY_URL` | `http://imgproxy:8080` | 内部アドレス |
| `IMGPROXY_KEY` | *（空）* | 16 進数。署名を有効にするため**必須** |
| `IMGPROXY_SALT` | *（空）* | 16 進数。**必須** |
| `IMGPROXY_PUBLIC_URL` | `/img` | 公開署名 URL のプレフィックス |
| `IMGPROXY_WORKERS` | `2` | V2 公開ワーカーと揃えた処理並行数の上限 |

### 3.5 ストレージ

| 変数 | 既定値 | 説明 |
|---|---|---|
| `STORAGE_PATH` | `/data/images` | V1 の original/thumbnail/processed ファイルと固定 V2 バリアントを格納する永続画像ディレクトリ |
| `TEMP_PATH` | `/data/images/.staging` | V1 一時処理ディレクトリ。Compose と V2 は同じ制限付きステージングボリュームを共有 |
| `V2_STAGING_PATH` | `<STORAGE_PATH>/.staging` | V2 アップロード用ステージングディレクトリ。アトミックな昇格のため `STORAGE_PATH` の子でなければならない |
| `DISK_SOFT_LIMIT_PERCENT` | `80` | 使用率がこの値を超えると新しい V2 アップロードセッションを拒否 |
| `DISK_HARD_LIMIT_PERCENT` | `90` | 使用率がこの値を超えるとアップロードパートまたは公開成果物への追加書き込みを拒否 |

### 3.6 V2 アップロードと公開

| 変数 | 既定値 | 説明 |
|---|---|---|
| `V2_UPLOAD_ENABLED` | `true` | 固定レシピの V2 セッションアップロードを有効化。V1 マルチパートアップロードは無効時のみ利用可能 |
| `V2_RECIPE_VERSION` | `2.0.0` | クライアントとサーバーで完全一致が必要な処理レシピのバージョン |
| `V2_MAX_PART_BYTES` | `67108864` | 1 つの WebP パートの上限（64 MiB） |
| `V2_MAX_PIXELS` | `50000000` | ソース画像とパートのピクセル上限（50 MP） |
| `V2_SESSION_TTL` | `30m` | 未完了アップロードセッションの有効期間 |
| `V2_GLOBAL_UPLOAD_CONCURRENCY` | `8` | 1 バックエンドインスタンスが同時受信できるパートの全体上限 |
| `V2_PER_USER_UPLOAD_CONCURRENCY` | `4` | 1 ユーザーが同時受信できるパートの上限 |
| `V2_WATERMARK_CONCURRENCY` | `2` | 公開/ウォーターマークワーカー数。3 コア、4 GB の共有ホストにおける上限 |
| `V2_JOB_POLL_INTERVAL` | `1s` | 公開ジョブのポーリング間隔 |
| `V2_JOB_LEASE` | `2m` | 公開ジョブのリース。ワーカーは更新し、フェンシングトークンを使ってコミット |

ブラウザーのアップロードパイプラインの並行数は 2 ですが、画像のデコードとエンコードは
直列です。アクティブなサーバー側セッションの上限は 4 で、別タブや復旧リクエスト用の
バックエンド容量を残します。サーバー公開と imgproxy はそれぞれ既定で 2 ワーカーのため、
同一のウォーターマークスナップショットを並列処理できます。同じホスト上の別コンポーネントで
CPU またはメモリ負荷が継続する場合は、両方の並行数を 1 に下げてください。
中間の `publish_source` とセッションステージングファイルは公開後に削除されます。

### 3.7 CDN と永続アウトボックス

| 変数 | 既定値 | 説明 |
|---|---|---|
| `CDN_PUBLIC_BASE_URL` | *（空）* | パージ配信を有効にする場合に必須の公開画像ベース URL |
| `CLOUDFLARE_ZONE_ID` / `CLOUDFLARE_API_TOKEN` | *（空）* | Cloudflare パージ用認証情報。必ず組で設定 |
| `CDN_PURGE_WEBHOOK_URL` / `CDN_PURGE_WEBHOOK_TOKEN` | *（空）* | Cloudflare 以外の CDN 用汎用パージ webhook |
| `OUTBOX_BATCH_SIZE` | `10` | 1 バッチで取得する永続イベント数 |
| `OUTBOX_POLL_INTERVAL` | `2s` | アウトボックスのポーリング間隔 |
| `OUTBOX_LEASE` | `3m` | イベント配信リース |
| `CDN_PURGE_REQUESTS_PER_SECOND` | `4` | CDN パージリクエストレートの上限 |
| `CDN_PURGE_REQUEST_TIMEOUT` | `15s` | 1 回のパージリクエストのタイムアウト |

### 3.8 CAPTCHA（差し替え可能・任意）

`CAPTCHA_PROVIDER` は `none`（既定）、`recaptcha`、`turnstile`、`geetest_v4`
のいずれかです。管理者は `/admin/configs` の `captcha_provider` キーから上書きできます。

| 変数 | 既定値 | 説明 |
|---|---|---|
| `CAPTCHA_PROVIDER` | *（空、導出）* | 空 + `RECAPTCHA_ENABLED=true` -> `recaptcha`。それ以外は `none` |
| `RECAPTCHA_ENABLED` | `false` | 後方互換性のため残された reCAPTCHA 全体スイッチ |
| `RECAPTCHA_SITE_KEY` / `RECAPTCHA_SECRET` | *（空）* | reCAPTCHA の公開鍵/シークレット |
| `RECAPTCHA_MIN_SCORE` | `0.5` | v3 の最小スコア |
| `RECAPTCHA_VERIFY_URL` | `https://www.recaptcha.net/recaptcha/api/siteverify` | 中国本土向けミラーを使う検証エンドポイント |
| `RECAPTCHA_FAIL_CLOSED` | `true` | 上流サービスが利用できない場合にリクエストを拒否するか |
| `RECAPTCHA_ALLOWED_HOSTNAMES` | *（空）* | 許可するホスト名。カンマ区切り |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET` | *（空）* | Cloudflare Turnstile |
| `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY` | *（空）* | GeeTest v4 |

> `provider=none` の場合、バックエンドは検証を行わず、フロントエンドもスクリプトを
> 読み込みません。`/api/v1/public/config` は現在のプロバイダーと対応する公開鍵を
> 返します。

> 既定の `CROSS_ORIGIN_ISOLATION=true` では `geetest_v4` を利用できません。COEP が
> 外部スクリプトリソースを遮断し、サーバーもこの設定での起動または管理画面からの
> プロバイダー切り替えを拒否します。`none`、`recaptcha`、`turnstile` を使用してください。
> クロスオリジン分離を明示的に無効にした場合だけ GeeTest を利用できますが、
> V2 で 50MP 画像を処理するための分離済み wasm-vips パスが失われます。

---

## 4. アーキテクチャとデプロイ

### 4.1 リクエスト経路

```text
利用者 --HTTPS--> Cloudflare --> nginx(:443) --HTTP--> backend(:8080, 127.0.0.1)
                                          \- TLS 終端 / レート制限 / 実 IP 転送
backend --> MySQL / Redis / imgproxy（Docker プライベートネットワーク）
```

### 4.2 nginx の重要設定（本番）

- `set_real_ip_from <Cloudflare range>; real_ip_header CF-Connecting-IP;` で実際の
  クライアント IP を復元します。
- `proxy_set_header X-Forwarded-For $remote_addr;` は追加せず**上書き**し、XFF
  偽装を防ぎます。
- `location = /metrics { return 404; }` で Prometheus メトリクスの公開範囲を抑えます。
- `client_max_body_size 20m;` を 1 リクエストあたりのアップロードサイズ上限に合わせます。
- セキュリティヘッダー：`HSTS` / `X-Content-Type-Options` / `X-Frame-Options` /
  `Content-Security-Policy`。

### 4.3 コンテナ構成（Docker Compose）

4 つのサービスはすべてプライベートネットワーク内だけで通信させることを推奨します。

| サービス | ポート公開 | 説明 |
|---|---|---|
| `backend` | `127.0.0.1:8080` のみにバインド | アプリケーション。**非 root（UID 10001）**で実行 |
| `mysql` | 公開しない | プライベートネットワークのみ |
| `redis` | 公開しない | プライベートネットワークのみ |
| `imgproxy` | 公開しない | 画像ボリュームを読み取り専用でマウント |

> 本番用シークレットは、Git で除外される `backend/.env` にモード 0600 で保存します。
> デプロイコマンドには `--env-file backend/.env` も必ず渡し、Compose の変数展開と
> バックエンドコンテナーが同じ設定を使用するようにしてください。

---

## 5. 運用ランブック

### 5.1 ヘルスチェック

| エンドポイント | 意味 |
|---|---|
| `GET /health` | プロセスが稼働中 -> `{"status":"ok"}` |
| `GET /ready` | DB + Redis の接続を確認。利用不可の場合は 503 |
| `GET /metrics` | Prometheus メトリクス。本番では nginx でアクセスを制限 |

### 5.2 バックグラウンドワーカー

`worker.Manager` が `internal/worker/` から次のワーカーを並列起動します。

| ワーカー | 間隔 | 役割 |
|---|---|---|
| `heartbeat` | 5 分 | ハートビートがタイムアウトしたデバイスセッションを期限切れにする |
| `view_flusher` | 60 秒 | Redis の `views:*` カウンターをデータベースへ書き出す |
| `cleanup` | 1 時間 | 期限切れセッション/CSRF/アクセストークン、失敗したアップロード記録、孤立した一時ファイルを削除 |
| `v2_publish` | 継続。既定並行数 2 | `publish_source` から最終公開画像を生成し、ウォーターマークを適用 |
| `v2_cleanup` | 継続・増分 | 期限切れセッション、ステージングディレクトリ、孤立した V2 ファイルを回収 |
| `outbox` | 既定 2 秒 | CDN パージイベントとローカル/R2 物理削除イベントを配信 |
| `user_deletion` | 5 分 | 期限到来済みアカウント削除を小さく復旧可能なバッチで実行 |

すべてのワーカーに `recover` があり、1 回のパニックでマネージャー全体が停止することは
ありません。

### 5.3 ログ

- アプリケーション：`docker logs summerain-backend`（標準出力）。
- nginx：`/var/log/nginx/your-domain.*.log`。

### 5.4 イメージ更新 / ロールバック

```bash
# GitHub Actions が公開した新しい正確なバージョンへアップグレード
# backend/.env を編集し、DOCKER_IMAGE を jaykserks/summerain:v2.0.1 に設定
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull backend
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build backend

# 直前の動作確認済み不変バージョンへロールバック
# backend/.env を編集し、DOCKER_IMAGE を jaykserks/summerain:v2.0.0 に戻す
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml pull backend
docker compose --env-file backend/.env -f backend/docker-compose.deploy.yml up -d --no-build backend
```

ダイジェスト単位で固定する場合は、`DOCKER_IMAGE` を
`jaykserks/summerain@sha256:<oci-index-digest>` に設定します。マルチプラットフォーム
デプロイでは OCI インデックス/マニフェストリストのダイジェストを固定してください。`amd64` または
`arm64` 固有の子マニフェストのダイジェストを別のアーキテクチャに再利用してはいけません。

> 非 root イメージへ切り替える前に、データボリュームで `chown -R 10001:10001` を
> 実行してください。新規作成した名前付きボリュームはイメージ内 `/data` の所有者を
> 継承します。

### 5.5 最小権限のデータベースアカウント

本番用に制限付きアプリケーションアカウントを作成します。

```sql
CREATE USER 'image_gallery'@'%' IDENTIFIED BY '<strong-random-value>';
GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,DROP,ALTER,INDEX,REFERENCES,
      CREATE TEMPORARY TABLES,LOCK TABLES,EXECUTE,CREATE VIEW,SHOW VIEW,TRIGGER
  ON summerain.* TO 'image_gallery'@'%';
FLUSH PRIVILEGES;
```

その後、`DB_USER` / `DB_PASSWORD` をこのアカウントへ向け、アプリケーションが
データベース全体の root 権限を持たないようにします。

---

## 6. 利用ガイド

> 完全なリクエスト/レスポンスフィールドは [`API.md`](./API.md) を参照してください。
> 以下では日常的なワークフローの要点を説明します。

### 6.1 アカウント

- **登録：** Web のみ（`POST /api/v1/auth/register`）。`username` は 3-50 文字で、
  `email` と 8-72 文字の `password` も必要です。登録後に**自動ログインしません**。
- **ログイン：** `POST /api/v1/auth/login`。成功すると
  `__Host-session_token`（30 日）と `__Host-csrf_token` Cookie を設定します。
- **パスワード変更：** `PATCH /api/v1/user/password`。成功すると**そのユーザーの
  すべてのセッションを削除**し、再ログインを強制して通知を送信します。

### 6.2 アップロードと直接リンク

1. Web クライアントは 15 MiB、50 MP 以下の静止 JPG/JPEG、PNG、BMP、WebP、AVIF を
   受け付けます。V2 初回リリースではアニメーション画像を拒否します。
2. ブラウザーは画像ごとに `master`（元解像度、Q80）、`gallery`（400x400、Q60）、
   `admin`（120x160、Q60）、`publish_source`（長辺 2048、Q80）を生成し、
   `/api/v1/uploads/*` を通じてパートごとにアップロードします。
3. バックエンドは `master`、`gallery`、`admin` を確定します。バックグラウンドワーカーは
   `publish_source` から任意でウォーターマーク付きの `publish` アセットを作成した後、
   `publish_source` とセッション中間ファイルを削除します。Image Management は
   120x160 の `admin` ファイルを CSS で 60x80 として表示し、2x ピクセル密度を
   保持します。
4. V2 公開直接リンクは `/i/<asset_link>.webp`、固定バリアントは
   `/i/<asset_link>/{master|gallery|admin|publish}.webp` です。クエリパラメーターから
   追加の V2 サイズは生成されません。
5. Web クライアントは最初に `/api/v1/uploads/recipe` から `v2_enabled` 機能フラグを
   読み取ります。`V2_UPLOAD_ENABLED=false` の場合だけクライアント前処理を省略し、
   `POST /api/v1/images/` から V1 互換マルチパートアップロードを使用します。任意サイズを
   含む V1 動的変換は上限付き一時ファイルを使用し、アクセスキャッシュとして
   永続化しません。
6. 同一内容は SHA-256 で重複排除され、`reference_count` が物理ファイルのライフサイクルを
   管理します。

メインサービスは履歴画像の一括移行エンドポイントを公開しません。互換運用中、未分類の
V1 画像は安全なローカルパスを最初に使用します。ローカル原本が存在せず、現在の R2 ターゲット
が完全に利用できる場合だけ、その正確なターゲットを試します。未分類の履歴レコードが残る間は
エンドポイント/バケットを変更できません。履歴データの検証、チェックポイント、監査、ロールバックは、
後に別リポジトリで提供する移行ツールが担当します。

### 6.3 公開 / 非公開

- 各画像の `visibility` は `public` または `private` です。
- **非公開画像**には、クエリ `?token=`、ヘッダー `X-Image-Token`、または
  `Authorization: Bearer` で渡すアクセストークンが必要です。
- `POST /api/v1/images/:id/tokens` でトークンを発行します。有効期間は 10 分から
  3 日です。平文は発行レスポンスと画像詳細で `owner`/`admin` にだけ返されます。
- `private` から `public` へ切り替えると、**その画像のすべてのトークンを自動的に失効**
  させます。

### 6.4 複数デバイス

- Web：Cookie 認証。書き込み操作には `X-CSRF-Token` ヘッダーが必要です。
- デバイス（android/windows）：`device-login` が 90 日有効な `identity_token` を返します。
  nonce 再利用防止付きの `device-bootstrap` で、15 分有効かつハートビートで延長される
  `session_token` と交換します。各プラットフォームで最大 3 台まで利用できます。

### 6.5 管理

管理エンドポイント群では、`role=admin`、`platform=web`、および全体での CSRF 保護が
必要です。

- ユーザー一覧と状態変更。`suspended` に設定するとすべてのデバイスを強制ログアウトします。
- システム統計の表示と、ウォーターマーク項目
  `watermark_enabled/text/position/opacity` などのシステム設定変更。

---

## 7. 制限値としきい値

| 項目 | 値 | 参照元 |
|---|---|---|
| V2 ソースファイル上限 | 15 MiB | `frontend/src/features/images/pages/Upload.tsx` |
| V2 画像ごとのピクセル上限 | 50 MP | `config.go` / `v2_upload_types.go` |
| 固定 V2 配信用バリアント | `master`、400x400 `gallery`、120x160 `admin`、長辺 2048 `publish` | `v2_upload_types.go` |
| V1 マルチパート上限 | 1 ファイル 10 MiB、1 リクエスト最大 20 ファイル | `image_service.go` |
| 既定ストレージクォータ | 500 MiB（524288000 バイト） | `model.User` |
| クォータ警告しきい値 | 90% | `image_service.go` |
| 画像短縮リンク | V2 は 12 桁の 16 進数。V1 は既定 12 桁で、連続衝突後に 16 桁へ切り替え | `generateUniqueLink` |
| Web セッション | 30 日 | `auth_service.go` |
| CSRF 有効期間 | 24 時間（スライド更新） | `auth_service.go` |
| デバイス識別情報 | 90 日 | `auth_service.go` |
| デバイスセッション | 15 分（ハートビート更新、猶予 600s） | `auth_service.go` / `model.Session` |
| プラットフォームごとのデバイス数 | 最大 3 | `auth_service.go` |
| ログインレート制限 | IP：15 分に 5 回。`username`：15 分に 3 回 | `auth_service.go` |
| Bootstrap レート制限 | 1 分に 10 回 | `auth_service.go` |
| アクセストークン有効期間 | 10 分から 3 日 | `image_service.go` |
| V1 動的変換サイズパラメーター | `w/h` は 4096 以下 | `public_handler.go` |
| V2 アップロード形式 | 静止 jpg/jpeg/png/bmp/webp/avif | `sniff.ts` / `v2_upload_types.go` |

---

## 8. セキュリティ要点

- **認証：** セッショントークンは SHA256 ハッシュだけを保存します。Cookie は `__Host-`
  プレフィックス、`SameSite=Strict`、`Secure`、`HttpOnly` を使用します。CSRF は二重
  送信方式で、Bearer リクエストでは省略します。
- **パスワード：** DefaultCost の bcrypt。
- **パス：** `NoRoute` の静的配信は `filepath.Clean` とベースディレクトリのプレフィックス
  検査を使用し、パストラバーサルを防ぎます。
- **レート制限：** ログイン/Bootstrap の制限は IP + `username` に基づきます。本番環境では
  nginx のレート制限と拒否リストが境界の主な防御です。
- **画像：** 拡張子と MIME スニッフィングの両方を検証します。XSS 防止のため SVG は
  `application/octet-stream` として強制ダウンロードします。非公開画像は `no-store` を
  使用します。
- **コンテナ：** 非 root（UID 10001）。MySQL/Redis のポートは公開せず、データベース
  アカウントは最小権限にします。
- **通信：** Cloudflare Full (Strict) + nginx HSTS。

> `GIN_MODE=debug` では、エラーレスポンスに内部情報が含まれる場合があります。本番環境では
> 必ず `release` を使用してください。

---

*参照元：`cmd/server/main.go`、`internal/config/config.go`、
`internal/{handler,service,middleware,model,worker}/*`、`Dockerfile`、
`docker-compose*.yml`。*
