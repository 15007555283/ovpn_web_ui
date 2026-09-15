# 配置与数据存储

程序使用 `-config` 指定配置文件。生产部署中 Web 与 root helper 必须使用同一份 `/etc/vpn-admin/config.json`，修改配置后重启 `vpn-admin`。`config.json` 同样支持中文注释，扩展名不影响解析。配置不通过页面修改，文件保持 root 所有、普通用户不可写。

完整生产示例见 `deploy/config.example.jsonc`。本地 `make dev-config` 生成 `mode: "demo"`；不会覆盖已有文件。

## 运行模式

```json
{
  "mode": "demo",
  "listen": "127.0.0.1:8080",
  "origin": "http://127.0.0.1:8080",
  "data_dir": "/absolute/path/to/demo-data"
}
```

- `demo`：使用 `data_dir/fake` 下独立的测试 PKI、配置和状态文件，不调用真实服务命令。生成的客户端配置不能连接真实 VPN。
- `production`：使用配置的 OpenVPN 文件与命令，要求 HTTPS origin。不会在数据缺失时回退到演示数据。
- 未提供 `mode` 时兼容旧 `fake` 字段；两者同时提供且冲突会拒绝启动。建议删掉旧 `fake`，只保留 `mode`。全部省略时默认生产。
- 两种模式使用不同的 `data_dir`。新存储清单记录模式，拒绝混用；旧 `state.json` 首次迁移按当前配置模式标记，因此迁移前先确认旧数据来源。

## OpenVPN 配置项

所有路径使用绝对路径。省略字段时使用以下默认值。

| 字段 | 默认值 | 用途 |
| --- | --- | --- |
| `server_config` | `/etc/openvpn/server/server.conf` | 检查路由引用、CRL 引用与禁止全局代理 |
| `routes_file` | `/etc/openvpn/server/routes.conf` | 读取、备份、写入分流规则及失败恢复 |
| `status_file` | `/var/log/openvpn/status.log` | 读取在线用户、连接时间、收发字节 |
| `easyrsa_dir` | `/etc/openvpn/easy-rsa` | 执行该目录下的 `easyrsa`，作为工作目录 |
| `pki_dir` | `easyrsa_dir` 下的 `pki` | 签发、撤销、读取证书/私钥/索引和生成 CRL |
| `ca_file` | `/etc/openvpn/server/ca.crt` | 校验客户端证书及 CRL，生成客户端配置 |
| `tls_key_file` | `/etc/openvpn/server/tls-crypt.key` | 读取 tls-crypt 密钥，生成客户端配置 |
| `crl_file` | `/etc/openvpn/server/crl.pem` | 安装和检查撤销列表 |
| `vpn_network` | `10.8.0.0/24` | 路由冲突校验、NAT 检查及展示；不改写服务端网段 |
| `server_cn` | `server` | PKI 统计中排除服务器证书的 Common Name |
| `service_name` | `openvpn-server@server` | 默认重启/状态命令使用的 systemd 服务名 |
| `systemctl_path` | `/usr/bin/systemctl` | 生成默认服务命令使用的程序 |
| `openvpn_path` | `/usr/sbin/openvpn` | 通过 `--version` 获取实际版本 |
| `iptables_path` | `/usr/sbin/iptables` | 只读检查 NAT 和 FORWARD 规则 |
| `ip_forward_file` | `/proc/sys/net/ipv4/ip_forward` | 读取 IPv4 转发开关 |
| `restart_command` | systemctl restart 服务名 | 重启 OpenVPN，支持配置为程序与参数数组 |
| `status_command` | systemctl show 服务名及状态属性 | 查询服务状态、启动时间和最近结果 |

命令示例：

```json
{
  "service_name": "openvpn-server@office",
  "restart_command": ["/usr/bin/systemctl", "restart", "openvpn-server@office"],
  "status_command": ["/usr/bin/systemctl", "show", "openvpn-server@office", "--property=ActiveState,ActiveEnterTimestamp,Result", "--no-pager"]
}
```

命令不会经过 shell，每个参数单独填写。自定义脚本可填写其绝对路径，必须由 root 拥有且不可被普通用户改写。系统程序的符号链接会解析为经过权限检查的真实目标。状态命令成功时输出 `ActiveState=active`，也兼容单行 `active`；可以额外输出 `ActiveEnterTimestamp=...` 和 `Result=...`。重启之后最多检查状态 5 次；单次非 EasyRSA 命令超时 30 秒。EasyRSA 写操作持锁执行到结束。

