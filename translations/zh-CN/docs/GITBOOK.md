# GitBook 发布

规范公开文档站点的默认英文根地址是
[summerain-1.gitbook.io/summerain](https://summerain-1.gitbook.io/summerain/)。
简体中文读者应使用
[summerain-1.gitbook.io/summerain/zh-cn](https://summerain-1.gitbook.io/summerain/zh-cn/)，
以便在浏览文档时保持中文语言变体。

本仓库是三个由仓库管理的 GitBook Space 的唯一事实来源：规范英文、简体中文
（`zh-CN`）和日语（`ja-JP`）。这三个 Space 作为同一 Docs site 的语言变体发布。
英文是默认变体，读者可通过 GitBook 的语言选择器切换至中文或日语。

## 仓库布局

英文 Space 按照 [`../.gitbook.yaml`](../.gitbook.yaml) 读取仓库根目录，以
[`../README.md`](../README.md) 作为首页，并以 [`../SUMMARY.md`](../SUMMARY.md)
定义导航。

翻译 Space 在各自的项目目录中使用相同的相对页面布局：

| 语言 | Git Sync 项目目录 | 首页 | 导航 |
|---|---|---|---|
| 英文 | `/` | `README.md` | `SUMMARY.md` |
| 简体中文 | `/translations/zh-CN` | `README.md` | `SUMMARY.md` |
| 日语 | `/translations/ja-JP` | `README.md` | `SUMMARY.md` |

每个翻译项目目录都包含自己的 `.gitbook.yaml`。翻译页面必须与英文源页面保持相同的
相对路径。例如，`docs/USAGE.md` 分别镜像为
`translations/zh-CN/docs/USAGE.md` 和 `translations/ja-JP/docs/USAGE.md`。

## 初次导入与语言变体

GitBook 组织的管理员或创建者必须完成账户侧连接：

1. 复用已连接公开站点的现有英文 Space，并为简体中文和日语另外创建两个 Space。
2. 在两个新的翻译 Space 中选择 **Set up Git Sync**；如有需要，安装或授权 GitBook
   GitHub App。
3. 授予该 App 对 `kserksi/summeRain` 的访问权限。现有英文 Space 继续使用 `main`
   分支，两个翻译 Space 也选择 `main`。
4. 严格按照上表配置项目目录，使 GitBook 能找到对应的 `.gitbook.yaml`。
5. 为两个新的翻译 Space 选择 **GitHub -> GitBook** 作为初始同步方向。
6. 在 Docs site 设置中，将英文 Space 关联为默认变体。
7. 将中文和日语 Space 添加为变体，分配对应语言，并使用 `zh-cn`、`ja` 等稳定 slug。
8. 将 Docs site 的受众设为 **Public**，发布后验证语言选择器能在三个变体的等价页面间切换。

如果 `main` 受保护，请明确允许 `gitbook-com` GitHub App 绕过适用的 push 限制。
Git Sync 是双向的，即使初始方向是从 GitHub 导入，也需要写权限。只有在审查仓库规则和
App 访问范围后，才能授予此权限。

不要将 `docs/` 用作英文 Git Sync 项目目录。英文 Space 有意从仓库根目录开始，
从而让项目首页、社区政策、Schema 迁移参考和 `docs/` 目录树共享同一导航，而无需复制
规范文件。同样，每个翻译 Space 只能连接到其准确的翻译项目目录；绝不能让多个 Space
连接同一个翻译目录。

## 编辑模型

- 将 GitHub `main` 视为三个 Space 的发布源分支。
- 通过提交和 Pull Request 修改文档。
- 先编写并审查规范英文页面，然后在同一次变更中更新相同相对路径下的两份翻译。
- 所有 README 都由仓库管理。不要在 GitBook 编辑器中另建 README，因为重复的
  README 文件可能造成同步冲突。
- 每个公开页面必须在对应语言的 `SUMMARY.md` 中且仅出现一次。翻译读者可见标签时，
  三套导航结构必须保持一致。
- 每个 `SUMMARY.md` 只能包含受版本控制的本地页面。外部引用应放在相关页面内，
  不得放入导航树。
- 翻译并审阅所有发生变化的页面后，从仓库根目录运行
  `bash scripts/update-translation-source-hashes.sh`。不得在未更新两种译文时单独刷新哈希。
- 提交前运行 `bash scripts/verify-gitbook-docs.sh`。
- 资源文件应位于相应的同步项目目录内，并使用相对路径。

首次导入后，Git Sync 仍保持双向同步。仓库变更会同步到 GitBook，而被接受的 GitBook
Change Request 也可同步回 GitHub。合并前必须审查生成的 Git 变更，尤其是涉及多个
语言 Space 的变更。

## 内容政策

英文文档是规范版本。`zh-CN` 与 `ja-JP` 目录树必须在相同相对路径下，包含每个公开
英文 Markdown 页面的完整翻译。除 SUMMARY 文件本身外，每种语言的 `SUMMARY.md`
必须且只能列出每个页面一次。历史方案发布在 **Archived Design Records** 下，并带有
归档提示，避免被误认为当前运维指导。

本地私有事故材料不得进入版本控制，也不得进入任何文档 Space。绝不能将其复制到翻译
目录树或加入任何 `SUMMARY.md`。

## GitBook 官方参考

- [Git Sync 概览](https://gitbook.com/docs/getting-started/git-sync)
- [通过 Git Sync 导入内容](https://gitbook.com/docs/guides/editing-and-publishing-documentation/import-or-migrate-your-content-to-gitbook-with-git-sync)
- [内容配置](https://gitbook.com/docs/getting-started/git-sync/content-configuration)
- [Monorepo 项目目录](https://gitbook.com/docs/getting-started/git-sync/monorepos)
- [内容变体](https://gitbook.com/docs/publishing-documentation/site-structure/variants)
- [使用变体本地化文档](https://gitbook.com/docs/guides/content-organization-and-localization/localize-your-docs-with-variants-in-gitbook)
- [Git Sync 故障排查](https://gitbook.com/docs/getting-started/git-sync/troubleshooting)
