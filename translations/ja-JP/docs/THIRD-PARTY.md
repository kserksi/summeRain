# サードパーティソフトウェア

summeRain はオープンソースソフトウェアの上に構築されています。以下に挙げるすべての
プロジェクトのメンテナーとコントリビューターに感謝します。

本ページでは、プロジェクトが直接採用している依存関係と、その作者または組織、ライセンス、
リンクを一覧にします。完全なチェックサム固定済みの依存関係グラフは
[`backend/go.sum`](https://github.com/kserksi/summeRain/blob/main/backend/go.sum) と
[`frontend/package-lock.json`](https://github.com/kserksi/summeRain/blob/main/frontend/package-lock.json)
に記録されています。フロントエンドの一覧にはビルド、開発、テスト用のツールも含まれます。

## フロントエンド

| コンポーネント | 作者または組織 | ライセンス | リンク |
|---|---|---|---|
| React、React DOM | Meta Platforms, Inc. | MIT | https://github.com/facebook/react |
| React Router | Shopify / Remix コントリビューター | MIT | https://github.com/remix-run/react-router |
| TanStack Query | TanStack | MIT | https://github.com/TanStack/query |
| Zustand | Poimandres | MIT | https://github.com/pmndrs/zustand |
| Radix UI | WorkOS | MIT | https://github.com/radix-ui/primitives |
| shadcn/ui | shadcn | MIT | https://github.com/shadcn-ui/ui |
| class-variance-authority | Joe Bell | Apache-2.0 | https://github.com/joe-bell/cva |
| clsx | Luke Edwards | MIT | https://github.com/lukeed/clsx |
| tailwind-merge | dcastil | MIT | https://github.com/dcastil/tailwind-merge |
| tailwindcss | Tailwind Labs | MIT | https://github.com/tailwindlabs/tailwindcss |
| tw-animate-css | Wombosvideo | MIT | https://github.com/Wombosvideo/tw-animate-css |
| next-themes | Paco Coursey | MIT | https://github.com/pacocoursey/next-themes |
| React Hook Form | React Hook Form | MIT | https://github.com/react-hook-form/react-hook-form |
| @hookform/resolvers | React Hook Form | MIT | https://github.com/react-hook-form/resolvers |
| Zod | Colin McDonnell | MIT | https://github.com/colinhacks/zod |
| i18next | i18next | MIT | https://github.com/i18next/i18next |
| react-i18next | i18next | MIT | https://github.com/i18next/react-i18next |
| @tabler/icons-react | Tabler（Paweł Kuna） | MIT | https://github.com/tabler/tabler-icons |
| sonner | Emil Kowalski | MIT | https://github.com/emilkowalski/sonner |
| react-easy-crop | Valentin Hervieu | MIT | https://github.com/ValentinH/react-easy-crop |
| idb | Jake Archibald | ISC | https://github.com/jakearchibald/idb |
| pica | nodeca（Vitaly Puzrin） | MIT | https://github.com/nodeca/pica |
| wasm-vips | Kleis Auke Wolthuizen | MIT | https://github.com/kleisauke/wasm-vips |
| Vite | Evan You と Vite チーム | MIT | https://github.com/vitejs/vite |
| @vitejs/plugin-react | Vite チーム | MIT | https://github.com/vitejs/vite-plugin-react |
| @vitejs/plugin-basic-ssl | Vite チーム | MIT | https://github.com/vitejs/vite-plugin-basic-ssl |
| vite-plugin-sri3 | yoyo930021 | MIT | https://github.com/yoyo930021/vite-plugin-sri3 |
| Vitest | Vitest チーム | MIT | https://github.com/vitest-dev/vitest |
| Testing Library | Testing Library コントリビューター | MIT | https://testing-library.com |
| MSW | Mock Service Worker | MIT | https://github.com/mswjs/msw |
| jsdom | jsdom コントリビューター | MIT | https://github.com/jsdom/jsdom |
| fake-indexeddb | Jeremy Scheff | Apache-2.0 | https://github.com/dumbmatter/fakeIndexedDB |
| ESLint | ESLint チーム（OpenJS Foundation） | MIT | https://github.com/eslint/eslint |
| @eslint/js | ESLint チーム（OpenJS Foundation） | MIT | https://github.com/eslint/eslint |
| eslint-plugin-jsx-a11y | jsx-eslint コントリビューター | MIT | https://github.com/jsx-eslint/eslint-plugin-jsx-a11y |
| eslint-plugin-react-hooks | Meta Platforms, Inc. | MIT | https://github.com/facebook/react |
| eslint-plugin-react-refresh | Arnaud Barré | MIT | https://github.com/ArnaudBarre/eslint-plugin-react-refresh |
| typescript-eslint | typescript-eslint チーム | MIT | https://github.com/typescript-eslint/typescript-eslint |
| TypeScript | Microsoft Corporation | Apache-2.0 | https://github.com/microsoft/TypeScript |
| Prettier | Prettier コントリビューター | MIT | https://github.com/prettier/prettier |
| globals | Sindre Sorhus | MIT | https://github.com/sindresorhus/globals |
| @types/* | DefinitelyTyped コントリビューター | MIT | https://github.com/DefinitelyTyped/DefinitelyTyped |

## バックエンド

| コンポーネント | 作者または組織 | ライセンス | リンク |
|---|---|---|---|
| Gin | Gin コントリビューター | MIT | https://github.com/gin-gonic/gin |
| GORM、GORM MySQL ドライバー | GORM | MIT | https://github.com/go-gorm/gorm |
| go-redis（v8） | go-redis 作者 | BSD-2-Clause | https://github.com/go-redis/redis |
| go-sql-driver/mysql | go-sql-driver | MPL-2.0 | https://github.com/go-sql-driver/mysql |
| AWS SDK for Go v2 | Amazon Web Services | Apache-2.0 | https://github.com/aws/aws-sdk-go-v2 |
| Smithy Go | Amazon Web Services | Apache-2.0 | https://github.com/aws/smithy-go |
| Prometheus Go クライアント | Prometheus 作者 | Apache-2.0 | https://github.com/prometheus/client_golang |
| golang.org/x/crypto | The Go Authors | BSD-3-Clause | https://pkg.go.dev/golang.org/x/crypto |
| golang.org/x/sys | The Go Authors | BSD-3-Clause | https://pkg.go.dev/golang.org/x/sys |

## サービスとランタイムコンポーネント

これらのコンポーネントは、公式デプロイが独立したコンテナーイメージまたは同梱のランタイム
ライブラリとして参照します。未改変のまま使用され、summeRain アプリケーションのバイナリには
リンクされません。

| コンポーネント | 作者または組織 | ライセンス | リンク |
|---|---|---|---|
| MySQL 8.4（Community） | Oracle Corporation | GPL-2.0 | https://github.com/mysql/mysql-server |
| Redis 8 | Redis Ltd. | RSALv2 / SSPLv1 / AGPLv3（三者択一） | https://github.com/redis/redis |
| imgproxy 4 | Evil Martians | Apache-2.0 | https://github.com/imgproxy/imgproxy |
| libvips | John Cupitt と libvips コントリビューター | LGPL-2.1-or-later | https://github.com/libvips/libvips |
| Alpine Linux（ベースイメージ） | Alpine Linux 開発チーム | 各種オープンソースライセンス | https://alpinelinux.org |

## ツールチェーン

| コンポーネント | 作者または組織 | ライセンス | リンク |
|---|---|---|---|
| Go | The Go Authors | BSD-3-Clause | https://go.dev |
| Node.js | OpenJS Foundation | MIT | https://nodejs.org |

## 注記

- 上記の直接依存関係は MIT、ISC、BSD、Apache-2.0、MPL-2.0 のいずれかでライセンスされて
  います。アプリケーションのバイナリには GPL、AGPL、SSPL のコードはリンクされていません。
- `wasm-vips` は MIT ライセンスの WebAssembly ラッパーで、ブラウザー用の `libvips`
  （LGPL-2.1-or-later）を同梱しています。imgproxy も自身のコンテナー内で `libvips` を
  共有ライブラリとして使用します。どちらの場合も `libvips` は未改変で利用されます。
- MySQL と Redis は独立した未改変のコンテナーサービスとして動作し、プロジェクトはその
  コードをリンクも再配布もしません。Redis 8 は Redis Ltd. により RSALv2、SSPLv1、AGPLv3
  の三者択一で提供されます。
- 依存関係を追加または削除した場合は、同じ変更の中で本ページと
  [`NOTICE`](https://github.com/kserksi/summeRain/blob/main/NOTICE) を更新してください。
