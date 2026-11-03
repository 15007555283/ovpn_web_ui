# 页面接口

由 Go 服务同源提供，统一前缀 `/api/v1`，无需额外部署 API 进程。

成功响应：`{"code":"OK","message":"ok","data":{},"request_id":"..."}`。失败使用非 2xx HTTP 状态，包含字符串错误码、中文 message 和 request_id。认证使用 HttpOnly Session Cookie；生产 Secure + SameSite=Strict。修改请求发送 JSON、`X-VPN-Admin: 1`，浏览器 Origin 必须匹配配置。无 CORS。

| 方法 | 路径 | 请求或说明 |
| --- | --- | --- |
| GET | `/setup/status` | `needs_setup`、`fake`、`version` |
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
| POST | `/routes` | `cidr` 或 `lines`（换行分隔多个地址）、`remark`；保存为一条记录 |
| POST | `/routes/batch` | `lines`（换行分隔）、`remark`；最多 500 行，逐行结果 |
| PUT | `/routes/:id` | `remark`、`enabled`、可选 `lines`（换行分隔目标 IP / CIDR） |
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

## 运行模式与生产统计

`/setup/status`、`/auth/me`、`/dashboard` 返回 `mode`（`demo` 或 `production`），保留 `fake` 的接口兼容字段（dashboard 通过 mode 区分）。生产 `/dashboard` 的 `total_users`、`active_certificates`、`revoked_certificates` 来源为实际 PKI 索引，`routes` 来源为当前服务器路由文件；读取失败字段为 `null`。`health.certificate_error` 提供 PKI 不可用原因。在线统计继续通过 `online.available` 判断可用性。

### 后台版本检查

- `GET /api/v1/dashboard` 新增 `version`，为当前 WebUI/ovpn-cli 进程的编译版本；不与 OpenVPN 版本混用。
- `health.started_at` 使用服务器本地时间 `YYYY-MM-DD HH:mm:ss`；无法解析时为空字符串，页面显示 `—`。
- `GET /api/v1/system/version`：需要管理员登录，返回 `current`、`latest`、`update_available`、`release_url`。复用 GitHub 正式版查询，成功结果缓存 60 秒，请求超时 5 秒；失败返回 `502 VERSION_CHECK_FAILED`，不显示为“已是最新版本”。
- 仪表盘提供 `sudo ovpn-cli update` 命令说明，页面检查不会自动安装程序或重启服务。

### 编辑多个路由地址

`POST /routes` 和 `PUT /routes/:id` 支持 `lines`，每行一个 IP / CIDR。多个地址合并为一条记录，共用备注、启用状态和 ID；编辑时整体替换该组地址，保留原 ID 和创建时间。不传 `lines` 的编辑仍只修改备注及启用状态。

`cidr` 字段保存规范化地址，多个地址用换行分隔，旧单地址记录无需迁移。列表用逗号分隔展示，编辑时每行一个地址；应用时逐个展开为独立的 OpenVPN `push route`。禁用或删除整组对组内全部地址生效。

每组最多 500 行，全局最多 4096 个地址。忽略空行；空地址、非法网段、重复或被现有规则覆盖的地址会整组拒绝。新增或编辑一次只增加一个草稿版本，不自动应用或重启 OpenVPN。旧 `/routes/batch` 继续按行建立独立记录，重复校验同时检查组内地址。

### 服务器出口 IP

`GET /system/egress-ip` 需要管理员登录，返回 `ip`、`message`、`checked_at`。生产模式由服务器直连 [ipify IPv4 API](https://www.ipify.org/)，不使用环境代理；超时 5 秒，成功缓存 5 分钟，失败缓存 1 分钟。失败返回空 `ip` 和中文说明，不回退为配置中的连接地址。演示模式不发起外网探测。该值是管理进程所在服务器的默认公网出口，不代表特殊策略路由下每个 VPN 客户端或目标地址的出口。

### 分流项目

- `GET /route-projects`：返回项目列表，字段 `id`、`name`、`remark`。
- `POST /route-projects` / `PUT /route-projects/:id`：创建或编辑，项目名称必填、最多 80 字、不能重复或为“未分组”，最多 200 个项目。
- `DELETE /route-projects/:id`：仅允许删除空项目；含规则返回 `409 PROJECT_NOT_EMPTY`。
- 路由新增和编辑支持 `project_id`；空字符串为“未分组”。编辑省略该字段时保留原归属，未知项目拒绝保存。旧批量接口仍新增到未分组。
- 项目名称、备注持久化在 `data/db/projects.jsonc`，规则归属随 `routes.jsonc` 保存。存储版本升为 3，兼容读取版本 2，旧规则自动显示为未分组；旧版程序不能读取版本 3，降级需恢复升级前完整备份。
- 项目只组织规则，不隔离用户；项目列表筛选不影响应用范围。应用时仍汇总所有项目启用的地址，跨项目重复地址沿用全局校验。
