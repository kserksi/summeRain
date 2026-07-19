# Release and Tag Management

`VERSION` is the single source of truth for formal project releases. Ordinary
code commits update only the development image; a formal release is created only
when a commit on `main` or `master` changes `VERSION`.

## Tagging Rules

| Scenario | Input version | Container tags |
|---|---|---|
| Ordinary push to `main` / `master` | No version change | `edge`, `sha-<short-commit>` |
| Stable release | `1.2.3` | `v1.2.3`, `1.2.3`, `1.2`, `1`, `latest`, `sha-<short-commit>` |
| Pre-release | `1.3.0-rc.1` | `v1.3.0-rc.1`, `1.3.0-rc.1`, `sha-<short-commit>` |

Exact version tags, including `vX.Y.Z`, `X.Y.Z`, and their pre-release forms,
must never be overwritten. `latest`, `X`, and `X.Y` are moving aliases. Each
stable release moves them to the newest version within the corresponding
compatibility range.

## Release Procedure

1. Confirm that the frontend and backend checks pass on `main`.
2. Select a new version using the strict format below. Never reuse a historical
   version number.
3. Commit the `VERSION` change separately with the message
   `chore: release vX.Y.Z`.
4. Push the commit and wait for `CI and Docker` to finish.
5. Verify the version tags and multi-platform manifests on GitHub Releases,
   Docker Hub, and GHCR. Also confirm that the Docker Hub repository description
   has synchronized the root `README.md`.

Example:

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:main
```

## Choosing a Version

- Patch: a backward-compatible defect fix, such as `1.2.3` to `1.2.4`.
- Minor: a backward-compatible feature, such as `1.2.3` to `1.3.0`.
- Major: an incompatible change, such as `1.2.3` to `2.0.0`.
- Pre-release: a candidate build, such as `2.0.0-rc.1`; it does not update
  stable aliases.

`VERSION` follows the SemVer 2.0.0 core and pre-release syntax. Major, minor,
patch, and numeric pre-release identifiers must not contain leading zeroes.
Therefore, `1.2.3` and `1.2.3-rc.1` are valid, while `01.2.3` and `1.2.3-01`
are invalid. Because Docker tags cannot represent `+` losslessly, project
release versions do not accept build metadata. A version may contain at most
127 ASCII characters so that `v<version>` remains within Docker's 128-character
tag limit. Before committing, run
`bash scripts/validate-release-version.sh "$(< VERSION)"`.

After every successful `main` / `master` image publication, the
`dockerhub_metadata` job synchronizes the root `README.md` to
`jaykserks/summerain`. Only a formal release refreshes the immutable-tag policy
before exact tags are pushed and again during metadata publication. The policy
helper performs bounded retries for transient 5xx responses from the Docker Hub
management API. `latest`, `X`, `X.Y`, `edge`, and commit tags do not match the
immutability rule, so they remain movable according to the policy above.

When a formal release is rerun, the workflow reads the registry descriptor
digest for exact version tags in Docker Hub and GHCR. If either registry already
contains `vX.Y.Z` or `X.Y.Z`, that digest becomes the recovery source. The
workflow fills in missing exact tags in both registries and repoints `latest`,
`X`, `X.Y`, and the current `sha-<commit>` to the same digest without rebuilding
or overwriting an existing immutable tag. If exact-tag digests disagree within
or between registries, the workflow fails for manual investigation; it never
guesses which image is correct.

These Docker Hub operations share the repository secrets
`DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`. The token requires
`read/write/delete` permissions for pushing images, configuring tag policy, and
updating the repository description.

## Supply-Chain Pinning

Every third-party GitHub Action in the workflow is pinned to a full commit SHA.
The end-of-line comment records the corresponding major version. Review both
the upstream release and the new SHA when upgrading an action.

`requirements.lock` records exact version tags for service images that must
support both `linux/amd64` and `linux/arm64`. A platform-specific child manifest
digest is not portable across architectures and therefore does not belong in
the shared lock file. When pinning a production deployment by digest, use the
published OCI index / manifest-list digest.

## Rollback

Never move or reuse an exact version tag. To roll back, set `DOCKER_IMAGE` to a
known-good exact version or OCI multi-platform index digest and redeploy with
`--no-build`. After correcting the problem, publish a new Patch release.
