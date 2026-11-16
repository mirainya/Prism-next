# URL 转存任务：实施记录

## 当前状态

- 存储端异步任务接口已部署，本次生产配置已启用；代码默认仍为关闭。
- Prism 已接入新任务提交、历史文件查询、后台恢复与处理状态展示。
- 客户端与运行流程使用本地 HTTP 模拟及 sqlmock；Prism 与存储端持久化均已通过隔离 MySQL 8.4 验证。
- **2026-11-07 两端已部署，生产新增三张表并完成健康校验。**
- **原有三笔失败任务尚未恢复或重传。**

## 已实现的约束

1. 每个 attempt/result ordinal 仅保留一个不可变的转存意图。
2. 在原 attempt 上加行锁；先保留待探测身份，再一次性选择方式；探测失败仍保留原身份。
3. 表内保存任务标识、目标目录、原始文件名及源和存储绑定的摘要，不存源地址或凭据。
4. 客户端固定使用绑定的存储端点和 Key，单次请求最长 15 秒。
5. 已选定异步方式后，接口故障或功能关闭不切换同步上传。旧服务仅允许一次同步请求，超时后只查文件。
6. 任务响应必须属于请求的 taskKey；成功提交响应还须核对目标目录和原始文件名。
7. 历史查询仅接受目录和原始文件名完全相同的文件。
8. 重复文件的大小、SHA-256、平台和 MIME 必须一致；缺失或冲突不触发上传。
9. 历史查询最多 10 页、每页 100 条；拒绝总数变化及不完整分页。
10. 错误不包含源地址、Key、签名 URL 或存储响应原文。
11. 首次调用继续等待至原有 5 分钟上限，并为父任务的结果提交预留 30 秒。
12. 后台处理中每 30 秒查询；查询等待不计入 5 次实际失败上限。24 小时后仍无结果则停止自动查询。
13. 异步成功结果和采用的历史文件不因本地数据库写入失败而被删除。
14. 日志将仍在查询的任务显示为“转存处理中”，不计为转存失败。

## 数据库变更

`database/migrations/20261107_110000_add_result_import_jobs.sql`

新增 `gw_result_import_jobs` 和 `gw_delivery_recovery_failures`，分别记录导入身份和真实恢复失败次数。
已通过本机一次性测试库验证，并于 2026-11-07 经确认执行生产迁移；不修改历史迁移或业务记录。
该文件单独加入 Git 忽略例外，历史私有 SQL 仍被忽略。

## 本地验证

```text
go test ./internal/gateway/runtime ./internal/gateway/delivery ./pkg/filestorage ./internal/migrate -count=1
go test ./internal/gateway/repository -run 'TestManagedURLImport|TestDeliveryFailureBudget|TestLegacyURLImport|TestManagedImportMode' -count=1
go test ./internal/service -run 'TestCallResult' -count=1
go test -race ./internal/gateway/runtime ./internal/gateway/delivery ./pkg/filestorage -count=1
go vet ./internal/gateway/runtime ./internal/gateway/delivery ./internal/gateway/repository ./internal/service ./internal/migrate ./pkg/filestorage
git diff --check
```

前端已通过结果文件页面测试及 TypeScript 检查。
回归覆盖响应丢失后只查询原任务、历史查找、并发方式选择、上传前持久化、正常等待不消耗失败预算，以及数据库异常时保留结果。
sqlmock 验证查询顺序及已有任务身份复用，不等同于真实数据库并发验证。
两端新表及并发已在本机 MySQL 8.4.8 验证；真实对象存储及完整进程重启恢复尚未验证。

隔离 MySQL 验证位于 `internal/gateway/repository/result_import_jobs_mysql_test.go`，2026-11-07 执行通过。
仅接受 `PRISM_IMPORT_TEST_DSN` 指向回环地址的 `13316` 端口。
创建全新 `prism_import_validation_20261107` 库；若同名库已存在则终止，不使用或删除它。
已验证迁移重复执行、8 个并发保留、方式选择、同步请求标记、重连后身份保持及独立失败计数；测试库已由测试清理。
同一集成验证的 `-race` 检查亦通过；本次专用 MySQL 进程及数据目录已停止并清理。

