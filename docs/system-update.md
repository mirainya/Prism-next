# 运维台在线更新

## 操作

管理员进入「运维台 → 系统更新」：检查版本、下载校验、预览安装计划、确认安装。
待执行迁移会显示完整 SQL；确认绑定版本、归档 SHA256 和迁移计划摘要。
安装在独立服务中运行，页面断开不取消安装；重新登录仍可读取更新状态。

## 更新源

GitHub 是固定来源。额外 HTTPS 镜像通过页面配置，保存在 `prism_update_sources`。
配置支持启停、优先级和版本检查，保存时检测并发修改。仓库不内置私人镜像域名。
检查版本按优先级读取来源；获得有效且符合发布通道的清单后立即返回，不再直连 GitHub 比较版本。仅在当前来源失败或清单无效时尝试下一来源。下载包失败仍可尝试其他来源，但版本、大小和 SHA256 必须与已选清单一致。
CLI 检查和下载只使用 GitHub，不读取数据库来源。

## 首次安装更新服务

适用 Linux AMD64、systemd、非 root 运行 Prism 的部署。容器更新由容器部署工具处理。
此步骤由服务器管理员执行一次，不能由网页请求决定服务名称或执行路径。

1. 安装支持本功能的 Prism 版本，执行该版本的已审核数据库迁移。
2. 将 Release 内 `prism-updater` 安装到 `/usr/local/libexec/prism-updater`，所有者 root、权限 0755。
3. 参考 `configs/updater.example.json` 创建 `/etc/prism/updater.json`，所有者 root、权限 0644；填写当前安装目录、服务名称、运行用户与本机健康检查 URL。
4. 将 `scripts/prism-update.service` 安装到 `/etc/systemd/system/prism-update.service`；添加与 Prism 服务相同的网关运行密钥环境配置，不把密钥写入仓库。
5. 使用 `visudo` 审核 `scripts/prism-update.sudoers` 并安装到 `/etc/sudoers.d/prism-update`，权限 0440；用户名必须与 Prism 服务一致。仅允许启动固定更新服务。
6. 执行 `systemctl daemon-reload`。Prism 运行用户需要能写安装目录下的 `.update`；其余服务文件和更新器保持 root 所有。

服务器须有 `/usr/bin/systemctl`、`/usr/sbin/runuser`、`/usr/bin/sudo` 与 `/usr/bin/mysqldump`。
候选版本须支持 `prism migrate plan`。更早的 Release 不接受网页自动安装。

## 安装过程与失败

更新服务重新验证归档，拒绝路径越界、链接、重复文件和过大解压；只提取 Prism 程序，不覆盖配置。
候选程序以 Prism 运行用户核对版本与迁移计划。随后备份程序和配置、停止服务；有迁移时先完整备份数据库，再执行已确认迁移。
通过原子替换安装程序，启动服务，检查数据库、Redis 与网关健康；全部通过才标记完成。

备份和执行记录位于 `/var/lib/prism-updater/job-*`，仅 root 可读取数据库与配置备份。
不涉及迁移的失败可恢复旧程序。迁移已开始后不会仅恢复旧程序并声称成功，而是停止服务并标记 `recovery_required`。
安装前检查失败且尚未停止服务时标记 `failed`，保留当前服务运行，允许修正配置后重新下载、预览和安装。
此时管理员检查 `journalctl -u prism-update.service`，依据备份恢复数据库及匹配程序；不得在不兼容结构上直接启动旧程序。
数据库备份不包含其他应用的写入隔离保证；共享数据库部署必须在更新期间停止其他写入方。

状态文件位于安装目录 `.update/install-status.json`。它记录安装结果，不含上游密钥或镜像地址。
