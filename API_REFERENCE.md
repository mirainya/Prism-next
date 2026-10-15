# Prism V2 API Reference

本文只记录当前代码注册的 HTTP 路由。公开 AI API 位于 `/v1`；控制台 API 位于 `/api`；内部上游回调位于 `/internal/gateway`。

实际可用模型与价格取决于当前实例已激活的目录发布版。

模型列表、详情、价格与运行状态见 [模型接口说明](docs/MODELS_API_SPLIT.md)；下游视频接入见 [Prism 接口指南](docs/PRISM_API_GUIDE_20261009.md)。

## 通用约定

### 基础地址

```text
https://provider3.example.com
```

### 认证

| 路由 | 认证 |
|---|---|
| `/v1/*` | `Authorization: Bearer sk-prism-...` 或 `x-api-key` |
| `/api/*`（除 auth/public） | `Authorization: Bearer <JWT>` |
| `/api/admin/*` | JWT 且用户角色为 admin |
| `/internal/gateway/*` | 执行级回调令牌 |

所有响应带 `X-Request-ID`。建立统一 Call 后，调用响应还带 `X-Prism-Call-ID`。

### 响应格式

Chat、Messages、Responses、Files、Models 和 Images 使用各自兼容协议。Videos 与控制台 API 使用 Prism 包装：

```json
{"code":0,"message":"success","data":{}}
```

失败示例：

```json
{"code":400,"message":"invalid parameters"}
```

## Models API

`GET /v1/models` 返回轻量摘要；`GET /v1/models/:code` 返回单个模型的详细能力与价格；`GET /v1/models/availability` 独立批量查询动态状态。视频详情的 `video_options` 提供分辨率、默认值和其他能力；各类模型详情统一通过 `pricing.variants` 提供价格组件和适用操作、档位与约束。具体视频请求报价使用 `POST /v1/videos/estimate`。缺失字段表示未知，不代表不支持。

## 对话 API

### POST /v1/chat/completions

OpenAI Chat Completions 兼容接口，支持非流式和 SSE。主要字段：`model`、`messages`、`stream`、`tools`、`tool_choice`、`response_format`、采样参数及输出上限。

```bash
curl "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"model-code","messages":[{"role":"user","content":"你好"}]}'
```

### POST /v1/messages

Anthropic Messages 兼容接口，支持非流式和 SSE。使用 `x-api-key` 时仍由 Prism Token 鉴权。

```bash
curl "$BASE_URL/v1/messages" \
  -H "x-api-key: $PRISM_TOKEN" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"model-code","max_tokens":1024,"messages":[{"role":"user","content":"你好"}]}'
```

### POST /v1/responses

OpenAI Responses 兼容接口，支持 `stream`、`store`、`background`、`previous_response_id` 和 `Idempotency-Key`。

```bash
curl "$BASE_URL/v1/responses" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-1" \
  -d '{"model":"model-code","input":"你好"}'
```

Responses 资源路由：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/responses/:id` | 查询 Response |
| DELETE | `/v1/responses/:id` | 删除 Response |
| POST | `/v1/responses/:id/cancel` | 取消后台 Response |
| GET | `/v1/responses/:id/input_items` | 输入项分页 |

`cancel` 仅属于 Responses 资源，不代表图片或视频支持取消。

### Conversation 关联

Chat、Messages 和 Responses 接受 `X-Prism-Conversation-ID` 续接当前 Token 的 Conversation。Chat 还接受请求体 `conversation_id`，两处同时提供时必须一致。调用的 canonical 输入和已产生输出会投影为 Conversation；`store: false` 不关闭该投影。

## Files API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/v1/files` | `multipart/form-data` 上传 |
| GET | `/v1/files` | 当前 Token 文件列表 |
| GET | `/v1/files/:id` | 文件元数据 |
| GET | `/v1/files/:id/content` | 文件内容 |
| DELETE | `/v1/files/:id` | 删除文件 |

