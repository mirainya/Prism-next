# 数据库协议接入与维护

模型发现、价格字段读取和可用性读取均使用通用执行器。来源地址、认证步骤、接口路径、响应条件及 JSON 字段映射存放在数据库；公共代码不包含上游专用 HTTP 协议。生产转换配置和快照留在忽略目录 `.temp/`，不得提交。

## 配置归属

| 数据 | 存储位置 | 维护入口 |
|---|---|---|
| 不可变的协议版本 | `gw_protocol_definitions` | 网关管理 → 可用性来源 → 协议配置 |
| 可用性来源及读取凭据 | `gw_availability_sources` | 网关管理 → 可用性来源 |
| 模型/价格发现来源 | `gw_catalog_source_profiles` | 管理 API |
| 当前观测缓存 | `gw_upstream_availability` | Worker 自动替换 |

来源分别使用 `declarative_catalog_v1`、`declarative_availability_v1`，通过 `protocol_definition_id` 绑定启用且类型匹配的协议。目录读取与可用性读取继续使用独立的凭据用途授权；协议 JSON 使用 `${username}`、`${password}` 等变量引用来源凭据，不写账号密码。

## 支持的协议字段

- `schema_version: 1`；`kind` 为 `catalog_discovery` 或 `availability`。
- `auth.mode` 支持 `none`、`bearer`、`json_login`；登录必须是第一步。
- `steps` 包含名称、方法、路径、查询参数、请求头、JSON 请求体、保存字段及响应 `checks`，最多 16 步。
- `checks` 使用 `path`、标量 `equals`、可选的 `optional`；失败即停止，不继续读取后续步骤。
- 模型映射保留名称、描述、标签、分组、端点类型、供应商 ID、计费方式、精确单价、倍率、归属及价格版本。
- 可用性映射支持成功率、样本标记、平均生成时间、渠道状态、预计耗时、统计窗口和观测时间。
- `buckets_path` 可保存分段状态数组，包含 `start`、`end`、`has_data`、`has_failure`、`success_rate` 和 `label`。无样本段保留为空状态，不参与成功率判断。
- 公开状态的 `average_completion_seconds` 是上游平均耗时，`buckets` 是上游时间段记录；与本站成功率独立。启用分段采集前先应用 `20261013_160000_availability_buckets.sql`，再创建带 `buckets_path: "buckets"` 的新协议版本并切换来源。
- 路径默认读取当前行；`$.` 读取响应根；`$parent.` 读取父行；`[]` 遍历数组。
- `children` 支持一层子渠道；`aggregate_children_eta` 取子渠道最大预计耗时；`estimated_completion_match` 筛选有效耗时类型。
- `deduplicate: latest` 保留最新的有样本观测；`worst` 合并最差状态和最长耗时。
- `filter_selected_group` 在目录解析前筛选来源分组，避免不同分类同名项目冲突。

成功率缺少有效样本时不生成成功率。渠道只提供状态或耗时，也能独立展示；字段缺失不表示零或不支持。价格解析保留精度；价格发布与自动读取仍是独立操作，不自动修改在售价格。

## 更新和停用

修改协议先创建新版本，再修改来源绑定。历史定义与发现快照不覆盖。可用性来源变更、停用或重新启用时，递增 `config_version` 并清除旧缓存；在途旧版本结果不能重新写入。重新启用后等待下一轮采集，避免显示停用前的数据。停用协议也递增关联监控来源版本并清除缓存，公开查询立即排除停用协议。

采集密钥完整时，模型发现与监控循环会保持运行；只配置监控来源也能独立采集。进程启动后新增或启用来源，由后续轮询读取，监控周期为四分钟。空配置且缺少采集密钥时，仍允许控制台启动；补充环境变量中的密钥后需要重启程序。

目录来源切换只允许在没有未完成发现任务时执行；来源版本随之递增。已完成快照保留原合同和事实；后续发现应使用新的目录草稿。

停止接入时先停用来源，再停用未被其他来源使用的协议版本。读取凭据及用途授权只在确认没有其他引用后删除。共用 Worker、公开字段和历史快照无需随上游退出而改代码。

已应用的历史迁移保持原文，因此其中可能保留旧合同或上游名称。本次只移除可执行的专用协议，不重写 Git 历史。

## 管理 API

全部需要管理员认证，不属于 `/v1/models`。

| 方法 | 路径（前缀 `/api/admin/unified-gateway`） | 作用 |
|---|---|---|
| GET / POST | `/protocol-definitions` | 列表 / 创建版本 |
| GET | `/protocol-definitions/:definition_id` | 读取完整配置 |
| POST | `/protocol-definitions/:definition_id/status` | 启停，提交 `status` 与 `expected_version` |
| PUT | `/availability-sources/:source_id` | 更新连接、合同及版本绑定 |
| PUT | `/catalog-sources/:source_id/protocol` | 切换目录协议，提交 `contract_code`、`protocol_definition_id`、`expected_version` |

协议创建提交 `protocol_code`、`protocol_version`、`protocol_kind`、`display_name`、`definition`。服务器校验 JSON 并计算摘要；内容修改必须递增 `protocol_version`。

## 发布验证

离线快照比对覆盖完整价格字段、分类统计元数据、样本过滤、子渠道状态与预计耗时。协议条件失败、登录 ID 无效、JSON 尾部多余内容、凭据特殊字符、重复模型和来源版本冲突均有回归测试。

生产转换须先备份来源和协议配置，停止采集进程，执行已确认的结构迁移与来源转换，再启动程序。上线后核对两类来源绑定、一次完整监控周期和公开接口；不创建收费生成任务。
