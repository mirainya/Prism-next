# 参考素材上传配置

通用视频适配器通过 `adapter.request.reference_upload` 对象启用上传；省略该字段时直接保留素材 URL。配置保存在渠道能力配置中，不内置服务商名称、域名、路径或模型别名。旧的字符串模式不再接受。

对象包含：

| 字段 | 含义 |
|---|---|
| `operations` | `prepare`、`wait`、`complete`、`abort` 的同源 POST 路径 |
| `request_fields` | 执行器输入名到请求 JSON 字段路径的映射 |
| `response_fields` | 执行器结果名到响应 JSON 字段路径的映射 |
| `part_fields` | 分片编号、长度和签名 URL 的字段映射 |
| `completed_part_fields` | 完成请求中分片编号和 ETag 的字段名 |
| `content_path` | 生成请求中素材数组的直接字段名 |
| `content_fields` | 素材 URL、类型、已上传对象引用的直接字段名 |
| `fixed_body` | 每个操作额外需要的固定请求字段 |
| `success_path` / `success_values` | 可选的成功标记检查；启用后缺少标记也视为失败 |
| `state_values` | `owner`、`waiting`、`reused` 对应的响应状态 |
| `mode_values` | `single`、`multipart` 对应的上传模式 |

`request_fields` 必须映射 `sha256`、`size_bytes`、`kind`、`content_type`、`ticket`、`session_token`、`generation`、`parts`。

`response_fields` 必须映射 `disposition`、`ticket`、`retry_after_seconds`、`storage_object_id`、`upload.session_token`、`upload.generation`、`upload.mode`、`upload.upload_url`、`upload.parts`。只有当前响应实际存在的字段会被读取；复用和完成响应均把对象引用映射到 `storage_object_id`。

`part_fields` 映射 `part_number`、`size_bytes`、`upload_url`；`completed_part_fields` 映射 `part_number`、`etag`；`content_fields` 映射 `url`、`type`、`reference`。

请求和响应字段路径支持点分对象路径。素材字段与完成分片字段使用直接字段名。字段映射不得重叠。所有操作路径必须是同源绝对路径，不能包含主机、查询参数或相对目录段。

运行时保留重复素材复用、取消等待、单次 PUT、分片 PUT、完成与失败终止流程。上传操作使用渠道 Bearer 凭据；签名 PUT 请求不携带渠道凭据。测试使用虚构域名及两套不同的接口和字段映射验证这些行为。
