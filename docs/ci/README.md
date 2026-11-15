# CI 工作流

`build.yml` 是 GitHub Actions 工作流，用于在每次推送时构建后端与前端。

它**没有**放在 `.github/workflows/` 下，因为把该文件推送到 GitHub 需要 Personal
Access Token 具备 `workflow` scope。若你使用只勾选了 `repo` 的经典 token，推送会被拒绝：

```
refusing to allow a Personal Access Token to create or update workflow
`.github/workflows/build.yml` without `workflow` scope
```

## 启用方式

任选其一：

1. 重新签发 token 并勾选 `workflow`，然后执行：

   ```bash
   mkdir -p .github/workflows
   git mv docs/ci/build.yml .github/workflows/build.yml
   git commit -m "ci: enable build workflow"
   git push
   ```

2. 直接在 GitHub 网页端新建 `.github/workflows/build.yml`，把本文件内容粘贴进去。

在启用之前，仓库可以正常浏览与克隆，但不会有自动构建。
