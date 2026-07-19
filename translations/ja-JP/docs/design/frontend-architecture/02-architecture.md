# 02 · アーキテクチャと基盤

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

## ディレクトリ構成

React のソースはリポジトリルートに新設する `frontend/` に置き、ビルド成果物は `backend/web/` へ出力します。

```
frontend/
├─ index.html
├─ vite.config.ts          # outDir: ../backend/web; base: '/'; 開発プロキシ /api+/i -> :8080; HTTPS 開発環境
├─ src/styles.css          # Tailwind v4 エントリー: @import "tailwindcss"; @theme {...コーヒーパレット}; :root/.dark 変数
├─ tsconfig.json           # strict; target/lib は 01-overview.md の「TypeScript / ES ベースライン」を参照
├─ components.json         # shadcn 設定（Tailwind v4 モード）
├─ package.json            # バージョンは SRI マニフェストが参照するリリース SemVer
└─ src/
   ├─ main.tsx             # QueryClientProvider + Router + ThemeProvider + Toaster をマウント
   ├─ App.tsx              # ルートツリー + AuthGuard/AdminGuard + Layout
   ├─ config/
   │  └─ constants.ts      # すべての定数を集約（07-production-standards.md 参照。マジック値を排除）
   ├─ i18n/
   │  ├─ index.ts          # i18next 初期化（既定 en-US）
   │  └─ locales/
   │     ├─ en-US.json     # 既定の英語文言
   │     ├─ zh-CN.json     # 簡体字中国語の文言
   │     └─ ja-JP.json     # 日本語の文言
   ├─ lib/
   │  ├─ api.ts            # 中核となる fetch ラッパー
   │  ├─ csrf.ts           # __Host-csrf_token クッキーを読み取る
   │  ├─ query-client.ts   # QueryClient 設定
   │  └─ utils.ts          # cn() / formatSize / timeAgo / formatNumber
   ├─ store/
   │  ├─ theme-store.ts    # zustand で永続化（light/dark）
   │  └─ auth-store.ts     # 現在のユーザー（hydrate / clear）
   ├─ components/
   │  ├─ ui/               # shadcn が生成（button/dialog/table/form/toast/...）
   │  └─ layout/           # Navbar / Footer / ThemeToggle / NotificationBell
   ├─ routes/
   │  └─ lazy.tsx          # React.lazy のルート単位エントリーポイント
   └─ features/
      ├─ auth/             # api.ts hooks.ts pages(Login, Register) components/
      ├─ captcha/          # api.ts(usePublicConfig) hooks.ts(useCaptcha) components(Captcha を provider で分岐)
      ├─ images/           # api.ts hooks.ts pages(List, Detail, Upload) components/
      ├─ user/             # api.ts hooks.ts pages(Profile) components/
      ├─ notifications/    # api.ts hooks.ts components(Dropdown)
      └─ admin/            # api.ts hooks.ts pages(Users, Stats, Configs) components/
```

各 `features/<ドメイン>/` は自己完結させます。`api.ts` にその領域のエンドポイント、`hooks.ts` に Query/Mutation Hook、さらに `pages/` と `components/` を置きます。領域をまたぐ共有コードは、トップレベルの `components/` と `lib/` に配置します。

## 中核基盤

### `lib/api.ts`: サイト全体で唯一のリクエスト窓口

- `baseUrl = '/api/v1'` を設定し、すべてのリクエストで `credentials: 'include'` を使用します。
- GET 以外のリクエストに `X-CSRF-Token` ヘッダーを自動で付与し、値は `lib/csrf.ts` を通して `__Host-csrf_token` クッキーから読み取ります。
- 共通エンベロープ `{ code, message, data }` を展開します。`code === 0` なら `data` を返し、それ以外は `throw new ApiError(code, message)` とします。
- **エラーコード処理を集約し**、HTTP ステータスだけでなく `code` で分岐します。
  - **401 / 4010 / 4011**（未認証／セッション期限切れ）: **既定では** `auth-store` を消去して `/login` へ移動します。`/auth/me` の確認で使うリクエスト単位の `{ skipAuthRedirect: true }` 例外を用意し、その場合の 401 は匿名を意味するだけでリダイレクトしません。匿名ユーザーを公開ページから誤って追い出すことを防ぎます。
  - **4030**（アカウント無効）: ログアウトし、`/login` へ移動して「このアカウントは無効化されています」と表示します。無効化されたユーザーが再度ログインを試みた場合も同じコードになるため、ログインフォームも同じ文言へマッピングします。
  - **429 / 2008 / 4029 / 2090**（レート制限）: 負荷を増幅しないよう、自動再試行しません。`ApiError` を投げ、UI で「操作が多すぎます。しばらくしてから再試行してください」と表示します。応答に `Retry-After` があれば、対応するカウントダウンを表示します。
  - **4032 / 4033**（管理エンドポイントは Web 限定／識別情報の誤用）: 呼び出し方法の誤りです。ユーザーに通知して報告しますが、ログアウトはしません。