新上传的文件使用当前 Token 绑定的 XFileStorage 地址与 Key；未绑定时使用系统级配置。读取和删除使用上传时保存的配置与对象定位符。

```bash
curl "$BASE_URL/v1/files" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -F "purpose=vision" \
  -F "file=@./image.png"
```

文件只能由创建它的 Token 查询、下载、引用或删除。

## Images API

### POST /v1/images/generations

OpenAI 风格 JSON 接口。必填字段为 `model` 和 `prompt`。支持 `n`、`size`、`aspect_ratio`、`quality`、`response_format`、`output_format`、`stream` 等参数。该路径只执行图片生成；图片编辑必须调用 `/v1/images/edits`。

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"image-model","prompt":"糖果色城市","size":"1024x1024","response_format":"url"}'
```

### POST /v1/images/edits

图片编辑接口支持两种请求格式：JSON 请求使用 `image_urls` 传入 URL、Base64 或 Data URL；`multipart/form-data` 请求使用可重复的 `image` 文件字段，并可带一张 `mask`。输入图片会转为统一媒体资产引用，不把 Base64 请求作为长期任务参数保存。

图片结果返回 OpenAI `created` 与 `data` 结构；流式请求返回 SSE。完整示例与文件限制见 [下游生图接入指南](docs/IMAGE_API_GUIDE.md)。

### 图片异步任务

任务型图片线路可使用异步接口，提交后立即返回 `202`：

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/v1/images/generations/async` | 异步图片生成 |
| POST | `/v1/images/edits/async` | 异步图片编辑 |
| GET | `/v1/images/tasks/:id` | 查询当前 Token 创建的图片任务 |

提交参数与对应同步接口相同，但 `stream` 必须省略或为 `false`，并且必须省略 `partial_images`。编辑接口仍同时支持 JSON `image_urls` 和 multipart `image` 文件。成功提交响应包含任务编号 `X-Prism-Call-ID`、`Location`、`Retry-After` 和 `data.location`；查询状态为 `queued`、`processing`、`completed` 或 `failed`。查询未结束任务和业务失败任务均返回 HTTP `200`，分别通过 `data.status` 和 `data.error` 表达状态；完成时 `data.result` 为 OpenAI 图片结果。任务只能由创建它的同一个 Prism Token 查询。线路策略允许时，异步生成可用 `Idempotency-Key` 在 `24` 小时内去重；异步编辑会先导入素材，不保证重复提交复用原任务。完整请求、轮询代码与响应示例见 [下游生图接入指南](docs/IMAGE_API_GUIDE.md)。

## Videos API

### POST /v1/videos/generations

创建视频。请求经过统一目录选路，固定发布版、SKU、Offering、成本方案、Transport 与凭据版本，并在事务中创建 Call、Attempt、异步执行和 SQL Outbox。

```json
{
  "model": "video-model",
  "prompt": "海边日落",
  "duration": 5,
  "resolution": "1080p",
  "aspect_ratio": "16:9",
  "service_tier": "standard",
  "callback_url": "https://example.com/prism-callback"
}
```

已有公网媒体地址时，在 `references[]` 直接填写 `url`，无需调用素材接口。例如公网首帧图片：

```bash
curl "$BASE_URL/v1/videos/generations" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"video-model","prompt":"镜头缓慢推进","references":[{"type":"image","role":"first_frame","url":"https://example.com/frame.png"}]}'
```

`references[].type` 可为 `image`、`video`、`audio`，对应角色如 `first_frame`、`reference_video`、`reference_audio`；是否允许该角色取决于模型能力。每项引用必须且只能填写 `url` 或 `asset_id` 之一。Prism 校验直传 URL，不在提交时下载或转存；上游仍需能访问该地址。

可使用 `Idempotency-Key`；目录策略可规定 required、optional 或 forbidden。成功创建：

```json
{"code":0,"message":"success","data":{"id":"uuid","status":"queued","service_tier":"standard"}}
```

