# 上游错误消息替换

规则存储在 `gw_upstream_error_rules` 表的唯一记录（`id=1`）中。
默认匹配余额、balance 和带货币标识的金额；任一正则命中，就将整条上游错误消息替换为配置的 `message`，状态码和错误码不变。
用户自身的额度错误不经过此规则。

管理员接口（沿用管理员 JWT 鉴权）：

- `GET /api/admin/unified-gateway/upstream-error-rules`：读取规则。
- `PUT /api/admin/unified-gateway/upstream-error-rules`：保存规则。

请求示例：

```json
{
  "patterns": ["(?i)余额|\\bbalance\\b", "[¥￥$€£]\\s*[0-9]+(?:\\.[0-9]+)?"],
  "message": "服务暂时不可用，请稍后重试"
}
```

`patterns: []` 关闭匹配。最多 32 条 Go 正则，每条最多 2048 字节；消息不能为空且最多 500 字符。
保存前编译校验，失败不写数据库。保存成功后本实例立即生效，其他实例每 30 秒刷新。
请求处理只读取内存规则，不查询数据库；刷新失败保留此前规则。

首次部署需先应用迁移 `20261013_110000_upstream_error_rules.sql`，再启动新版本服务。
上游原始响应的记录仍由现有日志保留策略控制。
