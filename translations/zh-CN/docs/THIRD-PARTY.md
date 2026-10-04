# 第三方软件

summeRain 构建于开源软件之上。我们感谢下列所有项目的维护者与贡献者。

本页列出项目直接采用的依赖及其作者或组织、许可证和链接。完整且带校验和锁定的依赖图记录在
[`backend/go.sum`](https://github.com/kserksi/summeRain/blob/main/backend/go.sum) 与
[`frontend/package-lock.json`](https://github.com/kserksi/summeRain/blob/main/frontend/package-lock.json) 中。
前端列表包含构建、开发与测试工具。

## 前端

| 组件 | 作者或组织 | 许可证 | 链接 |
|---|---|---|---|
| React、React DOM | Meta Platforms, Inc. | MIT | https://github.com/facebook/react |
| React Router | Shopify / Remix 贡献者 | MIT | https://github.com/remix-run/react-router |
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
| Vite | Evan You 与 Vite 团队 | MIT | https://github.com/vitejs/vite |
| @vitejs/plugin-react | Vite 团队 | MIT | https://github.com/vitejs/vite-plugin-react |
| @vitejs/plugin-basic-ssl | Vite 团队 | MIT | https://github.com/vitejs/vite-plugin-basic-ssl |
| vite-plugin-sri3 | yoyo930021 | MIT | https://github.com/yoyo930021/vite-plugin-sri3 |
| Vitest | Vitest 团队 | MIT | https://github.com/vitest-dev/vitest |
| Testing Library | Testing Library 贡献者 | MIT | https://testing-library.com |
| MSW | Mock Service Worker | MIT | https://github.com/mswjs/msw |
| jsdom | jsdom 贡献者 | MIT | https://github.com/jsdom/jsdom |
| fake-indexeddb | Jeremy Scheff | Apache-2.0 | https://github.com/dumbmatter/fakeIndexedDB |
| ESLint | ESLint 团队（OpenJS Foundation） | MIT | https://github.com/eslint/eslint |
| @eslint/js | ESLint 团队（OpenJS Foundation） | MIT | https://github.com/eslint/eslint |
| eslint-plugin-jsx-a11y | jsx-eslint 贡献者 | MIT | https://github.com/jsx-eslint/eslint-plugin-jsx-a11y |
| eslint-plugin-react-hooks | Meta Platforms, Inc. | MIT | https://github.com/facebook/react |
| eslint-plugin-react-refresh | Arnaud Barré | MIT | https://github.com/ArnaudBarre/eslint-plugin-react-refresh |
| typescript-eslint | typescript-eslint 团队 | MIT | https://github.com/typescript-eslint/typescript-eslint |
| TypeScript | Microsoft Corporation | Apache-2.0 | https://github.com/microsoft/TypeScript |
| Prettier | Prettier 贡献者 | MIT | https://github.com/prettier/prettier |
| globals | Sindre Sorhus | MIT | https://github.com/sindresorhus/globals |
| @types/* | DefinitelyTyped 贡献者 | MIT | https://github.com/DefinitelyTyped/DefinitelyTyped |

## 后端

| 组件 | 作者或组织 | 许可证 | 链接 |
|---|---|---|---|
| Gin | Gin 贡献者 | MIT | https://github.com/gin-gonic/gin |
| GORM、GORM MySQL 驱动 | GORM | MIT | https://github.com/go-gorm/gorm |
| go-redis（v8） | go-redis 作者 | BSD-2-Clause | https://github.com/go-redis/redis |
| go-sql-driver/mysql | go-sql-driver | MPL-2.0 | https://github.com/go-sql-driver/mysql |
| AWS SDK for Go v2 | Amazon Web Services | Apache-2.0 | https://github.com/aws/aws-sdk-go-v2 |
| Smithy Go | Amazon Web Services | Apache-2.0 | https://github.com/aws/smithy-go |
| Prometheus Go 客户端 | Prometheus 作者 | Apache-2.0 | https://github.com/prometheus/client_golang |
| golang.org/x/crypto | The Go Authors | BSD-3-Clause | https://pkg.go.dev/golang.org/x/crypto |
| golang.org/x/sys | The Go Authors | BSD-3-Clause | https://pkg.go.dev/golang.org/x/sys |

## 服务与运行时组件

这些组件由官方部署方案以独立容器镜像或内置运行时库的形式引用。它们以未修改的方式使用，
不会链接进 summeRain 应用二进制。

| 组件 | 作者或组织 | 许可证 | 链接 |
|---|---|---|---|
| MySQL 8.4（Community） | Oracle Corporation | GPL-2.0 | https://github.com/mysql/mysql-server |
| Redis 8 | Redis Ltd. | RSALv2 / SSPLv1 / AGPLv3（三选一） | https://github.com/redis/redis |
| imgproxy 4 | Evil Martians | Apache-2.0 | https://github.com/imgproxy/imgproxy |
| libvips | John Cupitt 与 libvips 贡献者 | LGPL-2.1-or-later | https://github.com/libvips/libvips |
| Alpine Linux（基础镜像） | Alpine Linux 开发团队 | 多种开源许可证 | https://alpinelinux.org |

## 工具链

| 组件 | 作者或组织 | 许可证 | 链接 |
|---|---|---|---|
| Go | The Go Authors | BSD-3-Clause | https://go.dev |
| Node.js | OpenJS Foundation | MIT | https://nodejs.org |

## 说明

- 上述直接依赖使用 MIT、ISC、BSD、Apache-2.0 或 MPL-2.0 许可。应用二进制中不链接任何
  GPL、AGPL 或 SSPL 代码。
- `wasm-vips` 是 MIT 许可的 WebAssembly 封装，捆绑了供浏览器使用的 `libvips`
  （LGPL-2.1-or-later）。imgproxy 也在其容器内以共享库方式使用 `libvips`。两种情况下
  `libvips` 均未经修改。
- MySQL 与 Redis 作为独立、未修改的容器服务运行；项目不链接或再分发其代码。Redis 8 由
  Redis Ltd. 以 RSALv2、SSPLv1 或 AGPLv3 三选一方式提供。
- 新增或移除依赖时，请在同一次变更中更新本页与
  [`NOTICE`](https://github.com/kserksi/summeRain/blob/main/NOTICE) 文件。