- **エラー文言のローカライズ:** 各 `code` を現在の言語リソースにある `errors.<code>` キーへマッピングします。バックエンドの `message` は未知の `code` に対するフォールバックとしてのみ使用し、コンポーネントで直接表示しません。
- `api.get`、`api.post`、`api.patch`、`api.del` と、`api.upload(formData)` を提供します。
- **フィールド変換レイヤーは設けません。** コンポーネントは `created_at`、`view_count`、`storage_used` などバックエンドの `snake_case` フィールドを直接使用し、データ形状を 1 つに保ちます。

### 認証フロー

- アプリケーション起動時に `GET /auth/me` を呼び出します。成功したら `auth-store` に書き込み、**401 ならログアウト状態を維持**します。このリクエストは `skipAuthRedirect: true` を使うため、401 は匿名訪問者の判定だけを行い、グローバルな遷移を発生させません。利用中のセッション失効による 401 とは区別します。
- 同時に、認証不要の `GET /public/config` を確認して `captcha_provider` とクライアントキーを取得し、ログイン／登録で CAPTCHA を読み込むか判断します（[03](03-features.md#pluggable-captcha-administrator-selected-default-none)を参照）。`provider=none` なら外部スクリプトを読み込みません。
- **`/i/` の画像描画:** `owner` / `admin` が自分の／任意の非公開画像を見る場合、同一オリジンのセッションで自動的に認可されるため、`<img src="/i/<link>">` にトークンは不要です。第三者は共有 URL `/i/<link>?token=<トークン>` でアクセスします。非公開画像の応答は `no-store` とし、フロントエンドではキャッシュしません。
- 認証が必要なルートを `AuthGuard` で囲み、`AdminGuard` ではさらに `auth.user.role === 'admin'` を確認します。
- **AdminGuard の降格時復旧:** 管理者が降格された場合、クライアントの `role` スナップショットが古くなる可能性があります。管理領域のリクエストで **4030/4032** を受けたら `refreshUser()` で `/auth/me` を再取得してスナップショットを更新し、`/admin/*` の外へ移動します。権限がなくなっていれば `/dashboard` に戻します。
- ログイン成功後、ブラウザーがクッキーを自動保存します。関連 Query を無効化し、**`/dashboard`** へ移動します。
- ログイン済みユーザーが公開の遷移先 `/` を訪れた場合、ルート内の `<Navigate>` で `/dashboard` へ自動リダイレクトし、追加リクエストは行いません。
- **確認以外**のリクエストが 401 を返した場合は、セッション期限切れまたは終了を意味します。`lib/api.ts` が一元的にログアウトして `/login` へ移動します。
- **ログアウト:** CSRF を付けて `POST /auth/logout` を呼び出します。成功後は以下のとおりキャッシュデータを消去し、`/` へハードリフレッシュします。

### セッション切り替え時のデータ消去（セキュリティ）

**ユーザーをまたぐデータ残留**を防ぎます。たとえば管理者がログアウトし、同じタブで一般ユーザーがログインしたとき、前セッションの管理データをメモリーから読めてはなりません。キャッシュされた JavaScript チャンク自体は無害です。フロントエンドはセキュリティ境界ではなく、バックエンドの `RequireAdmin` が 403 で未認可のデータ取得を防ぐからです。ここで防ぐ対象は**メモリー内の Query 結果**です。

- **ログアウト:** `queryClient.clear()` で Query キャッシュ全体を消去し、`auth-store` を消去して、`window.location.assign('/')` で**ハードリフレッシュ**します。リフレッシュにより QueryClient、React の状態、`auth-store` などすべてのメモリーが破棄され、次のログインは新しいページセッションになります。
- **ログイン成功:** 防御的に `queryClient.clear()` をもう一度呼び出し、`/dashboard` へ移動します。
- **`auth-store` を永続化しません**（下記「クライアント状態」を参照）。localStorage にユーザー本人情報を残さず、初回ページでは必ず `GET /auth/me` から `hydrate` し、サーバーを信頼できる唯一の情報源とします。
- **バックエンドのフォールバック:** `/admin/*` では `RequireAdmin` が `role` を確認し、一般ユーザーへ 403 を返します。フロントエンドのキャッシュ消去が残留を防ぎ、バックエンドの 403 が未認可取得を防ぎます。両方を適用します。
- ハードリフレッシュでバンドルを再ダウンロードすることはありません。全ユーザーが同じチャンクを使い、HTTP キャッシュにヒットするため、コストはほぼゼロです。

### TanStack Query の規則

- キーの規則:
  - `['images', { visibility, search }]`（一覧）
  - `['images', id]`（詳細。応答の `access_token` に非公開画像の現在のトークンが含まれ、独立したトークン用エンドポイント／キーはありません。[API.md §5.3](../../API.md)を参照）
  - `['admin', 'users', { page }]`
  - `['admin', 'stats']` と `['admin', 'configs']`
  - `['notifications']`、`['profile']`、`['public-config']`（CAPTCHA プロバイダー）
- 画像一覧には `useInfiniteQuery` を使い、`getNextPageParam: last => last.has_more ? last.next_cursor : undefined` を設定します。**`has_more` を終了条件**とし、`next_cursor` はカーソルとしてのみ扱います。両者が矛盾する場合は `has_more` を信頼します。
- `useMutation` が成功したら、該当キーに対して `queryClient.invalidateQueries` を呼び出します。たとえばアップロード、削除、公開範囲の変更では `['images']` を無効化します。
- 既定値を `refetchOnWindowFocus: true` とし、通知件数や統計を自動更新します。

### クライアント状態（zustand）

- `theme-store`: `light` または `dark` を、`constants.ts` に集約された **localStorage キー `ic_theme`** に永続化します。値が変わったら `<html>` の `.dark` クラスを追加または削除します。
- **テーマ切り替えアニメーション**（[MASTER §9](design-system/MASTER.md)を参照）: 利用可能なら `document.startViewTransition` を優先し、`::view-transition-new(root)` の円形 `clip-path` リビールを使います。利用できなければ `.theme-mask` と WAAPI の `transform:scale` アニメーションへフォールバックします。`prefers-reduced-motion` ではアニメーションなしで切り替えます。起動前には事前描画用のインラインスクリプトで保存済みテーマを適用し、ちらつきを防ぎます。
- `auth-store`: 初回 `hydrate` 用に現在のユーザーのスナップショットだけを保存します。**永続化せず**メモリー内だけに置き、初回ページで必ず `GET /auth/me` から再度 `hydrate` します。他の業務データはキャッシュしません。
- **スナップショット更新:** `/auth/me` を再呼び出してストアに書き戻す `refreshUser()` を提供します。ログイン成功後、`storage_used` と `image_count` が変わる画像のアップロード／削除／公開範囲変更後、および AdminGuard の降格時復旧で実行します。本スコープにはアバター／プロフィール編集エンドポイントがないため、他の更新元はありません。ストアと二重のデータソースになるため `/auth/me` を Query キャッシュに入れず、必要時に明示的に呼び出します。

## ルーティングとコード分割

- `BrowserRouter` を使用します。バックエンドの `NoRoute` は `index.html` にフォールバックするため、深いリンクの再読み込みも機能します。`base: '/'` を設定します。[05 ビルドとデプロイ](05-build-and-deploy.md)を参照してください。
- ルート表:
  - 公開: `/`（ランディングページ）、`/login`、`/register`
  - `AuthGuard` で保護: `/dashboard`、`/images`、`/images/:id`、`/upload`、`/profile`
  - `AuthGuard` + `AdminGuard` で保護: `/admin`、`/admin/users`、`/admin/configs`
  - ログイン済みで `/` へアクセス -> `<Navigate to="/dashboard">`。ログイン成功 -> `/dashboard`
- `/dashboard` はコンソールの遷移先であり、`auth.user.role` に応じて条件付き描画します。共通領域には個人統計、割り当て、最近の画像を表示し、`admin` に限りシステム概要カードと「管理画面を開く」入口を表示します。管理領域はさらに `React.lazy` で遅延読み込みし、一般ユーザーはそのコードをダウンロードしません。
- `React.lazy` で機能／ルート単位にコード分割し、`<Suspense>` と共通 `<Spinner>` をフォールバックにします。動的チャンクの完全性は [07 SRI 実行時ガード](07-production-standards.md)で保護します。

---

<- [01 概要](01-overview.md) · [索引](./README.md) · 次: [03 機能とページ](03-features.md)
