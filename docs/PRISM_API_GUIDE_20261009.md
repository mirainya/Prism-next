# Prism API 接口文档

## 1. 基础信息

| 项目 | 内容 |
|---|---|
| Base URL | `https://provider3.example.com` |
| 认证 | `Authorization: Bearer sk-prism-...` |
| 模型列表 | `GET /v1/models` |
| 模型详情与价格 | `GET /v1/models/:code` |
| 模型运行状态 | `GET /v1/models/availability` |
| 视频创建 | `POST /v1/videos/generations` |
| 视频估价 | `POST /v1/videos/estimate` |

查价金额按 `server.price_display_multiplier` 显示，默认 `"1.8"`。仅影响价格列表、模型介绍和视频估价；数据库原价、实际预授权及扣费不变。`reservation_amount` 仍是实际预授权额，不能按显示倍率推算实际扣费。

页面价格和估价总额保留两位小数，直接截断；API 的精确单价、公式及实际预授权金额保留精度。请求总额先按精确价格计算，再截断，不用页面单价相乘。

所有公开请求只使用 Prism 模型名和 Prism API Key。不要传渠道名、分组名、上游地址或上游模型名。

## 2. 查询模型

```bash
curl "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

响应为 OpenAI 风格的轻量模型列表，不包含详细能力、价格或运行状态：

```json
{
  "object": "list",
  "data": [
    {
      "id": "model-code",
      "object": "model",
      "owned_by": "prism",
      "name": "model-code",
      "model_code": "model-code",
      "description": "模型说明",
      "type": "video",
      "features": ["text-to-video", "image-to-video"],
      "supported_operations": ["videos.generate"],
      "supported_endpoints": ["/v1/videos/generations"]
    }
  ]
}
```

选中模型后查询详情及价格：

```bash
curl "$BASE_URL/v1/models/model-code" \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

详情保留摘要字段，额外返回已声明的 `llm_options`、`video_options` 及 `pricing`。各类模型使用同一价格结构；例如视频详情包含：

```json
{
  "id": "model-code",
  "object": "model",
  "video_options": {
    "duration_min": 4,
    "duration_max": 30,
    "resolutions": ["720p"],
    "max_images": 10,
    "max_videos": 5,
    "max_audios": 5
  },
  "pricing": {
    "currency": "CREDIT",
    "currency_version": 1,
    "fraction_digits": 8,
    "rounding_mode": "half_even",
    "variants": [{
      "operation": "videos.generate",
      "service_tiers": ["standard"],
      "constraints": {"resolutions": ["720p"]},
      "components": [{
        "component": "duration",
        "unit": "second",
        "unit_price": "0.36",
        "max_quantity": "30",
        "quantity_source": "request.seconds",
        "charge_event": "call.succeeded",
        "mode": "flat"
      }]
    }]
  }
}
```

示例仅展示详情的额外字段。动态状态使用独立批量接口：

