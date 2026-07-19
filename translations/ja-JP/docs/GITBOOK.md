# GitBook での公開

正式な公開ドキュメントサイトの既定の英語ルートは
[summerain-1.gitbook.io/summerain](https://summerain-1.gitbook.io/summerain/) です。
日本語で閲覧を続ける場合は、日本語バリアントの
[summerain-1.gitbook.io/summerain/ja](https://summerain-1.gitbook.io/summerain/ja/)
を使用してください。

このリポジトリは、リポジトリで管理される 3 つの GitBook Space、すなわち正規の
英語、簡体字中国語（`zh-CN`）、日本語（`ja-JP`）における唯一の信頼できる情報源です。
各 Space は同じ Docs site の言語バリアントとして公開されます。英語を既定の
バリアントとし、読者は GitBook の言語ピッカーから中国語または日本語を選択します。

## リポジトリの構成

英語 Space は [`../.gitbook.yaml`](../.gitbook.yaml) に従ってリポジトリのルートを
読み取り、[`../README.md`](../README.md) をホームページ、
[`../SUMMARY.md`](../SUMMARY.md) をナビゲーションとして使用します。

翻訳 Space は、それぞれのプロジェクトディレクトリ内で同じ相対ページ構成を使用します。

| 言語 | Git Sync プロジェクトディレクトリ | ホームページ | ナビゲーション |
|---|---|---|---|
| 英語 | `/` | `README.md` | `SUMMARY.md` |
| 簡体字中国語 | `/translations/zh-CN` | `README.md` | `SUMMARY.md` |
| 日本語 | `/translations/ja-JP` | `README.md` | `SUMMARY.md` |

各翻訳プロジェクトディレクトリには、それぞれの `.gitbook.yaml` があります。翻訳ページは
英語の原文と同じ相対パスに置かなければなりません。たとえば `docs/USAGE.md` は、
`translations/zh-CN/docs/USAGE.md` および
`translations/ja-JP/docs/USAGE.md` に対応します。

## 初回インポートと言語バリアント

GitBook Organization の管理者または作成者が、アカウント側で次の接続作業を行います。

1. 公開サイトに接続済みの既存の英語 Space を再利用し、簡体字中国語と日本語用に
   2 つの Space を追加します。
2. 新しい翻訳 Space で **Set up Git Sync** を選択し、必要に応じて GitBook GitHub
   App をインストールまたは認可します。
3. App に `kserksi/summeRain` へのアクセスを許可します。既存の英語 Space は
   `main` ブランチを維持し、2 つの翻訳 Space でも `main` を選択します。
4. GitBook が対応する `.gitbook.yaml` を検出できるよう、上表どおりにプロジェクト
   ディレクトリを設定します。
5. 新しい 2 つの翻訳 Space で、初回同期方向に **GitHub -> GitBook** を選択します。
6. Docs site の設定で、英語 Space を既定バリアントとしてリンクします。
7. 中国語と日本語の Space をバリアントとして追加し、それぞれの言語を割り当て、
   `zh-cn` や `ja` などの安定した slug を使用します。
8. Docs site の Audience を **Public** に設定して公開し、言語ピッカーで 3 つの
   バリアントの同等ページを切り替えられることを確認します。

`main` が保護されている場合は、該当する push 制限を `gitbook-com` GitHub App が
回避できるよう明示的に許可してください。初回方向が GitHub からのインポートであっても、
Git Sync は双方向であるため書き込み権限を必要とします。リポジトリルールと App の
アクセス範囲を確認した後に限り、この権限を付与してください。

英語の Git Sync プロジェクトディレクトリに `docs/` を指定しないでください。英語 Space
は意図的にリポジトリルートから開始し、正規ファイルを複製せずに、プロジェクトホーム、
コミュニティポリシー、Schema migration リファレンス、`docs/` ツリーで 1 つの
ナビゲーションを共有します。同様に、各翻訳 Space は対応する翻訳プロジェクトディレクトリ
だけに接続し、複数の Space を同じ翻訳ディレクトリへ接続してはいけません。

## 編集モデル

- GitHub の `main` を 3 つすべての Space の公開元ブランチとして扱います。
- ドキュメントはコミットと Pull Request を通じて変更します。
- まず正規の英語ページを作成してレビューし、同じ変更内で同一相対パスにある 2 つの
  翻訳ページも更新します。
- 各 README はリポジトリで管理します。README が重複すると同期競合の原因になるため、
  GitBook エディターで別の README を作成しないでください。
- 各公開ページは、その言語の `SUMMARY.md` に 1 回だけ追加します。読者向けラベルは
  翻訳しながら、3 言語のナビゲーション構造を一致させます。
- 各 `SUMMARY.md` には、バージョン管理されたローカルページだけを含めます。外部参照は
  ナビゲーションツリーではなく、関連ページ内に記載します。
- 変更されたすべてのページを翻訳してレビューした後、リポジトリルートから
  `bash scripts/update-translation-source-hashes.sh` を実行します。両方の翻訳を更新せずに
  ハッシュだけを更新してはいけません。
- コミット前に `bash scripts/verify-gitbook-docs.sh` を実行します。
- アセットは該当する同期プロジェクトディレクトリ内に置き、相対パスで参照します。

初回インポート後も Git Sync は双方向です。リポジトリの変更は GitBook へ同期され、
承認された GitBook Change Request は GitHub へ同期される場合があります。マージ前に、
特に複数の言語 Space に及ぶ生成済み Git 変更を確認してください。

## コンテンツポリシー

英語ドキュメントを正規版とします。`zh-CN` と `ja-JP` のツリーには、すべての公開英語
Markdown ページの完全な翻訳を同じ相対パスで含めなければなりません。SUMMARY ファイル
自体を除き、各言語の `SUMMARY.md` にはすべてのページを 1 回ずつ掲載します。過去の計画は
**Archived Design Records** の下で公開し、現在の運用ガイダンスと誤認されないよう
アーカイブ済みであることを明記します。

非公開のローカルインシデント資料は、バージョン管理およびすべてのドキュメント Space から
除外します。翻訳ツリーへコピーしたり、いずれかの `SUMMARY.md` に追加したりしてはいけません。

## GitBook 公式リファレンス

- [Git Sync の概要](https://gitbook.com/docs/getting-started/git-sync)
- [Git Sync でコンテンツをインポートする](https://gitbook.com/docs/guides/editing-and-publishing-documentation/import-or-migrate-your-content-to-gitbook-with-git-sync)
- [コンテンツ設定](https://gitbook.com/docs/getting-started/git-sync/content-configuration)
- [Monorepo のプロジェクトディレクトリ](https://gitbook.com/docs/getting-started/git-sync/monorepos)
- [コンテンツバリアント](https://gitbook.com/docs/publishing-documentation/site-structure/variants)
- [バリアントを利用したドキュメントのローカライズ](https://gitbook.com/docs/guides/content-organization-and-localization/localize-your-docs-with-variants-in-gitbook)
- [Git Sync のトラブルシューティング](https://gitbook.com/docs/getting-started/git-sync/troubleshooting)
