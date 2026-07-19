# コントリビューションガイド

summeRain に関心をお寄せいただきありがとうございます。本ガイドでは、プロジェクトへの貢献方法を説明します。

## 開発環境

### 必要条件

- Go 1.24 以降
- フロントエンドのビルド用 Node.js `^20.19.0 || >=22.12.0`。CI では 24.18.0 LTS を使用します
- MySQL、Redis、imgproxy の開発用依存サービスだけを実行するための Docker と Docker Compose
- ローカル HTTPS 証明書を生成するための OpenSSL

### WSL でバックエンドを起動する

```bash
./scripts/dev-wsl.sh deps-up   # MySQL、Redis、imgproxy を起動し、アプリケーションイメージはビルドしない
./scripts/dev-wsl.sh backend   # Go サービスを直接実行する
```

アプリケーションは既定で `127.0.0.1:18080` をリッスンします。ヘルスチェックエンドポイントは
`GET http://127.0.0.1:18080/health` です。

### フロントエンド開発サーバーを起動する

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend  # https://127.0.0.1:5173。/api と /i を同一オリジンでプロキシする
```

### フロントエンドをバックエンドへビルドする

```bash
cd frontend
npm run build                  # 出力先：../backend/web/
```

## コーディング規約

### Go バックエンド

- [Effective Go](https://go.dev/doc/effective_go)、`gofmt`、`go vet` に従ってください。
- 新機能には `_test.go` ファイルのテストを含めてください。
- import は、標準ライブラリ、サードパーティパッケージ、`github.com/kserksi/summerain/...` 配下のプロジェクトパッケージの順に並べてください。
- 各ソースファイルに著作権ヘッダー `// Copyright 2026 The summeRain Authors` を残してください。

コミット前に次のチェックを実行します。

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

### TypeScript フロントエンド

- 既存の React 19、TanStack Query、shadcn/ui スタックを使用してください。
- 新機能には Vitest のテストを含めてください。
- 既存の ESLint 設定に従ってください。

コミット前に次のチェックを実行します。

```bash
cd frontend
npm run lint
npm run build
npx vitest run
```

## コミット規約

[Conventional Commits](https://www.conventionalcommits.org/) 形式を使用します。

```text
<type>(<scope>): <description>

[optional body]
```

一般的な type は `feat`、`fix`、`docs`、`refactor`、`test`、`chore`、`i18n` です。

例：

```text
feat(upload): add drag-and-drop reordering
fix(auth): handle expired session on token refresh
i18n(extract): move navbar strings to locale files
```

## Pull Request を提出する

1. ローカルの `dev` ブランチを更新し、そこから作業ブランチを作成します。Pull Request の対象は `dev` に設定します。
2. `go build`、`npm run build`、関連するテストが成功することを確認します。
3. Pull Request の説明に、変更の目的と影響範囲を記載します。
4. UI を変更した場合はスクリーンショットを添付します。
5. レビューしやすいよう、1 件の Pull Request は 1 つの関心事に絞ります。

## プロジェクト構成

```text
backend/    Go + Gin + GORM (MySQL) + Redis + imgproxy
frontend/   React 19 + Vite + TypeScript + Tailwind + shadcn/ui
docs/       API 契約、デプロイガイド、アーキテクチャ記録
```

過去のフロントエンドアーキテクチャ記録は
[docs/design/frontend-architecture/README.md](docs/design/frontend-architecture/README.md) に索引されています。
アーカイブ済みと記載された内容は、設計経緯の確認目的でのみ保持されています。

## ドキュメントの翻訳

リポジトリルートの英語ドキュメントを正式版とします。完全な簡体字中国語版と日本語版は、それぞれ
`translations/zh-CN/` と `translations/ja-JP/` にあり、正式版ページと同じ相対パスを維持します。
英語ドキュメントを変更する場合は、同じ変更内で両方の翻訳も更新し、その後に翻訳元ハッシュを更新して
GitBook ドキュメント検証を実行してください。

```bash
bash scripts/update-translation-source-hashes.sh
bash scripts/verify-gitbook-docs.sh
```

## 行動規範

本プロジェクトに参加することで、[行動規範](CODE_OF_CONDUCT.md)に従うことへ同意したものとみなされます。思いやりと敬意を持って参加してください。
