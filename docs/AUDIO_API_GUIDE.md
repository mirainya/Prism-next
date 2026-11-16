# 语音生成下游对接文档

接口地址：`https://prism.example.com`。以下以公开模型 `indextts2` 为例，更新于 2026-11-06。

## 1. 鉴权与准备

所有请求携带 `Authorization: Bearer <PRISM_TOKEN>`。Token 须已绑定文件存储，并有可用额度。
当前 `indextts2` 成功调用为 **1 积分／次**，生成结果为 WAV。

流程：查询模型 → 提交任务 → 轮询任务 → 下载音频。

## 2. 模型发现与类型区分

```bash
curl 'https://prism.example.com/v1/models' \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

模型列表返回 `{"object":"list","data":[...]}`。其中 `indextts2` 条目包含：

```json
{
  "id": "indextts2",
  "object": "model",
  "owned_by": "prism",
  "name": "IndexTTS2",
  "type": "audio",
  "types": ["audio"],
  "features": ["audio"],
  "native_transports": ["audio_generation"],
  "supported_operations": ["audio.generate"],
  "supported_endpoints": ["/v1/audio/generations"]
}
```

下游按 `supported_operations` 是否包含 `audio.generate` 识别语音生成能力，按 `types` 分组。
直接使用返回的 `id` 作为请求中的 `model`，无需维护模型名称白名单。

```javascript
const speechModels = data.filter(model =>
  model.supported_operations?.includes('audio.generate')
);
```

该筛选能区分语音生成模型和仅支持音频输入的其他模型。
这里的模型条目是分类摘要，不是生成请求的参数表；当前模型详情也未提供语音参数 schema。
完整生成参数见第 3、5 节，不能据此摘要推断只支持几个参数。
列表只返回模型摘要；价格通过 `GET /v1/models/indextts2` 查询，详情直接返回模型对象。
价格位于 `pricing.variants[].components[]`，当前相关字段为：

```json
{
  "component": "request",
  "unit": "request",
  "unit_price": "1.000000000000000000",
  "quantity_source": "one",
  "charge_event": "call.succeeded",
  "mode": "flat"
}
```

`pricing.currency` 为 `CREDIT`。模型信息及结果不包含内部渠道、鉴权或请求映射。

## 3. 提交语音任务

`POST /v1/audio/generations`，请求体为 JSON。

| 字段 | indextts2 要求 | 说明 |
|---|---|---|
| `model` | 必填 | `indextts2` |
| `input` | 必填 | 字符串，1～2048 个 Unicode 字符，不能全为空白或包含 NUL |
| `reference_audio` | 必填 | 公网可访问的 MP3／WAV 音频 URL |
| `emotion` | 可选 | 情绪配置，省略时沿用参考音频情绪 |

参考音频使用有效的 HTTP(S) 地址，私网地址会被拒绝。未知请求字段会被拒绝。
参考音频必填和文本长度上限是当前模型的要求，其他模型可不同。

```bash
curl 'https://prism.example.com/v1/audio/generations' \
  -H "Authorization: Bearer $PRISM_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: speech-order-20261106-001' \
  -d '{
    "model": "indextts2",
    "input": "你好，这是语音生成测试。",
    "reference_audio": "https://assets.example.com/reference.wav"
  }'
```

请将示例参考地址替换为实际可访问的音频地址。
成功返回 HTTP **202**，响应头包含 `Location` 和 `Retry-After: 3`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "任务UUID",
    "status": "queued",
    "operation": "audio.generate",
    "location": "/v1/audio/tasks/任务UUID"
  }
}
```

HTTP 202 表示任务已接受；音频需等待任务完成后获取。

## 4. 轮询与下载

使用创建任务的同一个 Token，按返回的 `location` 查询，建议间隔 3 秒：

```bash
curl 'https://prism.example.com/v1/audio/tasks/任务UUID' \
  -H "Authorization: Bearer $PRISM_TOKEN"
```

