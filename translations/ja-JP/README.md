# summeRain

> リソースを考慮したアップロードパイプライン、固定画像バリアント、透かし、非公開共有、および既存 V1 画像との互換性を備えた、セルフホスト型の画像ホスティング・フォトアルバムサービスです。

[![CI と Docker](https://github.com/kserksi/summeRain/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kserksi/summeRain/actions/workflows/ci.yml)
[![リリース](https://img.shields.io/github/v/release/kserksi/summeRain)](https://github.com/kserksi/summeRain/releases)
[![Docker Hub](https://img.shields.io/docker/v/jaykserks/summerain?label=docker)](https://hub.docker.com/r/jaykserks/summerain)
[![ライセンス：Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://github.com/kserksi/summeRain/blob/main/LICENSE)

> **早期リリースに関する注意：** V2 はまだ早期リリースです。アップロードプロトコル、ブラウザー処理パイプライン、スキーマ、互換動作、運用上の既定値は頻繁に変更される可能性があります。更新ごとに変更履歴を確認し、MySQL と画像ボリュームをバックアップし、本番環境では正確なリリースタグまたは OCI インデックスダイジェストを固定してください。

[オンラインドキュメント](https://summerain-1.gitbook.io/summerain/ja/) | [V2.0.0 リリースノート](./docs/releases/v2.0.0.md) | [Docker Hub](https://hub.docker.com/r/jaykserks/summerain) | [GHCR](https://github.com/kserksi/summeRain/pkgs/container/summerain)

## 概要

summeRain は、モジュール化された Go モノリスと React シングルページアプリケーションを使用します。Go サービスは `/api/v1/*` API を公開し、`/i/*` で画像を配信し、同一オリジンからコンパイル済みフロントエンドをホストします。MySQL はアプリケーションとジョブ状態の正式な保存先です。Redis は上限付きキャッシュ、レート制限、表示回数のバッファリング、デバイスのリプレイ防止を提供します。imgproxy は上限付き V1 動的変換と、任意の V2 透かし処理を担当します。

```text
ブラウザー / Android / Windows
            |
            v
      Go + Gin (:8080)
        |-- /api/v1/* --> middleware --> handler --> service --> repository
        |-- /i/*
        |     |-- V2 --> 認可 --> 固定ローカルアセット
        |     `-- V1 --> 保存済みアセットまたは上限付き imgproxy 変換
        |-- 公開 worker --> 任意の imgproxy 透かし --> ローカル公開アセット
        `-- /*        --> backend/web (React SPA)
                                      |
                         +------------+------------+
                         |            |            |
                         v            v            v
                     MySQL 8.4     Redis 8    ローカル画像ボリューム
```

## V2 画像パイプライン

V2 は、負荷の高いデコード、リサイズ、形式変換、圧縮をブラウザーへ移します。バックエンドは固定レシピに従いマニフェストで宣言された WebP パートを受け入れ、ステージングストレージへストリーミングしながら検証し、検証だけを目的とした再読み取りを行わずに固定バリアントへ昇格させます。

| アセット | 形状 | 品質 | ライフサイクルと用途 |
|---|---|---:|---|
| `master` | 向き補正後の元の寸法 | 80 | 永続化するフル解像度 WebP。owner/admin がアクセス可能 |
| `gallery` | 400x400 cover クロップ | 60 | 「マイ画像」とダッシュボードの永続プレビュー |
| `admin` | 120x160 cover クロップ | 60 | 画像管理用の永続プレビュー。60x80 で表示 |
| `publish_source` | 長辺が最大 2048 px | 80 | サーバー側公開処理の一時入力 |
| `publish` | `publish_source` から派生 | 80 | 任意の透かしを持つ永続共有アセット |

公開後に `publish_source` とセッションのステージングデータを削除します。サーバーが透かしを適用するのは最終 `publish` アセットだけです。V2 は初回アクセス時に任意サイズを生成しないため、ホットリソースの急増時に V1 を脆弱にした無制限のサムネイル処理を回避します。

### アップロード動作

- 静止画の JPEG、PNG、BMP、WebP、AVIF 入力に対応します。
- V2.0.0 はアニメーション画像と GIF アップロードに対応しません。
- 元ファイルの上限は 15 MiB、50 MP です。
- ブラウザー処理の同時実行数は 1、アップロードパイプラインは 2 です。
- パートアップロードは同時実行数 2 で開始し、能力のあるクライアントでは 3 まで適応できます。
- アップロードセッションは再開可能かつ冪等で、状態ポーリングは永続化されます。クライアントのポーリング期限は 10 分で、その後も状態確認を再開できます。
- 高容量のブラウザー経路は `wasm-vips` を使用します。必要なブラウザー分離機能またはメモリ能力がない場合は、上限付き Canvas/Pica 経路を使用します。
- 既存 V1 画像は元のリンクから引き続き読み取れ、上限付き動的変換も維持されます。

## 機能

### 画像とストレージ

- 不変でコンテンツアドレス指定された `master`、`gallery`、`admin` アセットと、リビジョン単位の `publish` アセット。
- テキスト、位置、不透明度、サイズ、色を設定できるサーバー側テキスト透かし。
- SHA-256 コンテンツ ID と参照カウント付き物理ファイルライフサイクル。
- V2 アセットのローカル保存と、V1 互換経路でストレージ系統情報に基づくローカル/R2 読み取り。
- CDN purge とローカル/R2 物理削除を永続 outbox で配信。
- 削除保留ロック期間中に、ZIP をアプリケーションメモリ上で組み立てずアカウントアーカイブをストリーミング。
- ストレージクォータ、通知、ディスク負荷に応じた受付制御、上限付きクリーンアップ。

### アカウント、プライバシー、共有

- Secure、HttpOnly の `__Host-session_token` と、ブラウザーから読み取れ、サーバーに記録され、CSRF リクエストヘッダーで送信される `__Host-csrf_token`。
- Android および Windows クライアント向け Bearer token bootstrap とセッション。
- 非公開画像ごとに 1 つの有効な共有トークン。有効期間は 10 分～3 日で設定可能。
- ユーザーと管理者の役割、セッション管理、監査ログ、永続的な遅延アカウント削除ワークフロー。
- reCAPTCHA v3 と Cloudflare Turnstile の任意統合。GeeTest v4 はクロスオリジン分離を明示的に無効にした場合だけ利用可能。
- 公開/非公開オリジンエイリアスの即時切り替えと、永続 CDN purge 処理。

### Web と運用

- 認証、ダッシュボード、アップロードキュー、画像管理、プロフィール、通知、管理画面。
- 英語、簡体字中国語、日本語のインターフェースリソース。
- ライト/ダークテーマと、動きを抑える設定への対応。
- `/health`、`/ready`、`/metrics` 運用エンドポイント。
- advisory lock と不変チェックサムを使用するバージョン付き MySQL マイグレーション。
- GitHub Actions だけで構築し、Docker Hub と GHCR へ公開する `linux/amd64`、`linux/arm64` マルチプラットフォームイメージ。

## 技術

| レイヤー | スタック |
|---|---|
| フロントエンド | React 19、React Router 8、Vite 8、TypeScript 6、Tailwind CSS 4、shadcn/ui |
| フロントエンドデータ | TanStack Query 5、Zustand 5、React Hook Form 7、Zod 4 |
| バックエンド | Go、Gin、GORM、MySQL 8.4、Redis 8 |
| 画像処理 | wasm-vips、Pica、Web API、imgproxy 4 |
| ストレージ | V2 ローカルファイルシステム。V1 系統情報向け Cloudflare R2 および path-style 互換 S3 エンドポイント |
| テスト | Go testing、Vitest、Testing Library、MSW |

CI、サービス、ブラウザー処理の正確なバージョンは [`requirements.lock`](https://github.com/kserksi/summeRain/blob/main/requirements.lock) に記録されています。Go と npm の依存関係グラフは `backend/go.sum` と `frontend/package-lock.json` でロックされています。

## WSL クイックスタート

### 必要条件

- Go 1.24 以降。CI とコンテナービルドは現在 Go 1.26.5 を使用します。
- Node.js `^20.19.0 || >=22.12.0`。CI とコンテナービルドは現在 Node.js 24.18.0 LTS を使用します。
- Docker Engine と Docker Compose。
- ローカル HTTPS 証明書の生成に使用する OpenSSL。

### 1. 開発用依存サービスを起動する

```bash
./scripts/dev-wsl.sh deps-up
```

固定バージョンの MySQL、Redis、imgproxy コンテナーを起動します。summeRain アプリケーションイメージはローカルでビルドしません。

| サービス | 既定のアドレス |
|---|---|
| MySQL | `127.0.0.1:13306` |
| Redis | `127.0.0.1:16379` |
| imgproxy | `127.0.0.1:18081` |

ポートは `SUMMERAIN_DEV_MYSQL_PORT`、`SUMMERAIN_DEV_REDIS_PORT`、`SUMMERAIN_DEV_IMGPROXY_PORT` で変更できます。

### 2. バックエンドを起動する

```bash
./scripts/dev-wsl.sh backend
```

API は既定で `http://127.0.0.1:18080` をリッスンします。初回起動時に未適用のデータベースマイグレーションを実行します。ポートは `SUMMERAIN_DEV_BACKEND_PORT` で変更できます。

### 3. フロントエンドを起動する

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend
```

開発サーバーは既定で `https://127.0.0.1:5173` を使用し、`/api/` と `/i/` をローカルバックエンドへプロキシします。初回利用時に生成された開発証明書を承認してください。`__Host-` セッション Cookie には HTTPS と同一オリジンプロキシが必要です。

開発環境では、50 MP wasm-vips 経路のために COOP/COEP が既定で有効です。そのため、すべてのサードパーティスクリプト、フォント、画像は互換性のある CORS または Cross-Origin-Resource-Policy ヘッダーを提供する必要があります。

### 4. 検証を実行する

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

```bash
cd frontend
npm run lint
npm run build
npx vitest run
```

## 本番デプロイ

アプリケーションイメージは GitHub Actions でビルドされます。本番ホストは公開済みの正確なタグまたは OCI インデックスダイジェストを取得し、必ず `--no-build` を使用してください。

```bash
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# backend/.env を編集し、正確な DOCKER_IMAGE、データベースパスワード、
# Cookie secret、imgproxy key/salt を設定する。

docker compose --env-file backend/.env \
  -f backend/docker-compose.deploy.yml pull

docker compose --env-file backend/.env \
  -f backend/docker-compose.deploy.yml up -d --no-build
```

安定版イメージの例：

```text
jaykserks/summerain:2.0.0
```

公開先レジストリ：

- Docker Hub：`jaykserks/summerain`
- GHCR：`ghcr.io/kserksi/summerain`

複数アーキテクチャでダイジェストを固定する場合は OCI マルチプラットフォームインデックスダイジェストを使用してください。特定アーキテクチャ専用の子 manifest ダイジェストを `amd64` と `arm64` の両ホストで再利用しないでください。

環境、nginx/CDN、ヘルスチェック、アップグレード、ロールバックの完全なリファレンスは[デプロイと利用方法](./docs/USAGE.md)を参照してください。

## リリースチャンネル

- `main` への通常の push は `edge` と `sha-<12-character-commit>` を公開します。
- `2.0.0` のような安定版 `VERSION` は `v2.0.0`、`2.0.0`、`2.0`、`2`、`latest`、commit タグを公開します。
- `2.1.0-rc.1` のようなプレリリースは正確なバージョンタグと commit タグを公開し、安定版エイリアスは更新しません。
- 正確なセマンティックバージョンタグは不変です。移動可能なエイリアスは引き続き移動できます。
- リリースの再実行では検証済み manifest ダイジェストを使って Docker Hub と GHCR を調整し、正確なタグが競合する場合は停止します。
- 公開成功後、ルート README を Docker Hub に同期します。

完全なリリース契約は[リリースとタグ管理](./docs/RELEASING.md)を参照してください。

## リソースプロファイル

既定の Compose プロファイルは、他のサービスと共有する 3 コア、4 GiB のホストを想定しています。

| サービス | CPU 上限 | メモリ上限 |
|---|---:|---:|
| バックエンド | 0.75 CPU | 640 MiB |
| MySQL | 0.75 CPU | 1024 MiB |
| Redis | 0.15 CPU | 192 MiB |
| imgproxy | 0.70 CPU | 512 MiB |

バックエンドは、パートアップロードを全体で 8、ユーザーごとに 4 まで同時実行します。MySQL と Redis のプールには上限があります。既定のディスク使用率 80% のソフト上限では新しい V2 セッションを拒否し、90% のハード上限では新しいパートと公開出力を拒否します。ホストで継続的な競合が起きる場合は、imgproxy と透かし worker を 2 から 1 に減らしてください。

これらの制限は保守的な出発点であり、あらゆる環境の容量を保証するものではありません。ディスク遅延、データベース遅延、透かしの複雑さ、同居するワークロードがスループットに影響します。

## リポジトリ構成

```text
summeRain/
|-- backend/
|   |-- cmd/server/                 アプリケーションのエントリポイント
|   |-- internal/                   handler、service、repository、worker
|   |-- migrations/                 バージョン付き SQL マイグレーション
|   `-- web/                        生成されたフロントエンドビルド出力
|-- frontend/
|   |-- src/features/               ドメイン指向のアプリケーション機能
|   |-- src/components/             共有 UI とレイアウト
|   |-- src/lib/                    API、CSRF、エラー、ユーティリティ
|   |-- src/store/                  ユーザーとテーマの状態
|   `-- src/i18n/                   英語、中国語、日本語のリソース
|-- docs/                           API、運用、リリース、アーキテクチャ
|-- translations/                   簡体字中国語と日本語のドキュメントミラー
|-- scripts/                        開発とリポジトリ検証
|-- .gitbook.yaml                   GitBook Git Sync 設定
`-- SUMMARY.md                      GitBook ナビゲーション
```

バックエンドリクエストは次の境界に従います。

```text
request -> middleware -> handler -> service -> repository -> MySQL / Redis
                                      `-> filesystem / R2 / imgproxy
```

## ドキュメント

日本語オンラインドキュメントは
[summerain-1.gitbook.io/summerain/ja](https://summerain-1.gitbook.io/summerain/ja/) で公開されます。
Git で追跡されるすべてのプロジェクトドキュメントは、[`SUMMARY.md`](./SUMMARY.md) の GitBook ナビゲーションソースに含まれます。主なリファレンスは次のとおりです。

- [デプロイと利用方法](./docs/USAGE.md)
- [API リファレンス](./docs/API.md)
- [リリースとタグ管理](./docs/RELEASING.md)
- [フロントエンドアーキテクチャ](./docs/design/frontend-architecture/README.md)
- [スキーママイグレーション](./backend/migrations/README.md)
- [コントリビューションガイド](./CONTRIBUTING.md)
- [セキュリティポリシー](./SECURITY.md)

英語が正式なドキュメント言語です。レビュー可能な簡体字中国語版と日本語版は、`translations/zh-CN` と `translations/ja-JP` の下で同じページパスをミラーし、GitBook によって同一ドキュメントサイトの言語バリアントとして公開されます。

本リポジトリが GitBook Git Sync の正式な情報源です。README とナビゲーションの変更は Git で管理し、GitBook エディターで重複する README ページを作成しないでください。

## 既知の制限

- V2.0.0 が受け付けるのは静止画像だけです。アニメーション画像への対応は今後の予定です。
- V2 は固定 WebP バリアントを永続化し、任意の動的リサイズ組み合わせは公開しません。
- V2 は元のエンコード済みソースバイトを保持しません。`master` はフル解像度、品質 80 の WebP です。
- 既存 V1 画像は自動変換されず、V2 バリアントも割り当てられません。
- 過去の R2 移行は独立した移行ツールへ意図的に分離され、メインサービスでは実行しません。
- クロスオリジン分離を無効にすると GeeTest v4 を利用できますが、高容量の wasm-vips ブラウザー経路は使えなくなります。

## コントリビューションとセキュリティ

Pull Request を作成する前に [CONTRIBUTING.md](./CONTRIBUTING.md) を読み、プロジェクト内では [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) に従ってください。セキュリティ上の問題は公開 Issue ではなく、[SECURITY.md](./SECURITY.md) の非公開手順で報告してください。本リポジトリでは GitHub の非公開[セキュリティアドバイザリーフォーム](https://github.com/kserksi/summeRain/security/advisories/new)を利用できます。

## ライセンス

Copyright 2026 The summeRain Authors

[Apache License 2.0](https://github.com/kserksi/summeRain/blob/main/LICENSE) に基づいてライセンスされています。
