# GitBook Publication

The canonical public documentation site is
[summerain-1.gitbook.io/summerain](https://summerain-1.gitbook.io/summerain/).

This repository is the source of truth for three repository-managed GitBook
spaces: canonical English, Simplified Chinese (`zh-CN`), and Japanese (`ja-JP`).
The spaces are published as language variants of the same Docs site. English is
the default variant, and readers select Chinese or Japanese through GitBook's
language picker.

## Repository Layout

The English space reads the repository root according to
[`../.gitbook.yaml`](../.gitbook.yaml), uses [`../README.md`](../README.md) as its
home page, and uses [`../SUMMARY.md`](../SUMMARY.md) as its navigation.

The translated spaces use the same relative page layout under their own project
directories:

| Language | Git Sync project directory | Home page | Navigation |
|---|---|---|---|
| English | `/` | `README.md` | `SUMMARY.md` |
| Simplified Chinese | `/translations/zh-CN` | `README.md` | `SUMMARY.md` |
| Japanese | `/translations/ja-JP` | `README.md` | `SUMMARY.md` |

Each translated project directory contains its own `.gitbook.yaml`. A translated
page must have the same relative path as its English source. For example,
`docs/USAGE.md` is mirrored by `translations/zh-CN/docs/USAGE.md` and
`translations/ja-JP/docs/USAGE.md`.

## Initial Import and Language Variants

An administrator or creator of the GitBook organization must complete the
account-side connection:

1. Reuse the existing English space connected to the public site, and create two
   additional spaces for Simplified Chinese and Japanese.
2. Choose **Set up Git Sync** for each new translated space and install or
   authorize the GitBook GitHub App if necessary.
3. Grant the app access to `kserksi/summeRain`. Keep the existing English space
   on branch `main`, and select `main` for both translated spaces.
4. Configure the project directories exactly as shown in the table above so
   GitBook finds the corresponding `.gitbook.yaml` file.
5. Choose **GitHub -> GitBook** as the initial synchronization direction for
   both new translated spaces.
6. In the Docs site settings, link the English space as the default variant.
7. Add the Chinese and Japanese spaces as variants, assign their respective
   languages, and use stable slugs such as `zh-cn` and `ja`.
8. Set the Docs site audience to **Public**, publish it, and verify that the
   language picker switches between equivalent pages in all three variants.

If `main` is protected, explicitly allow the `gitbook-com` GitHub App to bypass
the applicable push restriction. Git Sync is bidirectional and needs write
access even when the initial direction imports from GitHub. Grant this permission
only after reviewing the repository rules and app access.

Do not use `docs/` as the English Git Sync project directory. The English space
deliberately starts at the repository root so the project home, community
policies, schema migration reference, and `docs/` tree can share one navigation
without copied canonical files. Likewise, connect each translated space only to
its exact translation project directory; never connect multiple spaces to the
same translation directory.

## Editing Model

- Treat GitHub `main` as the published source branch for all three spaces.
- Make documentation changes through commits and pull requests.
- Write and review the canonical English page first, then update both translated
  pages at the same relative path in the same change.
- Keep each README repository-managed. Do not create another README in the
  GitBook editor, because duplicate README files can cause synchronization
  conflicts.
- Add every public page exactly once to the `SUMMARY.md` for its language. Keep
  the three navigation structures aligned while translating reader-facing
  labels.
- Keep every `SUMMARY.md` limited to version-controlled local pages. Put external
  references inside the relevant page instead of in the navigation tree.
- After translating and reviewing every changed page, run
  `bash scripts/update-translation-source-hashes.sh` from the repository root.
  Never refresh the hashes without updating both translations.
- Run `bash scripts/verify-gitbook-docs.sh` before committing.
- Keep assets inside the applicable synchronized project directory and use
  relative paths.

Git Sync remains bidirectional after the initial import. Repository changes sync
to GitBook, while accepted GitBook change requests can sync back to GitHub.
Review generated Git changes before merging them, especially changes that affect
more than one language space.

## Content Policy

The English documentation is canonical. The `zh-CN` and `ja-JP` trees must
contain a complete translation of every public English Markdown page at the same
relative path. Each language's `SUMMARY.md` must list every page exactly once,
except for the summary file itself. Historical plans are published under
**Archived Design Records** and carry an archive notice so they are not mistaken
for current operational guidance.

Private local incident material is excluded from version control and from every
documentation space. Never copy it into a translation tree or add it to any
`SUMMARY.md`.

## Official GitBook References

- [Git Sync overview](https://gitbook.com/docs/getting-started/git-sync)
- [Import content with Git Sync](https://gitbook.com/docs/guides/editing-and-publishing-documentation/import-or-migrate-your-content-to-gitbook-with-git-sync)
- [Content configuration](https://gitbook.com/docs/getting-started/git-sync/content-configuration)
- [Monorepo project directories](https://gitbook.com/docs/getting-started/git-sync/monorepos)
- [Content variants](https://gitbook.com/docs/publishing-documentation/site-structure/variants)
- [Localize documentation with variants](https://gitbook.com/docs/guides/content-organization-and-localization/localize-your-docs-with-variants-in-gitbook)
- [Git Sync troubleshooting](https://gitbook.com/docs/getting-started/git-sync/troubleshooting)
