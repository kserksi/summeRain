# 发布与标签管理

`VERSION` 是项目正式版本的唯一来源。普通代码提交只更新开发镜像；只有
`main` 或 `master` 上包含 `VERSION` 变更的提交才会创建正式发布。

## 标签规则

| 场景 | 输入版本 | 容器标签 |
|---|---|---|
| `main` / `master` 普通推送 | 无版本变更 | `edge`、`sha-<short-commit>` |
| 稳定版 | `1.2.3` | `v1.2.3`、`1.2.3`、`1.2`、`1`、`latest`、`sha-<short-commit>` |
| 预发布版 | `1.3.0-rc.1` | `v1.3.0-rc.1`、`1.3.0-rc.1`、`sha-<short-commit>` |

精确版本标签 `vX.Y.Z`、`X.Y.Z` 及其预发布形式不得覆盖。`latest`、`X`
和 `X.Y` 是可移动别名；稳定版发布时，它们会指向对应兼容范围内的最新版本。

## 发布流程

1. 确认 `main` 上的前后端检查均已通过。
2. 按下述严格格式选择新版本，且不得与历史版本号重复。
3. 单独提交 `VERSION` 变更，提交信息使用 `chore: release vX.Y.Z`。
4. 推送提交并等待 `CI and Docker` 完成。
5. 检查 GitHub Releases、Docker Hub 和 GHCR 中的版本标签及多平台清单，
   同时确认 Docker Hub 仓库说明已经同步根目录 `README.md`。

示例：

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:main
```

## 版本选择

- Patch：向后兼容的缺陷修复，例如从 `1.2.3` 升至 `1.2.4`。
- Minor：向后兼容的新功能，例如从 `1.2.3` 升至 `1.3.0`。
- Major：不兼容的变更，例如从 `1.2.3` 升至 `2.0.0`。
- Pre-release：候选版本，例如 `2.0.0-rc.1`；不会更新稳定别名。

`VERSION` 遵循 SemVer 2.0.0 的 core 与 pre-release 语法。主版本、次版本、
修订版本以及纯数字预发布标识均不得有前导零。因此 `1.2.3`、
`1.2.3-rc.1` 合法，而 `01.2.3`、`1.2.3-01` 非法。由于 Docker 标签无法
无损表达 `+`，项目发布版本不接受 build metadata。版本最多可包含 127 个
ASCII 字符，以确保 `v<version>` 仍满足 Docker 的 128 字符标签上限。提交前请运行
`bash scripts/validate-release-version.sh "$(< VERSION)"`。

每次 `main` / `master` 镜像发布成功后，`dockerhub_metadata` Job 都会将根目录
`README.md` 同步至 `jaykserks/summerain`。只有正式发布会在推送精确标签前及
元数据发布阶段刷新不可变标签策略；策略脚本会对 Docker Hub 管理 API 的临时
5xx 响应进行有限重试。`latest`、`X`、`X.Y`、`edge` 与提交标签不匹配不可变
规则，因此仍可按上述策略移动。

正式发布重跑时，工作流会读取 Docker Hub 与 GHCR 中精确版本标签的 registry
descriptor digest。只要任一 registry 已存在 `vX.Y.Z` 或 `X.Y.Z`，该摘要即为
恢复源。工作流会补齐两个 registry 缺失的精确标签，并将 `latest`、`X`、`X.Y`
及本次 `sha-<commit>` 重新指向同一摘要，不再构建或覆盖已有的不可变标签。
如果 registry 内部或跨 registry 的精确标签摘要不一致，工作流会失败并要求人工
核查，绝不会猜测哪份镜像正确。

上述 Docker Hub 操作共用仓库 Secrets `DOCKERHUB_USERNAME` 与
`DOCKERHUB_TOKEN`。令牌需要 `read/write/delete` 权限，用于推送镜像、配置标签
策略和更新仓库说明。

## 供应链固定

工作流中的第三方 GitHub Actions 全部固定到完整 commit SHA。行尾注释记录对应的
主版本；升级 Action 时必须同时核对上游 release 与新的 SHA。

`requirements.lock` 使用精确版本标签描述必须同时支持 `linux/amd64`、
`linux/arm64` 的服务镜像。单个平台的 child manifest digest 不具备跨架构
可移植性，因此不得写入共享锁文件。生产部署按摘要固定时，应使用仓库发布的
OCI index / manifest-list digest。

## 回滚

不得移动或复用精确版本标签。回滚时，将 `DOCKER_IMAGE` 改为已知正常的精确版本
或 OCI 多平台索引摘要，并使用 `--no-build` 重新部署；修复问题后发布新的 Patch
版本。
