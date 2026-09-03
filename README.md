# idata-client

Windows 桌面端及 macOS/Linux 命令行 agent。它通过可见窗口或前台终端主动建立出站 WebSocket；也可在数字 loopback 地址上
提供可关闭的 Client Token 备用配对接口，不监听局域网地址。

## 构建

在任意 Go 开发机交叉编译无控制台窗口的 Windows EXE：

```bash
mkdir -p bin
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" \
  -o bin/idata-client-windows-amd64.exe ./cmd/idata-client
```

仓库已包含 Windows AMD64 资源对象，构建时会自动嵌入 Common Controls v6、DPI 和
`asInvoker` 应用清单；不要从发布构建中移除该资源，否则原生控件可能无法创建。

## 配置与启动

程序会自动读取与可执行文件位于同一目录的 `idata-client.json`。这适合 Windows 双击
启动场景；命令行参数优先于环境变量，环境变量优先于配置文件。未配置时默认连接
`ws://10.90.65.189:12345/ws/agent`。示例见
`deploy/idata-client.json.example`。

macOS 和 Linux 使用前台命令行模式，也会自动连接默认 Server；需要覆盖地址时可显式提供
完整的 Server WebSocket URL。进程会显示连接、审批和重连状态，可随时按 `Ctrl+C` 退出，
不会注册后台服务或开机启动：

```bash
IDATA_SERVER_URL='ws://127.0.0.1:12345/ws/agent' ./idata-client
```

首次连接会自动提交设备申请。Server 默认自动批准所有有效的原生 Client 请求，无需人工
操作；若 Server 关闭自动批准才会等待管理员处理。设备专属凭据会以仅当前用户可读的权限保存在二进制同目录的
`idata-client.json` 中。

| 环境变量 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `IDATA_SERVER_URL` | 否 | `ws://10.90.65.189:12345/ws/agent` | Client 启动后自动连接的管理 Server；成功连接后自动记住 |
| `IDATA_AGENT_TOKEN` | 否 | 无 | 仅用于旧版共享凭据兼容；新设备默认使用自动申请 |
| `IDATA_CLIENT_ID` | 否 | 当前 hostname | 稳定且唯一的设备 ID |
| `IDATA_DEVICE_TOKEN` | 否 | 自动生成 | 每台设备唯一的 Web 配对凭据，至少 32 字符 |
| `IDATA_OUTPUT_LIMIT` | 否 | `1048576` | stdout 和 stderr 各自的最大字节数 |
| `IDATA_ALLOW_INSECURE` | 否 | `true` | 是否允许对非本机地址使用明文 `ws://` |
| `IDATA_BROWSER_BRIDGE_ADDR` | 否 | `127.0.0.1:17891` | 浏览器配对和已运行 Client 启动参数交接的回环地址；设为 `off` 可关闭 |
| `IDATA_CONFIRM_BROWSER_PAIRING` | 否 | `false` | 是否兼容 v0.4 的 Windows 本机确认请求 |
| `IDATA_REGISTER_URL_PROTOCOL` | 否 | `true` | 当前 Windows 用户注册 `idata://` 免密登录唤起协议 |

管理员也可以通过 Windows PowerShell 预置隐藏配置：

```powershell
$env:IDATA_SERVER_URL = 'ws://服务器IP/ws/agent'
$env:IDATA_CLIENT_ID = 'office-windows'
.\idata-client-windows-amd64.exe
```

双击 EXE 后会显示由 EXE 内部直接创建的原生 Windows 状态窗口，不依赖 PowerShell UI
子进程。Server 地址默认填入 `10.90.65.189`，也可以在连接页修改；Client 启动后会立即按
当前地址自动连接，无需点击按钮。要改用其他 Server，可先取消或中断连接，再编辑地址并重新
连接。窗口同时显示当前用户名、机器名、主要局域网 IPv4
和对应网卡 MAC 地址；真正建立连接后，窗口切换到状态页，可“中断连接”返回连接页，或“最小化到
托盘”并继续在线。窗口右上角保留标准最小化和关闭按钮，托盘菜单可恢复窗口或退出。

默认 Server `10.90.65.189` 使用专用端口 `12345`。若通过配置或环境变量指定其他服务器 IP，
仍默认使用端口 `80`。该专用映射会覆盖旧配置中为 `10.90.65.189` 保存的端口，升级后不需要
手工清理此前保存的 `:80` 地址。

Client 默认直接进入设备申请流程。Server 开启自动批准时，会立即签发与 Client ID、Device
Token 哈希绑定的专属凭据，Client 自动保存并连接；关闭时才等待管理员在 `/admin/` 控制台
批准。显式配置 `IDATA_AGENT_TOKEN` 仍可兼容
旧版共享凭据。Client 首次启动仍会
自动生成并补存每台设备独立的 `device_token`，但登录界面不会显示它。
若保存的凭据属于旧 Client ID 或已被 Server 撤销，Client 会自动清除失效凭据并重新注册，
不会持续重试同一个永久无效的连接。

连接更新后的 Server 后，从 Windows PC 打开 Server 根地址：

