# 发布与标签管理

`VERSION` 是项目正式版本的唯一来源。普通代码提交只更新开发镜像；只有 `main` 或 `master` 上包含 `VERSION` 变更的提交才会创建正式发布。

## 标签规则

| 场景 | 输入版本 | 容器标签 |
|---|---|---|
| 普通分支构建 | 无版本变更 | `edge`、`sha-<commit>` |
| 稳定版 | `1.2.3` | `v1.2.3`、`1.2.3`、`1.2`、`1`、`latest`、`sha-<commit>` |
| 预发布版 | `1.3.0-rc.1` | `v1.3.0-rc.1`、`1.3.0-rc.1`、`sha-<commit>` |

精确版本标签 `vX.Y.Z`、`X.Y.Z` 及其预发布形式不可覆盖。`latest`、`X` 和 `X.Y` 是可移动别名，稳定版发布时指向该兼容范围内的最新版本。

## 发布流程

1. 确认 `main` 的前后端检查通过。
2. 按 SemVer 选择新版本，版本号不能与历史版本重复。
3. 单独提交 `VERSION` 变更，提交信息使用 `chore: release vX.Y.Z`。
4. 推送后等待 `CI and Docker` 完成。
5. 检查 GitHub Release、Docker Hub 和 GHCR 中的版本标签与多架构清单。

示例：

```bash
printf '1.2.3\n' > VERSION
git add VERSION
git commit -m "chore: release v1.2.3"
git push origin HEAD:main
```

## 版本选择

- Patch：兼容的缺陷修复，例如 `1.2.3` 到 `1.2.4`。
- Minor：向后兼容的新功能，例如 `1.2.3` 到 `1.3.0`。
- Major：不兼容变更，例如 `1.2.3` 到 `2.0.0`。
- Pre-release：候选版本，例如 `2.0.0-rc.1`，不会更新稳定别名。

## 回滚

不要移动或复用精确版本标签。回滚时将部署配置改回已知正常的精确版本或 digest；修复后发布新的 Patch 版本。