2026-11-07 两端验证包构建完成：Prism Linux 二进制与控制台、存储端 Spring Boot JAR。
存储端 58 项现有任务测试通过。验证包基于当前工作区，尚未发布或部署，不用于公开分发。

存储端新增 `UrlImportTaskMySQLTest` 已经明确授权并于 2026-11-07 执行通过，无跳过、失败或错误。
审阅文件为 `.temp/temp_xfs_import_validation_20261107.sql`，仅使用本机 `13316` 专用实例。
已验证真实 MyBatis 映射、迁移重复执行、8 个并发提交与领取、Key 隔离、租约失效及新会话恢复。
测试只加密合成源地址，不下载文件或写入对象存储；测试库已清理，专用 MySQL 进程已停止。
本轮数据目录 `.temp/temp_xfs_import_mysql_20261107` 的递归删除被执行策略拒绝，暂时保留。

## 独立发布准备

2026-11-07 已找到与线上二进制相同的本地生产快照，基于该快照制作独立修复包，并在下述生产验证阶段完成部署。
Prism 只加入本次后端修改和新迁移，原有迁移内容保持不变。
前端以 Git 中已跟踪的源码重建，复核全部 68 个输出的内容与线上一致后，再加入 3 个处理状态相关文件；比较时仅排除构建文件名和 HTML 空白差异。
存储端以当前线上 JAR 为基础替换或新增 18 个相关类，逐项验证其余 323 个条目内容保持不变，未包含已有文件管理改动。
另外准备仅识别新迁移、保留旧业务行为与前端的 Prism 回退程序；未经此处理的旧程序会拒绝较新的迁移记录。

独立包的转存运行流程、客户端、真实失败计数、结果接口、前端测试、类型检查及 Go vet 均通过。
生产快照原有 `TestProviderBillingSecondsMigrationPreservesExistingSources` 在修改前后均失败；这是历史基线测试与该生产快照不一致，本次没有为此改写历史迁移。
发布证据位于 `.temp/temp_managed_import_release_20261107/release-manifest.json`。
生产 SQL 审阅文件为 `.temp/temp_managed_import_production_20261107.sql`；已于 2026-11-07 经明确确认执行。

## 生产验证结果

- 三张新表的列、唯一索引、外键及 Prism 迁移摘要校验通过。
- 两端程序摘要与独立发布包一致，存储配置已启用新接口；Prism 外部配置保持不变。
- Prism 健康检查中的数据库、Redis 和网关均正常；实际返回的前端模块含处理状态展示。
- 配置中的存储入口返回 `schemaVersion=1`，真实任务数据库查询对不存在的测试标识返回 404。
- 存储任务执行器在重启后没有新增运行错误；上传前检查确认没有正在执行的文件上传。
- 未进行真实对象上传测试，也未验证带有进行中导入任务的重启恢复。
- 未重新生成、重新上传或修改原三笔历史交付；远端压缩上传包已清理，回退程序及原配置备份保留。

## 私有存储路径兼容修正

2026-11-07 首次真实调用暴露出成功响应校验遗漏：私有平台可能返回内部对象路径，而新任务客户端要求 `result.url` 必须为完整 HTTP 地址，导致存储已成功但 Prism 报 `managed_copy_import_stop_response`。
已修正为在两个地址字段一致且路径安全时，将内部路径保留为鉴权下载与签名接口使用的存储标识；不将其构造成浏览器 URL。
任务身份、目录、原始文件名、大小、MIME 和摘要仍按原规则校验，不允许不安全路径、非 HTTP 协议或目标不一致的结果。

修正版本 `2.0.20+imports.20261107.1` 已部署，仅更新 Prism，未新增迁移或修改存储服务。
客户端、转存运行流程与交付回归测试及 Go vet 通过。
报告的交付 1660 已通过原任务核对恢复为 ready；实际访问图片返回 200，PNG 校验通过，尺寸 1672×941、2540299 字节。
存储任务仍为同一标识、执行次数 1，未重新生成、重新上传或扣费。原先三笔历史交付仍未恢复。

## 后续阶段

1. 核对并恢复原有失败任务；只采用已确认的原文件，不重复上传或生成。
2. 观察真实导入的完成结果，补充带有进行中任务的重启恢复验证。

内联结果及依赖上游鉴权的下载不改为公开 URL 导入。
