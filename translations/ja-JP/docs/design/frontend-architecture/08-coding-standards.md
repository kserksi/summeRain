# 08 · コーディング標準

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

> 本標準は、プロジェクトの本番要件（[07 本番標準](07-production-standards.md)）と React、TypeScript、Tailwind の公式ガイドを組み合わせ、実装時の統一規則とするものです。根拠となる資料: [React ドキュメント](https://react.dev/learn/thinking-in-react)と [Hook のルール](https://react.dev/reference/rules)、[TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html)、[typescript-eslint](https://typescript-eslint.io)、[Tailwind CSS](https://tailwindcss.com/docs)。

## 基本規則

- 3 つのエコシステムの公式ガイドに従います。[07 本番標準](07-production-standards.md)は本番の必須要件、本節は日常のコーディング詳細を定め、両方を同時に適用します。
- 自己説明的なコードを優先します。既定ではコメントを書きません。理由が自明でない場合に限り `// why` を記述し、`// what` は書きません。公開フックとユーティリティには TSDoc を使用します。

## 命名

- 変数、関数、通常のファイルは camelCase、コンポーネント、型、インターフェースは PascalCase とし、コンポーネントのファイル名はコンポーネント名と一致させます。
- 定数は `UPPER_SNAKE_CASE` とし、`config/constants.ts` に集約します。フックは `use` プレフィックス、i18n キーはドット区切りの名前空間を使用します。
- 真偽値は `is`、`has`、`can`、`should` のいずれかをプレフィックスにします。イベントハンドラーは `handleXxx` または `onXxx` を使用します。
- CSS カスタムプロパティは言語仕様に従い kebab-case とし、それ以外は上記の camelCase 規則に従います。

## TypeScript

- `"strict": true` を設定し、**`any` を禁止します**。どうしても避けられない場合は、理由をコメントで説明します。公開 API は型を明示し、ローカルコードでは推論を利用します。
- 拡張可能またはマージ可能なオブジェクト形状には `interface`、ユニオン型やユーティリティ型には `type` を使い、規則を混在させません。
- `const` または `let` を使用し、`var` は禁止します。`async/await` を優先します。`catch` 変数は `unknown` として扱い、エラーには型付きの `ApiError` を使用します。
- リテラルを定数モジュールへ集約し、マジック値を排除します。

## React（公式のコンポーネント／フックガイドに準拠）

- 関数コンポーネントとフックのみを使用し、[フックの規則](https://react.dev/reference/rules/rules-of-hooks)に従います。1 ファイルにつき主コンポーネントは 1 つです。
- props は `interface` または `type` で定義し、必須フィールドを優先します。リストは安定した `key` を使い、配列インデックスは使いません。
- **`useEffect` より派生状態を優先します。** エフェクトの依存関係を正確にし、各エフェクトの責務を 1 つに限定して、エフェクトの連鎖を避けます。
- 継承より合成、非純粋なコンポーネントより純粋なコンポーネントを優先します。副作用を分離し、ガードは早期 `return` を使います。
- フォームには React Hook Form + Zod を使用し、スキーマを検証の信頼できる唯一の情報源とします。

## 状態とデータ

- **サーバー状態はすべて TanStack Query で管理します。** API データを `useState` や zustand にキャッシュしません。
- zustand はテーマと現在のユーザースナップショットという、真のクライアント状態だけを保存します。Query キーは標準化して集約します。
- ミューテーションは `invalidateQueries` でデータを無効化します。ロールバック付きの楽観的更新を実装する場合を除き、キャッシュを手動で編集しません。

## スタイル（Tailwind v4 公式規則）

- ユーティリティクラスを優先します。**コンポーネント内の生の 16 進数カラー値を禁止し**、`bg-primary` などの shadcn トークンクラスを使用します。
- `@apply` を乱用せず、複雑なスタイルはコンポーネントへ抽出します。条件付きクラスには `cn()`（clsx + tailwind-merge）を使用します。
- レスポンシブレイアウトは `sm:`、`md:`、`lg:` を使った**モバイルファースト**とします。ダークテーマは `.dark` クラスに基づく `dark:` バリアントを使用します。

## ファイルとエクスポート

- 機能ベースで整理します（[02 アーキテクチャ](02-architecture.md)を参照）。**名前付きエクスポート**を優先し、ページ／ルートコンポーネントでは `default export` も許可します。
- 循環依存とバンドルの肥大化を避けるため、`index.ts` のバレルファイルは慎重に使用します。

## フォーマットとリント（品質ゲート）

- **Prettier:** インデント 2 スペース、セミコロン、ダブルクォート、末尾カンマ `all`、行幅 100、`LF`、BOM なし UTF-8。
- **ESLint（フラット設定）:** `typescript-eslint` の `recommended` + `react` の `recommended` + `react-hooks` + `jsx-a11y`。
- **段階別ゲート**（[06 テスト](06-testing.md)を参照）:
  - コミット前（高速）: `prettier --check` + `eslint` + `tsc --noEmit`
  - CI（マージ／リリースを阻止）: 上記 3 項目 + `vitest run`
- `console.*`、`debugger`、コメントアウトされたデッドコード、未使用の `import` を禁止します。

## セキュリティとアクセシビリティ

- **フロントエンドはセキュリティ境界ではありません。** 認証と認可の判断はすべてバックエンドが行います（`/admin/*` は `RequireAdmin` を通り、一般ユーザーには 403 を返します）。フロントエンドコードとチャンクは公開情報として扱い、コードやルートを隠すことでアクセス制御を実現してはなりません。機密データ操作の唯一の防御はバックエンドの強制チェックです。
- **ユーザー切り替え時にキャッシュを消去します。** ログイン／ログアウトでは `queryClient.clear()` を呼び出し、`window.location.assign` で**ハードリフレッシュ**して、特に TanStack Query キャッシュなど前セッションのメモリーデータが別ユーザーへ残らないようにします。`auth-store` は永続化しません（[02 セッション切り替え時のデータ消去](02-architecture.md)を参照）。
- 秘密情報を出力・コミットしません。ユーザー入力をサニタイズし、CSRF と認証情報の処理を `lib/api.ts` に集約します。
- サニタイズ済みの入力を除き、`dangerouslySetInnerHTML` を禁止します。
- shadcn のアクセシビリティ基準を維持します。セマンティック HTML、`label`、`aria`、キーボード操作、可視フォーカス、画像の `alt`、4.5:1 以上のテキストコントラスト（WCAG AA）を満たします。

## パフォーマンス

- ルートを遅延読み込みします（[02 アーキテクチャ](02-architecture.md)を参照）。画像には `loading="lazy"` と適切な寸法を設定します。
- Query の `staleTime` と `gcTime` を適切に設定して過剰なリクエストを避けます。`memo` は計測後にのみ使用します。

## コミット

- Conventional Commits（`feat:`、`fix:` など）を使用し、各コミットを小さく単一目的に保ちます。
- 全体の規則に従い、ユーザーが明示的に依頼した場合にのみコミットします。

---

<- [07 本番標準](07-production-standards.md) · [索引](./README.md) · 次: [09 判断とスコープ](09-decisions-and-scope.md)
