# ovpn-cli 更新

`ovpn-cli` 是同一个 Go 可执行文件的命令入口，不额外安装语言运行时。安装脚本会创建 `/usr/local/bin/ovpn-cli` → `/usr/local/bin/vpn-admin` 的链接；如果已有其他同名工具，安装会停止，避免覆盖它。

## 使用

```sh
ovpn-cli version
ovpn-cli update --check
sudo ovpn-cli update
# 明确选择某个已发布的稳定版本，也可用于有意降级：
sudo ovpn-cli update --version v0.1.0
```

- `--check` 只读取 GitHub 版本信息，普通用户和本地开发机也能使用。
- 真正安装只支持 Linux amd64 / arm64，需要管理员通过 sudo 执行。不开放给 WebUI，也不加入 Web 用户的 sudoers 白名单。
- 更新源固定为 `15007555283/ovpn_web_ui` 的 GitHub Release。不接受任意仓库、下载 URL 或安装路径。
- 无指定版本时选择最新正式发布版本，不自动降级到较旧版本；草稿、预发行版本和非 `vX.Y.Z` 标签不受支持。
- 安装了早期不带 CLI/版本能力的包时，需要先用新版 `deploy/install.sh` 部署一次，再使用以上命令。

首次发布 Release 之前，`--check` 返回“未找到已发布版本或附件”，不会把 Git 提交当作可安装版本。推送普通 main 分支也不会自动发布。

私有仓库可以通过环境变量 `GH_TOKEN` 提供具备该仓库 Contents 读取权限的令牌。使用 sudo 时显式保留该变量：

```sh
sudo --preserve-env=GH_TOKEN ovpn-cli update
```

令牌不写入配置和日志；下载跳转离开 GitHub API 域名后会移除认证头。

## 更新过程

1. 获取 Release 元数据、目标架构程序及 `checksums.txt`，验证大小和 SHA256。
2. 执行候选程序的 `version` 自检；下载、校验和自检失败都不会停止 WebUI。
3. 在 root 独占的 `/var/lib/vpn-admin-update/pending` 中备份旧主程序、旧 helper 和更新事务记录。
4. 停止 `vpn-admin.service`，取得与 EasyRSA/路由管理共用的进程锁。仍有 PKI 操作时拒绝替换，不杀死 EasyRSA。
5. 原子替换主程序和 helper。原先正在运行的 WebUI 会启动并检查 HTTP 响应中的版本号；原先停止的 WebUI 保持停止。
6. 新版本启动失败，自动恢复两份旧程序和原服务状态。如果恢复也失败，保留备份并报告错误。

更新中断后，再次执行 `sudo ovpn-cli update` 会优先恢复上次事务，不依赖网络。恢复完成后，本次命令结束；再次执行才尝试新版本。

更新会短暂中断管理后台并使内存登录会话失效，需要重新登录。更新器不重启 OpenVPN，也不修改配置、业务 JSON、PKI、分流路由或 CRL。

SHA256 用于校验下载完整性，发布者信任建立在固定 GitHub 仓库及 HTTPS 上，不等同于独立签名认证。具有 Release 写入权限的人能够发布以 root 执行的新程序，应保护仓库及发布权限。

## 发版

代码已包含 `.github/workflows/release.yml`。推送正式版本标签时，GitHub Actions 构建和测试后发布：

- `vpn-admin-linux-amd64` / `vpn-admin-linux-arm64`：CLI 更新使用的单二进制。
- 对应 `.tar.gz`：包含程序、安装脚本和说明，用于首次部署。
- `checksums.txt`：上述附件的 SHA256 清单。

```sh
# 在准备发布的提交上操作，版本号按实际发布决定：
git tag v0.1.0
git push origin v0.1.0
```

也可以本地 `make linux VERSION=v0.1.0` 构建带版本号的程序。未指定 VERSION 或直接 go build 时版本为 `dev`。

本轮没有创建版本标签或发布 Release。仓库管理员需要允许 GitHub Actions 执行，并在首次发版后到 Ubuntu 测试完整在线升级。GitHub API 使用官方 [Release](https://docs.github.com/en/rest/releases/releases) / [Release Asset](https://docs.github.com/en/rest/releases/assets) 接口。

## 兼容性范围

CLI 更新当前安装的二进制，不自动覆盖 systemd、sudoers、Nginx 或用户配置。将来若版本要求更改部署权限、目录或进行不兼容数据迁移，应在发版说明中要求先按新的安装/迁移流程操作，不能仅靠替换二进制完成。回滚恢复程序，不回退管理员在运行期间产生的业务数据。
