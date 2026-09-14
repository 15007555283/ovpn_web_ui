# 页面接口

由 Go 服务同源提供，统一前缀 `/api/v1`，无需额外部署 API 进程。

成功响应：`{"code":"OK","message":"ok","data":{},"request_id":"..."}`。失败使用非 2xx HTTP 状态，包含字符串错误码、中文 message 和 request_id。认证使用 HttpOnly Session Cookie；生产 Secure + SameSite=Strict。修改请求发送 JSON、`X-VPN-Admin: 1`，浏览器 Origin 必须匹配配置。无 CORS。

| 方法 | 路径 | 请求或说明 |
| --- | --- | --- |
| GET | `/setup/status` | `needs_setup`、`fake` |
| POST | `/setup/admin` | `username`、`password`；一次性 |
| POST | `/auth/login` | `username`、`password` |
| POST | `/auth/logout` | 清除会话 |
| GET | `/auth/me` | 当前管理员及 fake 标志 |
| PUT | `/auth/password` | `old_password`、`new_password`；撤销全部会话 |
| GET | `/dashboard` | 状态、统计、最近 10 条审计 |
| GET | `/vpn-users` | 可选 `q`、`status` 查询 |
| POST | `/vpn-users` | `username`、`remark`、可选 `endpoint` |
| GET | `/vpn-users/:id` | 用户详情 |
| PUT | `/vpn-users/:id` | `remark` |
| POST | `/vpn-users/:id/revoke` | `confirm` 必须等于用户名 |
| GET | `/vpn-users/:id/config` | 附件下载，实时校验证书、PKI 索引和密钥匹配 |
| POST | `/vpn-users/:id/regenerate-config` | 保存当前连接设置快照，校验证书；不在 JSON 返回配置 |
| GET | `/routes` | 全部规则 |
| POST | `/routes` | `cidr`、`remark` |
| POST | `/routes/batch` | `lines`（换行分隔）、`remark`；最多 500 行，逐行结果 |
| PUT | `/routes/:id` | `remark`、`enabled` |
| DELETE | `/routes/:id` | 删除草稿记录 |
| GET | `/routes/pending` | `revision`、`applied_revision`、`pending`、`history` |
| POST | `/routes/apply` | `revision`、`confirm:true`；冲突返回 409，重复已应用版本不重启 |
| GET | `/sessions/online` | `available`、`message`、`updated_at`、`clients` |
| GET | `/system/health` | 健康检查，fake 数据明确标记 |
| GET | `/system/openvpn` | 同健康检查 |
| POST | `/system/openvpn/restart` | `confirm:true` |
| GET | `/system/diagnostics` | 脱敏 JSON 附件，无用户来源 IP、密钥或原始日志 |
| GET | `/audit-logs` | `page` 从 1 开始，每页 50 条；`q` 搜索动作/资源/请求 ID |
| GET | `/settings` | 非敏感设置 |
| PUT | `/settings` | 完整设置对象，见下例 |

```json
{
  "endpoint": "vpn.example.com",
  "port": 1194,
  "protocol": "udp",
  "dns": "",
  "refresh": 10,
  "session_hours": 8,
  "name": "VPN Admin"
}
```

- `protocol` 为 `udp` / `tcp-client`，`refresh` 5～300 秒，`session_hours` 1～168 小时。
- 用户状态：`active`、`create_failed`、`revoke_pending`、`revoked`。`revoke_pending` 是 CRL 重试恢复状态，同样禁止下载。
- 下载使用创建或重新生成时保存的连接设置快照；更改系统设置不会偷偷改写旧用户的配置。
- `applied_revision=-1` 表示新实例尚未应用；不会假定现有服务器文件与空 JSON 相同。
- 状态文件不可用时响应仍为 200，但 `available=false`，页面不得将其解释为零人在线。
- 私钥不会出现在任何 JSON 响应或持久化元数据中。仅授权下载附件包含对应客户端私钥。