### POST /v1/videos/estimate

使用当前活动目录估算费用，返回字符串金额及币种。估算不创建任务。请求指定分辨率时，`estimated_cost` 按本次请求的分辨率、时长和其他已知参数计算；未指定分辨率或缺少其他计价事实时，返回最高预授权估价。`estimate_basis` 为 `request_estimate` 时表示请求估价，为 `reservation_maximum` 时表示最高预授权估价。`reservation_amount` 是创建任务时实际预授权的最高金额，不是最终扣款；最终费用按上游结果结算。

模型列表、价格列表及估价的查价金额乘以 `server.price_display_multiplier`，默认 `"1.8"`；仅调整返回价格，数据库原价、实际预授权和扣费不变。`reservation_amount` 保留实际金额，不乘系数。

页面显示价格及估价总额 `estimated_cost`、`base_cost` 保留两位小数，直接截断、不四舍五入。API 结构化单价和公式保留精度；`reservation_amount` 不截断。不要使用截断后的页面单价推算请求估价。

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "estimated_cost": "9.00",
    "base_cost": "9.00",
    "reservation_amount": "15",
    "markup_ratio": "0",
    "pricing_mode": "catalog",
    "estimate_basis": "request_estimate",
    "is_upper_bound": false,
    "service_tier": "standard",
    "currency": "CNY"
  }
}
```

### 视频查询

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/videos/generations` | 当前 Token 的视频分页列表 |
| GET | `/v1/videos/generations/:id` | 视频详情与可用结果 |
| GET | `/v1/videos/generations/:id/queue` | 单任务异步状态 |
| GET | `/v1/videos/queue` | 最多 100 个活动任务的轻量投影 |

视频没有取消路由。任务进入提交未知状态时由恢复 Worker 查询或进入人工核验，不会自动重复提交。

### 视频素材

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/v1/videos/assets` | 上传本地文件，或按需下载并转存 URL |
| POST | `/v1/videos/uploads` | 与素材创建相同的兼容入口 |
| GET | `/v1/videos/assets/:asset_id` | 素材详情 |
| DELETE | `/v1/videos/assets/:asset_id` | 将未占用素材标为过期 |

本地文件使用 `multipart/form-data` 上传：

```bash
curl "$BASE_URL/v1/videos/assets" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -F "kind=image" \
  -F "file=@./frame.png"
```

取得响应中的 `data.id` 后，在视频请求的 `references[]` 中填入 `{"type":"image","role":"first_frame","asset_id":"<data.id>"}`。`application/json` 的 URL 登记会下载源文件并重新上传，不是公网 URL 生成视频的必经步骤。素材按用户及 Token 隔离；已被任务引用时不能删除。新建素材优先使用当前 Token 绑定的 XFileStorage 地址与 Key，未绑定时使用系统级配置；创建时固定的配置供后续读取和清理使用。

## Models API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/models` | 轻量摘要：名称、类型、介绍、功能标签、协议和端点 |
| GET | `/v1/models/:code` | 单个模型详细能力及统一结构的价格 |
| GET | `/v1/models/availability` | 批量动态状态：成功率、渠道状态、耗时和时间段记录 |

模型列表来自已激活的统一目录，覆盖语言、图片、视频等全部可调用模型。列表不查询价格和动态观测；详情不包含 `availability`。动态状态接口返回 `{ "object": "list", "data": [{ "id": "model-code", "availability": {} }] }`，按 `id` 关联模型。没有观测的模型仅返回 `id`；查询失败返回 503，不伪装为零成功率。`availability` 可包含 `success_rate`、`channel_status`、`estimated_completion_ms`、`average_completion_seconds`、`buckets` 与观测时间。字段缺失表示未知。
各类模型详情统一返回 `pricing.currency` 和 `pricing.variants[]`；每个价格方案的 `operation`、`service_tiers`、`constraints` 分别说明操作、档位和适用约束，`components[]` 提供单价或公式及计费单位。价格是 Prism 显示价格，不公开内部 SKU 名或上游成本。
LLM 可选能力位于 `llm_options`：`context_window`、`supports_streaming`、`supports_tools`、`supports_vision`、`supports_audio`、`supports_reasoning`。只返回所有可执行线路共同确认的字段；缺失表示未知，明确的 `false` 会保留。未知配置、上游名称、地址、Key 和映射不会公开。