- “新用户下载 Client”会跳转到 idata-client 最新 GitHub Release。
- “启动 Client 并进入”会打开只包含当前页面 Server IP、端口和 HTTP/WSS 模式的
  `idata://connect` 链接。Windows 可能先显示“是否打开 iData Client”的系统提示；允许后，
  本 EXE 会启动并自动连接该 Server。链接不包含 token 或命令。如果 Client 已经运行，新启动的唤起进程会通过
  loopback 接口把经过校验的 Server 地址交给现有 Client；现有 Client 会切换到链接指定的
  IP、端口和 WS/WSS 模式，新进程随后退出，不会建立第二条 agent 连接。
- Server 发现与浏览器直接来源 IP 相同的在线 Client 后完成免密登录，并列出该 IP 下的
  全部 Client，均可选择操作。

macOS 用户在同一页面点击“macOS：复制启动命令”，将复制的命令粘贴到 Terminal 并按
Enter。Client 使用同一个 Server 地址自动注册和连接，页面执行与 Windows 完全相同的轮询、
同 IP 会话、设备选择及终端流程。若浏览器不允许自动写入剪贴板，页面会显示可手动复制的
只读命令。

Client 每次正常启动都会在当前 Windows 用户的 `HKCU\Software\Classes\idata` 注册 URL
协议；不需要管理员权限。设置 `IDATA_REGISTER_URL_PROTOCOL=false` 可禁止后续注册，执行
`idata-client.exe --unregister-url-protocol` 可删除现有注册。移动或重命名 EXE 后需重新
运行一次，以更新协议指向的新路径。注册所需的 Windows 系统命令会隐藏运行，因此手动启动
Client 时只显示一个主窗口。浏览器或 Windows 把链接规范化为 `idata://connect/?...` 时也可
正常唤起，其他路径和额外参数仍会被拒绝。

授权后页面会创建一个持续 Shell。连续输入 `cd`、`set` 等 shell 内建
命令时状态会保留，stdout/stderr 会实时返回。页面关闭或连接断开时，客户端会结束该 Shell
及其进程树。第一版使用持久管道 Shell，不支持 `vim`、`top` 等依赖真实 PTY/ConPTY 的
全屏程序，也不能可靠传递 Ctrl+C；这些能力需要后续终端后端升级。

远程命令和持续 Shell 使用隐藏的后台 `cmd.exe`，不会在 Windows 桌面弹出命令行窗口；
用于结束进程树的 `taskkill.exe` 同样隐藏运行。该行为只影响窗口显示，不改变当前用户权限、
命令审计边界或用户主动退出 Client 的能力。

Windows 也可以提前把下面两个文件放在同一目录后，直接双击 EXE：

```text
idata-client.exe
idata-client.json
```

标准发布构建只显示客户端主窗口，不显示额外控制台。点击“最小化到托盘”时连接继续，点击
关闭按钮或托盘“退出”时客户端结束。程序不会自行注册开机启动。

客户端意外断线后会在状态页显示提示并按指数退避自动重连；用户主动中断连接时不会重连。
客户端以当前 Windows 账户的权限执行命令，请遵循最小权限原则。

## 错误日志

Client 会把启动、连接、断线重试和异常退出信息追加到 `idata-client.log`。日志优先保存在
EXE 同目录；如果该目录不可写，则保存在当前用户的本地应用数据缓存目录下的 `iData` 目录。
发生启动错误或 panic 时，Client 会弹窗显示原因和实际日志路径，不再无提示闪退。诊断日志
还会逐项记录配置读取、URL 协议注册、原生 UI 启动和 UI 操作；Go 运行时写到 stderr 的崩溃
信息也会进入该文件。日志不会记录 token 或完整 Authorization header。

## 安全说明

- 当前 `ws://` 端口 80 方案没有传输加密，只适合受信任内网。
- client ID、本机 IP、MAC 地址都不是密钥；真正的认证凭据是 Server 签发的设备专属 token。
- 设备申请中的用户名、机器名、本机 IP 和 MAC 是 Client 自报信息，管理员还应核对 Server 看到的来源 IP。
- 共享 Agent Token 不能提供设备级隔离，只应用于旧版兼容；
  更严格的部署应关闭自动批准并使用人工审批签发设备专属凭据。
- 普通 Web 免密登录按 Server 看到的直接来源 IP 划分范围；共享 NAT、VPN 或代理出口下的
  用户会看到并能操作该出口下的全部 Client。这只适合小规模可信公司内网。
- `idata://` 只负责启动 Client，不携带 agent token、device token 或浏览器 Cookie。
- 已运行 Client 的启动参数交接只接受本机 loopback 上不带浏览器 `Origin` 的原生请求，并在
  切换前再次校验数字 IP、端口、WS/WSS 模式和固定 `/ws/agent` 路径。
- 浏览器配对 HTTP 接口只绑定 `127.0.0.1`，只允许配置的 Server Origin 读取，并可关闭。
- `device_token` 是可控制本机终端的 Bearer 凭据；不要在不同 Client 之间复用或对外发送。
- `idata-client.json` 中的 token 是明文凭据，应限制该文件仅授权管理员和运行账户可读，
  不要提交到版本库或通过不安全渠道传输。
- 卸载只需停止托管服务、删除可执行文件和相应环境配置；本程序不会自行注册持久化。
