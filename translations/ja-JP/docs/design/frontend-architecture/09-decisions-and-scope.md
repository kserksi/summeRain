# 09 · 判断とスコープ

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

## 移行と整理

- リポジトリルートの `index.html`、`css/`、`js/` にある既存のネイティブ JavaScript フロントエンドを `frontend/` で置き換えます。
- 新しいフロントエンドの準備と検証が完了した後、古いネイティブフロントエンドのファイルを削除します。
- 移行期間中は両方を共存させ、相互に影響しないようにします。旧フロントエンドはリポジトリルートからアクセスし、新フロントエンドは開発中に Vite を使用して、ビルド後は `backend/web/` へ出力します。

<a id="explicit-exclusions-yagni"></a>
## 明示的な対象外（YAGNI）

- 公開ギャラリー、発見ページ、コミュニティ選出（バックエンドに公開一覧エンドポイントがない）
- 画像カテゴリー／タグ（バックエンドの Image モデルに該当フィールドがない）
- 管理者向け画像審査一覧（バックエンドに該当エンドポイントがない）
- ユーザー削除（バックエンドはステータス変更のみ対応）
- サーバーサイドレンダリング／Node ミドルウェア層
- フィールド変換レイヤー（バックエンドの `snake_case` フィールドを直接使用）

<a id="decision-record"></a>
## 意思決定記録

| 判断項目 | 選択 | 理由 |
|---|---|---|
| 技術スタック | React + Vite + TypeScript + shadcn/ui + Tailwind | エコシステムが広く、コンポーネントライブラリが成熟し、静的 SPA に適するため |
| 機能スコープ | バックエンドと正確に一致 | バックエンドが対応しない機能を作らないため |
| プロジェクト構成 | リポジトリルートの `frontend/` -> `backend/web/` へビルド | フロントエンドとバックエンドのソースを分離しつつ、バックエンドの静的配信契約を満たすため |
| コード構成 | 機能ベース + TanStack Query | 5 領域の境界を明確かつ高凝集にし、サーバー状態を集約するため |
| shadcn プリセット | `apply --preset b3RZAU6YV`、色と書体は不採用 | プリセットのコンポーネント構造を再利用しつつ、コーヒーパレットを維持するため |
| ルーティング | BrowserRouter + 遅延読み込み | バックエンドの SPA フォールバックで対応でき、深いリンクも利用できるため |
| i18n | 既定は `en-US`、`zh-CN` と `ja-JP` を同梱し、すべての文言を i18n リソースへ格納 | 英語を主要言語としながら、完全なローカライズ機能を維持するため |
| 本番成果物 | SHA512 SRI + SemVer + リソースのローカル化 + マジック値なし | ユーザーが定めた本番品質の必須要件。開発段階は免除 |
| TypeScript/ECMAScript ベースライン | `target` は `ES2025`（TS 6.0 で名前が付いた最高の `target`） | TS 6.0 は ES2026 という `target` リテラルを扱えないため、安定した名前付き `target` を選び、実際のダウンレベルを Vite に任せる |
| コーディング標準 | [08 コーディング標準](08-coding-standards.md): 公式ガイド + ユーザー要件、Prettier/ESLint/tsc ゲートで強制 | 測定・検証可能な単一の実装規則を形成するため |
| セッション切り替えの安全性 | ログアウト／ログイン時に `queryClient.clear()` とハードリフレッシュを実行し、`auth-store` は永続化せず、バックエンドの `RequireAdmin` 403 を維持 | メモリーデータがユーザー境界を越えるのを防ぎ、フロントエンドがセキュリティ境界ではないことを再確認するため（[02](02-architecture.md)、[08](08-coding-standards.md)を参照） |
| 非公開画像トークンモデル | 画像ごとに共有トークン 1 つ。TTL は既定 1h、10min–72h で調整可能、ミリ秒で検証。トークン文字列は不変。失効後は自動再発行せず `owner` / `admin` が手動で再申請。`owner` / `admin` セッションはトークンを迂回して直接アクセス。欠落／誤り／期限切れ -> 403、失効済み -> 404 | ユーザー定義の規則で、バックエンドに実装済み（API.md §4.3/§5、エラーコード 4037/4042） |
| CAPTCHA | 管理者が選択する 3 つの差し替え可能なプロバイダー（reCAPTCHA/Turnstile/Geetest v4）、既定は `none` | 中国の利用者には Turnstile/Geetest を優先。CAPTCHA は §07 で唯一管理された外部リソース例外であり、既定の `none` なら規則を維持できるため |
| ビジュアルとページ UI/UX | Warm Soft Studio（maia カードスタイル + 大きな角丸 + コーヒートークン）、Tabler Icons。ページ別仕様は[デザインシステム MASTER](design-system/MASTER.md)と [10-pages-ui-ux.md](10-pages-ui-ux.md)を参照 | プロトタイプで検証済み。shadcn/ui を使用し、ネイティブコントロールを禁止 |
| アイコンライブラリ | **Tabler Icons**（`@tabler/icons-react`）でプリセットの Phosphor を上書き | 単一のアウトラインスタイルで外観を統一。`components.json` は `iconLibrary=tabler` |
| テーマ切り替え | View Transitions を主経路（実コンテンツの円形リビール）、`.theme-mask` WAAPI をフォールバックとし、動きを抑える設定では即時切り替え | ヘッダーに明確な入口を設け、最良の効果、幅広いブラウザー対応、アクセシビリティを両立するため |

## 参考資料

- バックエンド API 契約: `docs/API.md`
- バックエンドのエントリーポイント（SPA フォールバック／静的配信）: `backend/cmd/server/main.go`
- バックエンドの認証と CSRF: `backend/internal/middleware/{auth,csrf}.go`
- 既存のコーヒーテーマ（移行対象の色の参照元）: `css/style.css`
- 公式コーディングガイド: [React](https://react.dev/learn/thinking-in-react) · [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html) · [typescript-eslint](https://typescript-eslint.io) · [Tailwind CSS](https://tailwindcss.com/docs) · [Prettier](https://prettier.io) · [WCAG](https://www.w3.org/WAI/standards-guidelines/wcag/)

---

<- [08 コーディング標準](08-coding-standards.md) · [索引](./README.md)