```bash
curl "$BASE_URL/v1/models/availability" \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

```json
{
  "object": "list",
  "data": [
    {"id": "model-code", "availability": {
      "success_rate": "95.50",
      "source": "upstream",
      "window_minutes": 120,
      "channel_status": "available",
      "estimated_completion_ms": 120000,
      "observed_at": "2026-10-13T09:00:00Z"
    }},
    {"id": "model-without-observations"}
  ]
}
```

按 `id` 与模型列表关联，状态可独立刷新。列表和详情不再含 `availability`。没有观测时省略该字段；查询失败返回 503。能力或观测缺失表示未知，不表示不支持或数值为零。

### 2.1 价格和可用性字段

| 字段 | 含义 |
|---|---|
| `pricing.currency` | Prism 结算币种；不要固定假设为人民币 |
| `pricing.variants[].operation` | 对应操作，如 `chat.completions`、`images.generate`、`images.edit`、`videos.generate` |
| `pricing.variants[].constraints` | 该价格适用的分辨率、时长和素材限制 |
| `pricing.variants[].service_tiers` | 适用服务档位 |
| `components[].unit_price` / `unit` | 应用显示倍率后的单价及计费单位；金额为字符串 |
| `components[].mode` | `flat` 为固定单价，`expression` 为参数计价 |
| `components[].formula` | 参数计价公式；请求报价使用估价接口 |
| `availability.success_rate` | 0-100 的百分数，例如 `"95.50"`；非 0-1 小数 |
| `availability.samples` / `window_minutes` | 样本数及观测窗口；可能缺省 |
| `availability.channel_status` | 上游报告的当前渠道状态；非统一枚举 |
| `availability.estimated_completion_ms` | 预计生成时间，单位毫秒；不是承诺完成时间 |
| `availability.average_completion_seconds` | 观测到的平均完成时间，单位秒 |
| `availability.buckets` | 时间段记录，包含起止时间、是否有样本、是否有失败和成功率；按观测实际提供 |
| `availability.observed_at` | 观测时间；渠道状态另有 `channel_observed_at` 时以该时间为准 |

以上 JSON 仅示意字段组合，不代表某个模型必定同时提供成功率、渠道状态和预计生成时间。单分辨率和多分辨率模型都使用同一结构。

## 3. 视频请求

### Wan3 已启用型号

2026-10-09 已上架，调用时直接使用下表模型名，无渠道或分组前缀。

| 模型名 | 分辨率 | 时长 | 图片 / 视频 / 音频上限 | 当前销售单价 |
|---|---|---|---|---|
| `wan3.0-720p` | `720p` | 4-30 秒 | 10 / 5 / 5 | 0.36 CREDIT/秒 |
| `wan3.0-1080p` | `1080p` | 4-30 秒 | 10 / 5 / 5 | 0.576 CREDIT/秒 |

分辨率省略时使用该模型的固定分辨率。单价以 `/v1/models/:code` 当前返回为准；CREDIT 为本实例的结算单位。

### 3.1 创建

```bash
curl "$BASE_URL/v1/videos/generations" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wan3.0-720p",
    "prompt": "海边日落，镜头缓慢推进",
    "duration": 6,
    "resolution": "720p",
    "service_tier": "standard"
  }'
```

请求成功返回：

```json
{"code":0,"message":"success","data":{"id":"uuid","status":"queued","service_tier":"standard"}}
```

### 3.2 参考素材

```json
{
  "model": "wan3.0-720p",
  "prompt": "保持主体一致并生成自然动作",
  "duration": 6,
  "references": [
    {"type": "image", "role": "reference_image", "url": "https://example.com/a.png"},
    {"type": "audio", "role": "reference_audio", "url": "https://example.com/a.mp3"}
  ]
}
```

每项引用只能填写 `url` 或 `asset_id` 其中一个。是否允许某种素材以及数量上限，以模型的 `video_options` 为准。

### 3.3 估价

```bash
curl "$BASE_URL/v1/videos/estimate" \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"wan3.0-720p","prompt":"海边日落","duration":6,"resolution":"720p"}'
```

`estimated_cost` 是本次请求估价；省略 `resolution` 或缺少计价事实时会返回最高预授权估价，并通过 `estimate_basis` 和 `is_upper_bound` 标明。即使是固定分辨率型号，估价请求也应显式传入该分辨率。

`reservation_amount` 是创建任务时预授权的最高金额，不是报价或最终扣款。估价不创建视频任务，也不预授权；创建任务后按固定的价格规则结算。完整字段见主参考文档。

## 4. 查询任务

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/videos/generations` | 当前 Token 的视频列表 |
| GET | `/v1/videos/generations/:id` | 视频详情和结果 |
| GET | `/v1/videos/generations/:id/queue` | 单任务队列状态 |
| GET | `/v1/videos/queue` | 活动任务摘要 |

任务状态为 `queued`、`submitted`、`tracking`、`completed`、`failed` 或 `submission_unknown`。完成结果位于 `data.result.video_url`。

## 5. 素材管理

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/v1/videos/assets` | 上传或登记视频素材 |
| GET | `/v1/videos/assets/:asset_id` | 查看素材 |
| DELETE | `/v1/videos/assets/:asset_id` | 删除未使用素材 |

创建素材后，在视频请求的 `references[]` 中使用 `asset_id`。

## 6. 其他兼容接口

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/v1/chat/completions` | OpenAI Chat 兼容 |
| POST | `/v1/messages` | Anthropic Messages 兼容 |
| POST | `/v1/responses` | OpenAI Responses 兼容 |
| POST | `/v1/images/generations` | 图片生成 |
| POST | `/v1/images/edits` | 图片编辑 |

图片和语言模型是否可用，以 `/v1/models` 当前结果为准。
