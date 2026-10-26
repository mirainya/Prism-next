# Prism 模型接口说明

本文说明当前版本拆分后的三个模型查询接口。所有接口都使用 Prism Token，不暴露上游地址、渠道名称、Key 或内部 SKU。

## 接口概览

| 接口 | 用途 | 返回内容 |
|---|---|---|
| `GET /v1/models` | 选择模型 | 轻量摘要，不含价格和动态状态 |
| `GET /v1/models/:code` | 查看模型详情、展示价格 | 能力参数、协议、操作、价格 |
| `GET /v1/models/availability` | 刷新运行状态 | 成功率、渠道状态、耗时和时间段记录 |

请求头：

```text
Authorization: Bearer <PRISM_TOKEN>
```

## 1. 模型列表

```bash
curl "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

列表项只包含摘要字段：`id`、`name`、`description`、`type`、`types`、`features`、`native_transports`、`supported_operations` 和 `supported_endpoints`。

列表适合启动时加载和搜索模型。不要从模型名称猜测能力，也不要在列表页自行拼接价格。

## 2. 模型详情与价格

```bash
curl "$BASE_URL/v1/models/<MODEL_CODE>" \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

详情保留列表摘要，并按模型类型提供以下字段：

- LLM：`llm_options`，包含已确认的上下文窗口、流式、工具、视觉、音频和推理能力。
- 图片：通过 `supported_operations` 区分 `images.generate`、`images.edit` 等操作。
- 视频：`video_options` 提供任务类型、素材上限、时长、分辨率和比例等已声明限制。
- 所有类型：`pricing` 使用统一结构。

价格示例：

```json
{
  "pricing": {
    "currency": "CREDIT",
    "currency_version": 1,
    "fraction_digits": 8,
    "rounding_mode": "half_even",
    "variants": [
      {
        "operation": "chat.completions",
        "service_tiers": ["standard"],
        "components": [
          {
            "component": "input",
            "unit": "token",
            "unit_price": "0.0000054",
            "max_quantity": "1000000",
            "quantity_source": "usage.input_tokens",
            "charge_event": "call.succeeded",
            "mode": "flat"
          }
        ]
      }
    ]
  }
}
```

`variants[].operation` 是计费对应的公开操作；`service_tiers` 和 `constraints` 是适用条件；`components` 可以是固定单价，也可以使用 `mode: "expression"` 与 `formula` 表示参数计价。金额和公式按当前实例的显示价格返回，实际请求报价仍以对应业务接口为准。

## 3. 运行状态

```bash
curl "$BASE_URL/v1/models/availability" \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

返回：

```json
{
  "object": "list",
  "data": [
    {
      "id": "model-code",
      "availability": {
        "success_rate": "95.50",
        "source": "upstream",
        "window_minutes": 120,
        "channel_status": "available",
        "estimated_completion_ms": 120000,
        "average_completion_seconds": "18.4",
        "observed_at": "2026-10-13T09:00:00Z"
      }
    },
    {"id": "model-without-observations"}
  ]
}
```

调用方按 `id` 与模型列表关联。没有可用观测时只返回 `id`，不代表成功率为零；接口自身查询失败返回 HTTP `503`，不能当作模型不可用数据缓存。

## 推荐调用流程

1. 页面或客户端启动时请求 `/v1/models`，按 `type` 和 `features` 分组。
2. 用户选定模型后请求 `/v1/models/:code`，展示能力、支持的操作和对应价格。
3. 需要展示健康状态时单独请求 `/v1/models/availability`，按 `id` 合并，不阻塞模型列表加载。
4. 发起请求时只提交 Prism 模型名和公开业务参数，不提交渠道、上游模型名或内部 SKU。
