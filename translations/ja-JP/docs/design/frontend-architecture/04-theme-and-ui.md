# 04 · テーマと UI

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

## shadcn プリセット戦略

- 初期化後に `npx shadcn@latest apply --preset b3RZAU6YV` を実行してプリセットを取得します。
- **プリセットはコンポーネント構成と既定構造の決定にのみ使用します**（コンポーネントセット、角丸、バリアントの骨格など）。
- **プリセットの色、フォント、アイコンは採用しません:**
  - 色 -> **独自のコーヒーパレット**で上書きします（`src/styles.css` の `@theme` および `:root`/`.dark` CSS 変数に定義。Tailwind v4 は CSS ファーストで、`tailwind.config.js` は使用しません）。
  - フォント -> プリセットの Geist ではなく、システムフォントスタックを維持します。
  - アイコン -> プリセットの Phosphor ではなく **Tabler Icons** を使用し、`components.json` の `iconLibrary` を `tabler` に設定します。

## コーヒーパレット（Tailwind v4 `@theme` の接続）

> **[design-system/MASTER.md](design-system/MASTER.md) §2 を信頼できる唯一の情報源とします**（完全なトークン表とコントラスト比を含みます）。以下はその要約です。プロトタイプの反復で**コントラスト向上のため色を濃く調整**し（カード上の補助テキスト 6.24:1、本文 11.98:1）、以前の `css/style.css` を**置き換えます**（従来のネイティブフロントエンドは `frontend/` に置き換わるため、以後は色の参照元にしません）。

Tailwind v4 は CSS ファーストです。`src/styles.css` の `@theme { --color-*: ...; }` と `:root`/`.dark` 変数にパレットを定義し、コンポーネントは `bg-primary` や `text-foreground` などのトークンクラスを直接使用します。`tailwind.config.js` はありません。

ライトテーマ（モカ）、`:root`:
- 背景 `#F1E7DA` / カード `#FFFCF8` / テキスト `#33261B` / 補助テキスト `#6E5C49` / 境界線 `#DAC7AE`
- プライマリ `#6F4E37`（ホバー `#573D2B`、淡い背景 `#EFE2D2`）/ セカンダリ `#A9764F`

ダークテーマ（エスプレッソ）、`.dark`:
- 背景 `#16100D` / カード `#251B14` / テキスト `#F2E7D6` / 補助テキスト `#C5B59E` / 境界線 `#4D3A29`
- プライマリ `#D4A57E`（ホバー `#B9895F`）/ セカンダリ `#C39A72`

彩度を抑えたステータス色は MASTER §2 を参照してください。

ステータス色はすべて柔らかく調整し、緑、青、赤、紫をそれぞれ成功、警告、危険、情報に割り当てます。

## 書体と i18n

- **書体:** システムフォントスタック `-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Roboto, ...` を使用します。外部 Web フォントは一切読み込みません（[07 本番標準 · 外部リソースのローカル化](07-production-standards.md#local-external-resources)を満たします）。
- **i18n:** 英語を主要言語とし、既定ロケールを `en-US` とします。`zh-CN` と `ja-JP` も提供します。すべての UI 文字列を言語別に `src/i18n/locales/` へ格納し、コンポーネントは必ず `t('key')` で取得します。ユーザー向け文言のハードコードは禁止します。

> この要件は、i18n の導入を見送るとしていた以前の YAGNI 判断を置き換えます（[09 意思決定記録](09-decisions-and-scope.md#decision-record)を参照）。

---

<- [03 機能](03-features.md) · [索引](./README.md) · 次: [05 ビルドとデプロイ](05-build-and-deploy.md)
