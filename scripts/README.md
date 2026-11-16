# 脚本与私有配置

本目录只提交可公开的通用工具及其测试。

- 真实渠道配置保存在 `configs/video/`，由 Git 和 Docker 忽略。
- 渠道专用维护脚本及其测试保存在 `scripts/private/`，不进入公开仓库或镜像。
- 本地专用 Go 测试使用 `*.private_test.go` 命名；通用公开测试不得依赖这些文件或私有配置。
- 不要使用 `git add -f` 上传私有资料；提交前检查暂存区和文件内容。
- 公开安装包必须从干净的 Git 检出构建，不能复用含私有文件的本地部署产物。

价格页可通过本地 `console/.env.local` 的 `VITE_HIDDEN_SOURCE_LABELS`
设置需隐藏的来源标签（逗号分隔，默认空）。它只影响页面显示；公开名称和
描述仍应在目录配置中维护。该文件及含本地配置的构建产物不得上传。

通用脚本测试：

```powershell
python -m unittest discover -s scripts -p 'test_*.py'
```

本地私有脚本测试：

```powershell
python -m unittest discover -s scripts/private -p 'test_*.py'
```

私有资料在公开历史中已有的副本不会因忽略规则或文件迁移而消失，须另行审计处理。
