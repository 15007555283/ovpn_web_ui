# OpenVPN WebUI

Go + Gin / Vue 3 + TypeScript + Naive UI 的单节点 OpenVPN 管理控制台。中文界面，前端资源嵌入一个 Go 二进制，JSON 原子持久化，无 SQLite、无单独 API 服务、无运行时 CDN。实现需求文档 Phase 0 / Phase 1，并提供基础部署及恢复支持；可选 Phase 3 不在范围内。

## 本地开发

需要 Go 1.25+、Node 20+。代码位于独立仓库 `ovpn_web_ui`。

```sh
make dev
```

`make dev` 会构建程序，并在 `config.json` 缺失时自动生成本地演示配置（`mode: "demo"`），数据目录使用当前项目下 `data` 的绝对路径；已有配置不会被覆盖。配置和数据均被 Git 忽略。如已完成构建，可先执行 `make dev-config`，再运行 `./bin/vpn-admin -config config.json`。

浏览器打开 `http://127.0.0.1:8080`，首次创建管理员（密码至少 12 字节），再登录。开发模式生成独立测试 CA/客户端证书，所有密钥留在忽略的 `data/fake` 中；生成的配置有 FAKE 标记，不能用于连接实际 VPN。fake 模式也不允许 Web 进程以 root 运行。

前端热更新可运行 `cd web && npm run dev`，将开发配置 `origin` 改成 Vite 的实际 URL（通常为 `http://127.0.0.1:5173`），Go 服务保持运行。部署构建使用本地打包资源。

fake 默认不伪造在线用户。可将 status-version 3 测试文件写到 `data/fake/status.log`；TIME 时间戳必须在两分钟内，必须含 CLIENT_LIST HEADER 和 END。无需安装 OpenVPN。

## 功能

- 一次性初始化、bcrypt 密码、服务端会话、退出及改密失效、来源校验、IP 登录失败锁定与限流。
- 用户创建、备注编辑、详情、证书实时验证、内存生成并下载 `.ovpn`、重新生成、撤销重试。
- IP/CIDR 规范化、保留网段及默认路由拦截、覆盖检测、逐行批量反馈、编辑/启停/删除、版本应用与失败回滚。
- status-version 3 在线列表、统计、最后连接时间、健康检查、受控服务重启、脱敏诊断及分页审计。
- JSONC 按功能保存到 `data/db/` 的固定模块文件，演示文件保留在 `data/fake/`；审计每 1000 条分片，事务支持中断恢复；Web 进程锁和 helper 跨进程锁阻止并发写入。

首次使用不会自动导入服务器已有路由，初始状态标记为“尚未应用”。首次应用会以界面中启用的规则全量替换服务器分流规则，请先补录需要保留的目标。

所有页面接口均在同一个 Go 服务 `/api/v1` 内。配置下载只在内存生成，无持久化 `.ovpn` 临时文件，因此无需清理下载文件。CA 私钥绝不返回 Web 服务。设置中的地址、协议、端口只改变生成的客户端配置，不修改服务器监听参数。

## Ubuntu 部署

先由你现有脚本安装 OpenVPN / EasyRSA，确认 VPN 已能独立运行。WebUI 不安装 OpenVPN、不修改安全组、NAT 或 FORWARD。

```sh
make linux
# 将 bin/vpn-admin-linux-amd64（或 arm64）和 deploy/ 传到服务器。
sudo bash deploy/install.sh /path/to/vpn-admin-linux-amd64
```

安装脚本安装同一二进制为 Web 和 root helper 两个路径，Web 使用专用 `vpn-admin` 用户，helper 经 sudoers 精确白名单调用。配置保存在 root 拥有且不可由普通用户修改的 `/data/ovpn/config.json`；`/etc/vpn-admin/config.json` 为 helper 使用的兼容链接。解析后的配置路径及所有祖先不能对组/其他用户可写；CA 私钥保持 root-only。

按服务器实际路径修改配置，并完成：

1. `origin` 设置为实际 HTTPS 管理地址；监听保持 `127.0.0.1`。
2. OpenVPN 主配置应使用绝对路径，包含：

   ```conf
   config /etc/openvpn/server/routes.conf
   crl-verify /etc/openvpn/server/crl.pem
   status /var/log/openvpn/status.log 10
   status-version 3
   ```

3. `routes.conf` 必须已存在；CRL 使用 EasyRSA 生成并安全安装；客户端私钥权限为 `0600`。TLS 使用 `tls-crypt`，不支持以 `tls-auth` 代替。
4. 参照 `deploy/nginx.conf` 配置 HTTPS 证书与反向代理。必须覆盖 `X-Real-IP` 和 `X-Forwarded-Proto`，不要透传客户端伪造值。访问控制建议仅允许自己的管理 IP。
5. `sudo nginx -t` 后按你的部署流程加载 Nginx；运行 `sudo systemctl start vpn-admin`，完成首次管理员初始化。

