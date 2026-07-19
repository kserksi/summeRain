# リリースとタグの管理

`VERSION` はプロジェクトリリースにおける唯一の信頼できる情報源です。開発版は
`dev` から、安定版は `main` または `master` から公開し、両チャンネルでは別々の
イメージタグ名前空間を使用します。

## タグ規則

| シナリオ | 入力バージョン | コンテナタグ |
|---|---|---|
| `dev` への通常の push | バージョン変更なし | `dev`、`dev-sha-<short-commit>` |
| `dev` からの開発版リリース | `2.0.1` | `dev-v2.0.1`、`dev-2.0.1`、`dev`、`dev-sha-<short-commit>` |
| `main` / `master` への通常の push | バージョン変更なし | `main`、`main-sha-<short-commit>` |
| `main` / `master` からの安定版リリース | `2.1.0` | `v2.1.0`、`2.1.0`、`2.1`、`2`、`latest`、`main`、`main-sha-<short-commit>` |
| 安定チャンネルのプレリリース | `2.2.0-rc.1` | `v2.2.0-rc.1`、`2.2.0-rc.1`、`main`、`main-sha-<short-commit>` |

安定チャンネルの正確なタグと `dev-vX.Y.Z` / `dev-X.Y.Z` は上書きできません。
`dev`、`main`、`latest`、`X`、`X.Y` は移動可能なエイリアスです。`latest` は
`main` または `master` の安定版リリースだけが更新します。
開発ブランチの GitHub Release はプレリリースとして公開します。開発版を同じ
バージョン番号のまま昇格してはいけません。開発シリーズの完了後、安定ブランチは
新しいバージョン番号を公開します。

## リリース手順

1. リリース対象ブランチでフロントエンドとバックエンドのチェックが成功していることを確認します。
2. 下記の厳密な形式に従って新しいバージョンを選びます。過去のバージョン番号を
   再利用してはいけません。
3. `VERSION` の変更だけを、`chore: release vX.Y.Z` というメッセージで
   コミットします。
4. 開発版は `dev`、安定版は `main` に push し、`CI and Docker` の完了を待ちます。
5. GitHub Releases、Docker Hub、GHCR のバージョンタグとマルチプラットフォーム
   マニフェストを検証します。また、Docker Hub のリポジトリ説明にルートの
   `README.md` が同期されていることも確認します。

例：

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:dev
```

## バージョンの選択

- Patch：後方互換性のある不具合修正。例：`1.2.3` から `1.2.4`。
- Minor：後方互換性のある新機能。例：`1.2.3` から `1.3.0`。
- Major：互換性のない変更。例：`1.2.3` から `2.0.0`。
- Pre-release：`2.0.0-rc.1` などの候補版。安定版エイリアスは更新しません。

`VERSION` は SemVer 2.0.0 の core および pre-release 構文に従います。major、
minor、patch、ならびに数字だけのプレリリース識別子に先頭ゼロを付けてはいけません。
したがって `1.2.3` と `1.2.3-rc.1` は有効ですが、`01.2.3` と
`1.2.3-01` は無効です。Docker タグでは `+` を損失なく表現できないため、
プロジェクトのリリースバージョンでは build metadata を使用できません。
`v<version>` が Docker の 128 文字というタグ上限内に収まるよう、バージョンは
最大 127 ASCII 文字です。コミット前に
`bash scripts/validate-release-version.sh "$(< VERSION)"` を実行してください。

`main` / `master` のイメージ公開が成功するたびに、`dockerhub_metadata` Job は
ルートの `README.md` を `jaykserks/summerain` へ同期します。開発ビルドは安定版の
Docker Hub 説明や `latest` を置き換えません。正確なタグを push する前と
メタデータ公開時に不変タグポリシーを更新するのは正式リリースだけです。
ポリシーヘルパーは Docker Hub 管理 API の一時的な 5xx 応答に対して回数を制限した
再試行を行います。`latest`、`main`、`dev`、`X`、`X.Y`、チャンネル接頭辞付きの
commit タグは不変ルールに一致しないため、引き続き移動できます。

正式リリースを再実行すると、ワークフローは Docker Hub と GHCR にある対象
チャンネルの正確なタグから registry descriptor digest を読み取ります。いずれかの
registry に正確なタグが存在すれば、そのダイジェストを復旧元とします。ワークフローは
両方の registry で不足している正確なタグを補い、対象チャンネルのエイリアスだけを
同じダイジェストへ向け直します。既存の不変タグを
再ビルドまたは上書きすることはありません。registry 内または registry 間で正確な
タグのダイジェストが一致しない場合、ワークフローは失敗して手動調査を求めます。
どのイメージが正しいかを推測することはありません。

これらの Docker Hub 操作は、リポジトリ Secrets の `DOCKERHUB_USERNAME` と
`DOCKERHUB_TOKEN` を共用します。トークンには、イメージの push、タグポリシーの
設定、リポジトリ説明の更新に使用する `read/write/delete` 権限が必要です。

## サプライチェーンの固定

ワークフロー内のすべてのサードパーティ GitHub Actions は、完全な commit SHA に
固定されています。行末コメントには対応するメジャーバージョンを記録しています。
Action を更新する際は、上流 release と新しい SHA の両方を確認してください。

`requirements.lock` は、`linux/amd64` と `linux/arm64` の両方をサポートする必要が
あるサービスイメージを、正確なバージョンタグで記録します。プラットフォーム固有の
child manifest digest はアーキテクチャ間で移植できないため、共有ロックファイルには
記録しません。本番デプロイをダイジェストで固定する場合は、公開された OCI index /
manifest-list digest を使用してください。

## ロールバック

正確なバージョンタグを移動または再利用してはいけません。ロールバックするには、
`DOCKER_IMAGE` を動作確認済みの正確なバージョンまたは OCI マルチプラットフォーム
インデックスダイジェストに設定し、`--no-build` で再デプロイします。問題を修正した後、
新しい Patch リリースを公開してください。
