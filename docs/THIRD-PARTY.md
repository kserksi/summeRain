# Third-Party Software

summeRain is built on open-source software. We thank the maintainers and
contributors of every project listed here.

This page lists the dependencies the project directly adopts, with their
authors or organizations, licenses, and links. The complete, checksum-locked
dependency graphs are recorded in [`backend/go.sum`](../backend/go.sum) and
[`frontend/package-lock.json`](../frontend/package-lock.json). The frontend
list includes build, development, and testing tools.

## Frontend

| Component | Author or organization | License | Link |
|---|---|---|---|
| React, React DOM | Meta Platforms, Inc. | MIT | https://github.com/facebook/react |
| React Router | Shopify / Remix contributors | MIT | https://github.com/remix-run/react-router |
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
| @tabler/icons-react | Tabler (Paweł Kuna) | MIT | https://github.com/tabler/tabler-icons |
| sonner | Emil Kowalski | MIT | https://github.com/emilkowalski/sonner |
| react-easy-crop | Valentin Hervieu | MIT | https://github.com/ValentinH/react-easy-crop |
| idb | Jake Archibald | ISC | https://github.com/jakearchibald/idb |
| pica | nodeca (Vitaly Puzrin) | MIT | https://github.com/nodeca/pica |
| wasm-vips | Kleis Auke Wolthuizen | MIT | https://github.com/kleisauke/wasm-vips |
| Vite | Evan You and the Vite team | MIT | https://github.com/vitejs/vite |
| @vitejs/plugin-react | Vite team | MIT | https://github.com/vitejs/vite-plugin-react |
| @vitejs/plugin-basic-ssl | Vite team | MIT | https://github.com/vitejs/vite-plugin-basic-ssl |
| vite-plugin-sri3 | yoyo930021 | MIT | https://github.com/yoyo930021/vite-plugin-sri3 |
| Vitest | Vitest team | MIT | https://github.com/vitest-dev/vitest |
| Testing Library | Testing Library contributors | MIT | https://testing-library.com |
| MSW | Mock Service Worker | MIT | https://github.com/mswjs/msw |
| jsdom | jsdom contributors | MIT | https://github.com/jsdom/jsdom |
| fake-indexeddb | Jeremy Scheff | Apache-2.0 | https://github.com/dumbmatter/fakeIndexedDB |
| ESLint | ESLint team (OpenJS Foundation) | MIT | https://github.com/eslint/eslint |
| @eslint/js | ESLint team (OpenJS Foundation) | MIT | https://github.com/eslint/eslint |
| eslint-plugin-jsx-a11y | jsx-eslint contributors | MIT | https://github.com/jsx-eslint/eslint-plugin-jsx-a11y |
| eslint-plugin-react-hooks | Meta Platforms, Inc. | MIT | https://github.com/facebook/react |
| eslint-plugin-react-refresh | Arnaud Barré | MIT | https://github.com/ArnaudBarre/eslint-plugin-react-refresh |
| typescript-eslint | typescript-eslint team | MIT | https://github.com/typescript-eslint/typescript-eslint |
| TypeScript | Microsoft Corporation | Apache-2.0 | https://github.com/microsoft/TypeScript |
| Prettier | Prettier contributors | MIT | https://github.com/prettier/prettier |
| globals | Sindre Sorhus | MIT | https://github.com/sindresorhus/globals |
| @types/* | DefinitelyTyped contributors | MIT | https://github.com/DefinitelyTyped/DefinitelyTyped |

## Backend

| Component | Author or organization | License | Link |
|---|---|---|---|
| Gin | Gin contributors | MIT | https://github.com/gin-gonic/gin |
| GORM, GORM MySQL driver | GORM | MIT | https://github.com/go-gorm/gorm |
| go-redis (v8) | go-redis authors | BSD-2-Clause | https://github.com/go-redis/redis |
| go-sql-driver/mysql | go-sql-driver | MPL-2.0 | https://github.com/go-sql-driver/mysql |
| AWS SDK for Go v2 | Amazon Web Services | Apache-2.0 | https://github.com/aws/aws-sdk-go-v2 |
| Smithy Go | Amazon Web Services | Apache-2.0 | https://github.com/aws/smithy-go |
| Prometheus Go client | Prometheus authors | Apache-2.0 | https://github.com/prometheus/client_golang |
| golang.org/x/crypto | The Go Authors | BSD-3-Clause | https://pkg.go.dev/golang.org/x/crypto |
| golang.org/x/sys | The Go Authors | BSD-3-Clause | https://pkg.go.dev/golang.org/x/sys |

## Services and Runtime Components

These components are referenced by the official deployment as separate
container images or bundled runtime libraries. They are used unmodified and are
not linked into the summeRain application binaries.

| Component | Author or organization | License | Link |
|---|---|---|---|
| MySQL 8.4 (Community) | Oracle Corporation | GPL-2.0 | https://github.com/mysql/mysql-server |
| Redis 8 | Redis Ltd. | RSALv2 / SSPLv1 / AGPLv3 (tri-license, at your option) | https://github.com/redis/redis |
| imgproxy 4 | Evil Martians | Apache-2.0 | https://github.com/imgproxy/imgproxy |
| libvips | John Cupitt and libvips contributors | LGPL-2.1-or-later | https://github.com/libvips/libvips |
| Alpine Linux (base image) | Alpine Linux development team | Various OSS licenses | https://alpinelinux.org |

## Toolchains

| Component | Author or organization | License | Link |
|---|---|---|---|
| Go | The Go Authors | BSD-3-Clause | https://go.dev |
| Node.js | OpenJS Foundation | MIT | https://nodejs.org |

## Notes

- The direct dependencies above are licensed under MIT, ISC, BSD, Apache-2.0,
  or MPL-2.0. No GPL, AGPL, or SSPL code is linked into the application
  binaries.
- `wasm-vips` is an MIT-licensed WebAssembly wrapper around `libvips`
  (LGPL-2.1-or-later), which it bundles for browser use. imgproxy also uses
  `libvips` as a shared library inside its own container. In both cases
  `libvips` is consumed unmodified.
- MySQL and Redis run as separate, unmodified container services; the project
  does not link against or redistribute their code. Redis 8 is offered by
  Redis Ltd. under a choice of RSALv2, SSPLv1, or AGPLv3.
- When a dependency is added or removed, update this page and the
  [`NOTICE`](../NOTICE) file in the same change.