系统安全约束：`NoNewPrivileges=true` 会禁止 sudo helper 提权，因此此方案有意不启用。使用 Unix 权限、只读 `/usr`、root-owned helper、固定命令及严格校验保护边界。`ProtectSystem=full` 配合 `/etc/openvpn` 写入白名单，仅 root helper 能通过 Unix 权限写入该目录。应用标准输出仅记录启动信息；操作审计写入私有 JSON，不记录密码、Session、证书私钥或完整配置。

## 失败恢复

- 创建前先写入 `create_failed` 占位。外部创建失败后禁止下载。若实际证书已生成，可以“重新生成”验证并恢复；也可以撤销遗留证书。名字不能复用，不删除 PKI 历史。
- 撤销先标记 `revoke_pending`，立即禁止下载。若 CRL 生成/安装失败，继续点击“重试撤销”，会识别 PKI 已撤销状态并重新生成、验证、安装 CRL。
- 应用路由前保留 `.vpn-admin-backup`。重启失败则恢复旧文件并再次启动。若服务恢复也失败，备份保留，界面明确报告错误。helper 再次执行修改操作时会尝试恢复未完成事务。
- 应用与 JSON 写入之间崩溃可能留下 `applying` 记录；当前草稿仍可再次应用。重复提交同一个已应用 revision 不会再次重启服务。
- 配置预检查只检查固定 include、CRL 引用和禁止全局代理；OpenVPN 没有通用等价于 `nginx -t` 的无副作用全配置检查，最终以服务重启及 active 校验作为结果。不要把 `--test-crypto` 当作服务器配置语法检查。[OpenVPN 官方手册](https://openvpn.net/community-docs/community-articles/openvpn-2-6-manual.html)
- 健康检查对 iptables 做规则存在性提示，不能证明实际数据包一定正确转发。

## 备份、恢复与升级

JSON 包含管理员哈希与审计，权限必须保持 `0600`；目录 `0700`。需要单独备份 OpenVPN/EasyRSA 的完整 PKI（含 CA 私钥），用 root-only、加密的离线存储，不要放入 Git。为保持一致性，备份时停止 WebUI，并确保没有 helper 运行后，再一起备份整个 `/data/ovpn/data` 数据目录、`/etc/vpn-admin/config.json` 和 `/etc/openvpn`。在线状态、会话无需备份。

恢复时先停止 WebUI，恢复同一备份点的 JSON 与 PKI/CRL/路由，恢复原属主权限，再启动。不能只恢复旧 JSON 来“恢复”撤销的证书。升级前备份，重新执行安装脚本；它保留现有配置和数据，安装后由管理员手动启动。JSON 文件损坏时启动失败，不会以空数据覆盖；多进程实例共享数据目录会被锁拒绝。

## 验证边界

`make test` 运行 Go 单元/集成测试和静态检查；`make linux` 构建 Linux amd64/arm64。测试使用临时目录和 fake 命令，不会撤销宿主机证书或重启实际服务。

上线仍需在 Ubuntu 22.04/24.04 验证：OpenVPN Connect 导入、真实连接、status 文件更新、指定目标分流、其他目标本地出口、证书撤销后重连失败、系统重启后的服务和 NAT 恢复。开发 fake 测试不代表这些验收已经完成。

接口字段见 [docs/API.md](docs/API.md)，已验证项目和真实环境待验收项见 [docs/VALIDATION.md](docs/VALIDATION.md)。

## 命令行更新

新版安装脚本会提供 `ovpn-cli` 命令：

```sh
ovpn-cli version
ovpn-cli update --check
sudo ovpn-cli update
```

从本仓库 GitHub Release 下载当前架构的程序，验证 SHA256 后同步更新 WebUI 与 helper，启动失败自动回滚。配置、业务数据和 PKI 保留；只重启管理后台，不重启 OpenVPN。需要先发布正式 Release；旧安装须先用新版安装脚本部署一次。详见 [docs/UPDATE.md](docs/UPDATE.md)。

## 配置与存储

`config.json` 使用 `mode: "demo"` 或 `mode: "production"` 区分运行环境，可配置 OpenVPN 文件路径、PKI、服务名与重启/状态命令。生产仪表盘读取实际状态文件、PKI 索引与服务命令结果。旧数据自动迁移为 JSONC 分片；降级需要恢复迁移前的整套备份。完整字段、存储结构及迁移说明见 [docs/CONFIG.md](docs/CONFIG.md)。

## 前端维护

前端按页面组织在 `web/src/views`，布局、共用组件、请求和类型分别独立维护。入口 `App.vue` 只负责应用启动、主题和登录切换。目录职责与页面刷新约定见 [web/README.md](web/README.md)。
