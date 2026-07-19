# 01 · 概要と技術スタック

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

## 背景

既存のフロントエンドはネイティブ JavaScript で実装されています（リポジトリルートの `index.html`、`css/`、`js/`。localStorage のモックとコーヒーテーマのライト／ダークモードを含みます）。バックエンドには、Go、Gin、MySQL、Redis、imgproxy で構築された本番品質の画像ホスティングサービスがすでに用意されています。

**目標:** React 19、Vite 8、TypeScript でフロントエンドを書き直し、実際のバックエンド API に直接接続して、モックとネイティブ実装を置き換えます。

## 必須制約

以下の制約は、バックエンド `cmd/server/main.go` の `NoRoute` ハンドラーに由来します。

- バックエンドは SPA モードで `./web/*` の静的アセットを配信し、`./web/index.html` にフォールバックします。
- したがって、フロントエンドは `backend/web/` に置く**静的成果物**としてビルドし、バックエンドと**同一オリジン**でデプロイする必要があります。
- 認証には cookie（`__Host-session_token` と `__Host-csrf_token`）を使用し、同一オリジンの HTTPS が必要です。このため、独立した Node プロセスを必要とする SSR 構成は対象外です。

## 機能スコープ

バックエンドの機能に正確に合わせ、**認証、マイ画像、プロフィール、通知、管理**の 5 領域を対象とします。バックエンドが対応していない公開ギャラリーの閲覧、カテゴリー／タグ、管理者による画像審査、ユーザー削除は含みません。

## 技術スタック

> 以下のバージョンは、2026-06-18 に各パッケージの npm `dist-tag` にある安定版 `latest` と照合済みです。`package.json` は `^` 範囲を使用し、`package-lock.json` が正確なバージョンを固定して再現可能なインストールを保証します。

| 分類 | 採用技術 |
|---|---|
| フレームワーク | React 19 + TypeScript 6（`strict`） |
| ビルド | Vite 8 + @vitejs/plugin-react 6 |
| コンポーネントライブラリ | shadcn/ui 4（[04 テーマと UI](04-theme-and-ui.md)を参照。Tailwind v4 対応） |
| スタイル | Tailwind CSS 4（**CSS ファースト**: `@theme` 変数によるテーマ、`tailwind.config.js` なし） |
| ルーティング | React Router 8（**宣言的（Declarative）モード** + BrowserRouter） |
| サーバー状態 | TanStack Query 5 |
| クライアント状態 | Zustand 5（テーマ + 現在のユーザーのみ） |
| フォーム | React Hook Form 7 + Zod 4（@hookform/resolvers 経由） |
| i18n | react-i18next 17 + i18next 26（既定は `en-US`、中国語と日本語を同梱） |
| 本番ビルド拡張 | vite-plugin-sri3 2（SHA512 SRI を注入）+ ビルドマニフェスト |
| 定数管理 | 集約された定数モジュール（マジック値を排除） |
| テスト | Vitest 4 + @testing-library/react 16 + MSW 2 |
| 開発用 HTTPS | @vitejs/plugin-basic-ssl 2（`__Host-` cookie に必要な同一オリジン HTTPS の前提を満たす） |

> **検証メモ:** React Router 8 は宣言的（Declarative）モードと `<BrowserRouter>` を**維持しています**（根拠: RR の変更履歴に「Declarative Mode」が残り、公式の勧告は Declarative/Data の両モードを併記し、v8 の議論でも破壊的変更の予定はないとされています）。したがって、本設計の「RR8 + Declarative + BrowserRouter」は有効であり、`createBrowserRouter` へ変更する必要はありません。

## 依存バージョンの固定（npm の `latest`、2026-06-18 検証済み）

| 依存関係 | バージョン | ドキュメント |
|---|---|---|
| react / react-dom | 19.2.7 | https://react.dev |
| typescript | 6.0.3 | https://www.typescriptlang.org/docs/ |
| vite | 8.0.16 | https://vite.dev/guide/ |
| @vitejs/plugin-react | 6.0.2 | https://www.npmjs.com/package/@vitejs/plugin-react |
| @vitejs/plugin-basic-ssl | 2.3.0 | https://www.npmjs.com/package/@vitejs/plugin-basic-ssl |
| tailwindcss | 4.3.1 | https://tailwindcss.com/docs/v4 |
| @tailwindcss/vite | v4 最新版 | https://tailwindcss.com/docs/v4 |
| react-router | 8.0.0 | https://reactrouter.com/home |
| @tanstack/react-query | 5.101.0 | https://tanstack.com/query/latest/docs/framework/react/overview |
| zustand | 5.0.14 | https://github.com/pmndrs/zustand |
| react-hook-form | 7.79.0 | https://react-hook-form.com/get-started |
| @hookform/resolvers | 最新版 | https://react-hook-form.com/docs/useform/SchemaValidation |
| zod | 4.4.3 | https://zod.dev/ |
| react-i18next | 17.0.8 | https://react.i18next.com/ |
| i18next | 26.3.1 | https://www.i18next.com/ |
| shadcn (CLI) | 4.11.0 | https://ui.shadcn.com/docs |
| @tabler/icons-react | 3.44.0 | https://tabler.io/icons |
| vite-plugin-sri3 | 2.0.0 | https://www.npmjs.com/package/vite-plugin-sri3 |
| vitest | 4.1.9 | https://vitest.dev/guide/ |
| @testing-library/react | 16.3.2 | https://testing-library.com/docs/react-testing-library/intro/ |
| msw | 2.14.6 | https://mswjs.io/docs/ |

## TypeScript / ECMAScript ベースライン（検証済み）

- **TypeScript 6.0.3** を使用します。`tsconfig.json` に `"target": "ES2025"`、`"lib": ["ES2025", "DOM", "DOM.Iterable"]`、`"strict": true` を設定します。
- **検証結果（2026-06-18）:** ES2026 標準は実在します（第 17 版、2026 年 6 月確定）が、**TS 6.0 には `ES2026` という `target` / `lib` のリテラルがありません**。TS 6.0 で名前が付いた最高の `target` は `ES2025` です（公式説明では、ES2025 に新しい JavaScript 言語機能は含まれず、組み込み API 型のみが更新されます）。それより新しい指定には `esnext` が必要です。`ES2026` という `target` は TS 7.0（開発中の Go ネイティブ版）で追加される見込みです。
- **判断:** 選択肢を評価し、アップグレードを予測可能にするため、`esnext` ではなく安定した名前付き `target` **`ES2025`** を採用します。
- 実際の構文ダウンレベルは、モダンブラウザー／Browserslist に基づく **Vite `build.target`** が処理します。TypeScript は型チェックのみを行い（`noEmit`）、`target` は主に既定の `lib` と型の意味を決めるため、最終成果物には直接影響しません。

---

<- [索引](./README.md) · 次: [02 アーキテクチャと基盤](02-architecture.md)