## 不存在的旧路由

以下路由未注册，返回 404：

- `/v1/channels`
- `/v1/capabilities`
- `/v1/capabilities/:capability`
- `/v1/tasks/:task_no`
- `/v1/tasks/:task_no/cancel`
- `/v1/videos/generations/:id/cancel`

## 控制台 API 概览

公开认证：`POST /api/auth/register`、`POST /api/auth/login`、`POST /api/auth/logout`、`GET /api/public/pricing`。

登录用户可管理自身资料和 Token，查看仪表盘、统一调用、只读任务投影、观测日志、Conversation、文档及 Playground。

Token 文件存储接口：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/tokens/file-storage-endpoints` | 获取允许绑定的 XFileStorage 服务地址 |
| PUT | `/api/tokens/:id/file-storage` | 验证并绑定服务地址与 Key，请求体为 `{"base_url":"https://xfs.example.com","api_key":"xfs_..."}` |
| DELETE | `/api/tokens/:id/file-storage` | 清空该 Token 的 XFileStorage 绑定；后续图片和视频结果直接使用上游地址 |

Token 列表与详情返回是否已配置、服务地址及 Key 尾号，不返回完整 XFileStorage Key。绑定服务地址只能从服务端允许的地址中选择；新调用会固定当时的地址和 Key。

管理员统一网关路由以 `/api/admin/unified-gateway/*` 为前缀，覆盖：

- overview、channels、pools、credentials；
- catalog、catalog-sources、discoveries、products、SKUs、rates；
- currencies、rate-evidence；
- `protocol-definitions`：数据库协议定义的查询、创建和启停；catalog/availability source 创建时可绑定 `protocol_definition_id`；
- deployments、members、catalog/crypto readiness；
- `GET /api/admin/unified-gateway/deployments/runtime-identity`：读取当前实例与二进制摘要；
- `POST /api/admin/unified-gateway/deployments/:id/prove-current`：校验当前目录、Adapter 与密钥并提交当前实例证明；
- calls、attempts、request logs 与按权限读取的 payload；
- offering credential validations。

管理员视频任务为只读投影：`GET /api/admin/video/tasks`、`GET /api/admin/video/tasks/:id`、`GET /api/admin/video/stats`。不存在旧能力渠道或视频渠道写接口。

`POST /api/admin/unified-gateway/catalog-changes/model-onboard` 原子创建对外模型及其线路、调用费率。`sell_rate` 必填，`cost_rate` 可省略；省略表示成本未知，不生成成本费率。`additional_sell_rates`、`additional_cost_rates` 可添加独立分项；提供后者必须同时提供 `cost_rate`。LLM 支持输入、输出、缓存读取、缓存写入定价；缓存单独计价时，普通输入使用排除相应缓存的计量来源，避免重复收费。详见[模型配置手册](docs/MODEL_ONBOARDING.md)。

## 内部回调

`POST /internal/gateway/callback` 接收上游回调。路径不再接受客户端提供的 `scope`；事件作用域由服务端固定为 `unified`。请求必须带 `Content-Type: application/json`（可带标准参数）和 `X-Gateway-Callback-Token`，可带 `X-Gateway-Event-ID`。正文上限为 1 MiB，入口按客户端地址限速并限制并发，只持久化短期回调证据后返回 `202`，业务状态由 SQL Worker 消费后更新。认证失败不会创建 Receipt；Receipt 到期或处理完成后会清除加密正文。
