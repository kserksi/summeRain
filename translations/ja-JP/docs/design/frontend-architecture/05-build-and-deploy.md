# 05 · ビルドとデプロイ

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前の内容であり、
> バージョン、パス、実装状況が古くなっている可能性があります。

> 所属: [フロントエンド・アーキテクチャ設計（索引）](./README.md)

## ビルド

`vite build` -> `build.outDir: '../backend/web'`、`build.emptyOutDir: true`、`base: '/'`。

> **`base` は相対値 `'./'` ではなく `'/'` でなければなりません:** BrowserRouter は HTML5 History ルーティングを使用します。`/images/123` のような深いリンクを再読み込みすると、バックエンドの `NoRoute` ハンドラーが `index.html` にフォールバックし、ブラウザーは**現在のドキュメント URL**を基準にアセットパスを解決します。`base` が相対値 `./` の場合、`./assets/index.js` は `/images/assets/index.js` として解決され、HTML フォールバックに到達してスクリプトの読み込みに失敗します。Vite の公式ドキュメントも、相対 `base` は History ルートの再読み込みと互換性がないと警告しています。本フロントエンドは**ドメインのルート**にデプロイされ（Go が `./web/` から同一オリジンで配信）、`base: '/'` が適切です。将来サブパスへデプロイする場合は、`base` と React Router の `basename` を同時に変更する必要があります（現時点では不要です）。

本番ビルドでは `vite-plugin-sri3` を有効にして SHA512 の `integrity` 属性を注入し、`assets.manifest.json` を出力します（成果物ごとに `{ version, integrity }` を記録。詳細は [07 本番標準](07-production-standards.md)を参照）。

## 本番デプロイ

単一の Go プロセスが `backend/web/`、`/api`、`/i` を同一オリジンで配信するため、クッキーも自然に同一オリジンになります。

## 開発ワークフロー

Vite は HTTPS の `:5173` で稼働します。`server.proxy` が `/api` と `/i` を `http://localhost:8080` の Go へ転送し、開発中もクッキーの同一オリジン性を保ちます。

**`__Host-` クッキーに関するプロキシ要件**（満たさない場合、ブラウザーがクッキーを拒否し、セッションを保持できません）:
- `changeOrigin: true` を設定し、`cookieDomainRewrite` と `cookiePathRewrite` は**設定しません**（`__Host-` は Domain 属性なし、Path=/ を要求し、書き換えるとプレフィックス規則に違反します）。
- 応答の `Set-Cookie` ヘッダーを変更せず転送します。`:5173` の HTTPS が `Secure` フラグを満たします。上流の Go は HTTP ですが、`c.SetCookie` は引き続き `Secure` を出力し、ブラウザーは HTTPS 側のプロキシを通して受け入れられます。
- クッキーはプロキシオリジンの `localhost:5173` に保存され、その後の `/api` と `/i` リクエストに自動で付与されます。
- 以下の HTTPS 前提条件と組み合わせて有効になります。

## ⚠️ HTTPS は必須（開発環境の必須前提条件）

`__Host-` プレフィックス付きクッキーには `Secure` と HTTPS が必要です。ローカル HTTP ではブラウザーが `__Host-session_token` の保存を拒否し、ログイン状態を保持できません。開発環境では Vite の HTTPS を**必ず**有効にしてください（`@vitejs/plugin-basic-ssl` または mkcert）。有効でなければログイン連携は正常に動作しません。

---

<- [04 テーマと UI](04-theme-and-ui.md) · [索引](./README.md) · 次: [06 テスト戦略](06-testing.md)