省略两个命令数组时，会根据 `systemctl_path` 和 `service_name` 自动生成；已经显式填写数组时，以数组为准，不再替换其中的服务名。更改服务名时应同步更改数组或删除数组字段。

PKI 内部布局采用 EasyRSA 标准：`issued/<用户名>.crt`、`private/<用户名>.key`、`index.txt`、`crl.pem`。服务器证书 CN 应与 `server_cn` 一致，此控制台面向单节点、单服务器证书的 PKI。客户端连接地址、端口、协议、DNS 仍在页面“设置”中维护，不修改服务器监听参数。

生产路径须满足现有 root 属主及目录权限检查。若将可写的 PKI、CRL、路由放到 `/etc/openvpn` 之外，需同步修改 systemd 服务的 `ReadWritePaths`；若放到用户主目录，还会受 `ProtectHome=true` 限制。修改后执行 `systemctl daemon-reload` 并重启管理后台。sudo 和 helper 的调用路径仍由安装包固定，它们属于管理后台的权限边界，不是 OpenVPN 参数。

## 按模块保存 JSONC

运行数据与演示 OpenVPN 文件使用两个独立目录：

```text
data/
├── db/
│   ├── admin.jsonc          # 管理员账号与密码哈希
│   ├── settings.jsonc       # 连接和页面设置
│   ├── users.jsonc          # VPN 用户列表及证书元数据
│   ├── routes.jsonc         # 分流路由草稿
│   ├── revisions.jsonc      # 路由版本与应用记录
│   ├── audits-000000.jsonc  # 每片最多 1000 条审计，有记录时创建
│   ├── metadata.jsonc      # 格式版本、模式、模块校验摘要
│   └── legacy/             # 已迁移的旧数据备份，仅历史安装存在
├── fake/                   # 演示 CA、客户端证书、密钥和 OpenVPN 配置
├── app.lock                # 进程锁
└── state.json              # 旧版保护提示，不包含任何业务数据
```

`data_dir` 指向上面的 `data`。生产模式不会生成 `fake` 目录。启动参数配置仍是项目根目录或 `/etc/vpn-admin/config.json`，`db/settings.jsonc` 保存页面中的连接、刷新周期和展示设置，两者职责不同。

模块文件名固定，不带摘要。`metadata.jsonc` 不存用户列表、配置内容等业务数据。文件均为带中文注释、格式化缩进的明文 JSONC，不压缩、不使用 SQLite。支持 `//` 和 `/* */` 注释，不支持尾随逗号。目录权限为 `0700`，文件权限为 `0600`。

业务数据通过页面修改。每次保存先在 `db/.pending` 准备完整事务，再替换有变化的模块，最后提交元信息。若文件替换中断，下次启动会校验并完成已准备事务；准备阶段失败保留原数据。未变化的正式模块文件不重写。不要直接编辑模块文件或校验摘要。

旧 `state.json` 和旧 `store/` 摘要分片在启动时自动迁移，原文件归档到 `db/legacy/`；根目录 `state.json` 只保留中文保护提示，防止旧版程序误建空库。数据库缺失、损坏或模式不符时拒绝启动，不会自动用过期归档覆盖新数据。

备份前停止管理后台，并将整个 `data/db`、启动配置和 OpenVPN PKI/CRL/路由作为同一备份点保存。降级须恢复迁移前整套备份，不能只替换旧二进制。数据仍加载到内存中，按模块存储不代表无限容量。

## 生产仪表盘数据来源

- 在线用户、连接时间、流量：读取 `status_file`，要求 `status-version 3`，更新时间两分钟内；缺失、过期或截断显示暂不可用。
- 服务状态、启动时间：执行 `status_command`；实际版本来自 `openvpn_path --version`。
- 客户端证书总数、有效数、撤销数：读取实际 `pki_dir/index.txt`，排除 `server_cn` 和 CA；包含后台之外签发的客户端证书，并根据有效期统计。
- 分流规则数：读取当前 `routes_file` 中的 `push "route ..."` 条目；管理页面的路由列表仍是后台维护的草稿。
- IPv4 转发、NAT、FORWARD：读取指定系统文件及实际 iptables 规则。规则存在性检查不代表真实网络连通验证。
- 最近操作：后台的实际审计记录。

生产数据读取失败显示“—”及相应提示，不以演示数据或零值冒充真实数据。
