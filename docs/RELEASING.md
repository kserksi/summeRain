# Release and Tag Management

`VERSION` is the single source of truth for project releases. Development
releases are cut from `dev`; stable releases are cut from `main` or `master`.
Images from the two channels use separate tag namespaces.

## Tagging Rules

| Scenario | Input version | Container tags |
|---|---|---|
| Ordinary push to `dev` | No version change | `dev`, `dev-sha-<short-commit>` |
| Development release from `dev` | `2.0.1` | `dev-v2.0.1`, `dev-2.0.1`, `dev`, `dev-sha-<short-commit>` |
| Ordinary push to `main` / `master` | No version change | `main`, `main-sha-<short-commit>` |
| Stable release from `main` / `master` | `2.1.0` | `v2.1.0`, `2.1.0`, `2.1`, `2`, `latest`, `main`, `main-sha-<short-commit>` |
| Stable-channel prerelease | `2.2.0-rc.1` | `v2.2.0-rc.1`, `2.2.0-rc.1`, `main`, `main-sha-<short-commit>` |

Exact stable tags and `dev-vX.Y.Z` / `dev-X.Y.Z` tags must never be
overwritten. `dev`, `main`, `latest`, `X`, and `X.Y` are moving aliases.
`latest` is written only by a stable release from `main` or `master`.
Development GitHub Releases are marked as prereleases. A development version
is never promoted under the same version number; the stable branch must publish
a new version after the completed development series.

## Release Procedure

1. Confirm that the frontend and backend checks pass on the release branch.
2. Select a new version using the strict format below. Never reuse a historical
   version number.
3. Commit the `VERSION` change separately with the message
   `chore: release vX.Y.Z`.
4. Push the commit to `dev` for a development release or to `main` for a stable
   release, then wait for `CI and Docker` to finish.
5. Verify the version tags and multi-platform manifests on GitHub Releases,
   Docker Hub, and GHCR. Also confirm that the Docker Hub repository description
   has synchronized the root `README.md`.

Example:

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:dev
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
`jaykserks/summerain`. Development builds never replace the stable Docker Hub
description or the `latest` tag. Only a formal release refreshes the
immutable-tag policy before exact tags are pushed and again during metadata
publication. The policy helper performs bounded retries for transient 5xx
responses from the Docker Hub management API. `latest`, `main`, `dev`, `X`,
`X.Y`, and channel-prefixed commit tags do not match the immutability rule, so
they remain movable according to the policy above.

When a formal release is rerun, the workflow reads the registry descriptor
digest for the channel's exact version tags in Docker Hub and GHCR. If either
registry already contains an exact tag, that digest becomes the recovery
source. The workflow fills in missing exact tags in both registries and
repoints only the aliases belonging to that channel without rebuilding
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
