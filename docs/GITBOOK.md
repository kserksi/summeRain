# GitBook Publication

This repository is the source of truth for the summeRain documentation. GitBook
Git Sync reads the repository root according to [`.gitbook.yaml`](../.gitbook.yaml),
uses the root [`README.md`](../README.md) as the documentation home page, and
uses [`SUMMARY.md`](../SUMMARY.md) as the navigation definition.

## Initial Import

An administrator or creator of the GitBook organization must complete the
account-side connection:

1. Create or select a GitBook space.
2. Choose **Set up Git Sync** and install or authorize the GitBook GitHub App.
3. Grant the app access to `kserksi/summeRain`.
4. If `main` is protected, explicitly allow the `gitbook-com` GitHub App to
   bypass the applicable push restriction. Git Sync is bidirectional and needs
   write access even when the initial direction imports from GitHub; grant this
   permission only after reviewing the repository rules and app access.
5. Select repository `kserksi/summeRain` and branch `main`.
6. Set the Git Sync project directory to `/` so GitBook finds the root
   `.gitbook.yaml` file.
7. Choose **GitHub -> GitBook** for the initial synchronization direction.
8. Add the synchronized space to a Docs site, set its audience to **Public**,
   and publish it.

Do not choose `docs/` as the Git Sync project directory. The synchronized
content deliberately starts at the repository root so the project home,
community policies, schema migration reference, and `docs/` tree can share one
navigation without copied files.

## Editing Model

- Treat GitHub `main` as the published source branch.
- Change documentation through commits and pull requests.
- Keep the root README repository-managed. Do not create another README through
  the GitBook editor because duplicate README files can cause sync conflicts.
- Add each new public Markdown page exactly once to `SUMMARY.md`.
- Keep `SUMMARY.md` limited to version-controlled local pages. Put external
  references inside the relevant page instead of in the navigation tree.
- Run `bash scripts/verify-gitbook-docs.sh` before committing.
- Keep assets within the synchronized repository root and reference them with
  relative paths.

Git Sync is bidirectional after the initial import. Repository changes sync to
GitBook, while accepted GitBook change requests can sync back to GitHub. Review
generated Git changes before merging them.

## Content Policy

Every tracked Markdown document must appear exactly once in `SUMMARY.md`, except
`SUMMARY.md` itself. Historical plans are published under **Archived Design
Records** and carry an archive notice so they are not mistaken for current
operational guidance.

Private local incident material is excluded from version control and
documentation publication. Never add ignored local incident records to
`SUMMARY.md`.

## Official GitBook References

- [Git Sync overview](https://gitbook.com/docs/getting-started/git-sync)
- [Import content with Git Sync](https://gitbook.com/docs/guides/editing-and-publishing-documentation/import-or-migrate-your-content-to-gitbook-with-git-sync)
- [Content configuration](https://gitbook.com/docs/getting-started/git-sync/content-configuration)
- [Git Sync troubleshooting](https://gitbook.com/docs/getting-started/git-sync/troubleshooting)