| `data.status` | 处理方式 |
|---|---|
| `queued` | 等待并继续查询 |
| `processing` | 生成中，继续查询 |
| `completed` | 读取 `data.result` 并下载 |
| `failed` | 停止查询，读取 `data.error` |

完成时的响应示例（省略时间与进度字段）：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "任务UUID",
    "model": "indextts2",
    "operation": "audio.generate",
    "status": "completed",
    "result": {
      "url": "https://storage.example.com/result.wav?signature=...",
      "format": "wav"
    }
  }
}
```

`result.url` 是已转存结果的下载地址。下载请求无需携带 Prism Token；保留完整 URL 及查询参数。
签名地址有时效，过期后重新查询任务获取当前下载地址。
失败时 `data.error` 为 `{"code":"audio_task_failed","message":"audio generation failed"}`。

## 5. 情绪控制

除 `model` 外，生成参数共 13 项：`input`、`reference_audio`，以及 `emotion` 内的 11 项。
情绪字段采用通用命名并嵌套组织，全部保留；`emotion.strengths` 下的 8 项均为数值。

| 完整字段 | 类型／取值 | 含义与默认值 |
|---|---|---|
| `emotion.mode` | `voice`／`reference`／`vector` | 控制模式，省略时为 `voice` |
| `emotion.reference_audio` | HTTP(S) URL | 情绪参考音频；`reference` 模式必填，支持 MP3／WAV |
| `emotion.random` | `true`／`false` | 情绪随机化开关，默认 `false`，不接受字符串或数字 |
| `emotion.strengths.happy` | number，0～1.4 | 开心，默认 0 |
| `emotion.strengths.angry` | number，0～1.4 | 愤怒，默认 0 |
| `emotion.strengths.sad` | number，0～1.4 | 悲伤，默认 0 |
| `emotion.strengths.afraid` | number，0～1.4 | 害怕，默认 0 |
| `emotion.strengths.disgusted` | number，0～1.4 | 厌恶，默认 0 |
| `emotion.strengths.melancholic` | number，0～1.4 | 忧郁，默认 0 |
| `emotion.strengths.surprised` | number，0～1.4 | 惊讶，默认 0 |
| `emotion.strengths.calm` | number，0～1.4 | 平静，默认 0 |

`voice` 沿用音色参考情绪；`reference` 使用独立情绪参考；`vector` 使用上述强度。
强度范围包含端点，即每项 `0 <= 值 <= 1.4`，支持小数。范围按单项校验，不要求 8 项之和等于 1。
`emotion.mode` 省略时使用 `voice`；有效模式名区分大小写。
音色参考 `reference_audio` 在三种模式中均必填。以下是包含全部强度字段的生成请求：

```json
{
  "model": "indextts2",
  "input": "你好，这是带情绪控制的语音生成测试。",
  "reference_audio": "https://assets.example.com/voice.wav",
  "emotion": {
    "mode": "vector",
    "random": false,
    "strengths": {
      "happy": 0.5,
      "angry": 0,
      "sad": 0,
      "afraid": 0,
      "disgusted": 0,
      "melancholic": 0,
      "surprised": 0,
      "calm": 0.3
    }
  }
}
```

使用情绪参考时，将 `emotion` 改为
`{"mode":"reference","reference_audio":"https://assets.example.com/emotion.wav"}`。

## 6. 幂等、计费与错误

每个业务任务生成一个唯一的 `Idempotency-Key`。网络重试时保持同一个 Key、Token 和请求内容。
相同幂等请求返回原任务 ID，完成前后重复提交均不重复生成、计费；同一个 Key 更换请求内容返回 HTTP 409。
省略幂等 Key 的重复提交可能创建独立任务。

| HTTP 状态 | 含义 |
|---|---|
| `400` | 参数无效或额度不足，检查错误体 |
| `401` | Token 缺失、无效或已撤销 |
| `404` | 当前 Token 下找不到该任务，或模型不存在 |
| `409` | 幂等 Key 与原请求冲突 |
| `503` | 当前生成服务不可用 |

任务查询的 HTTP 200 仍须检查 `data.status`，不能将 HTTP 成功直接视为生成成功。
