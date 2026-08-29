# idata-client

Windows 桌面端 agent。它通过可见窗口主动建立出站 WebSocket；也可在数字 loopback 地址上
提供可关闭的 Client Token 备用配对接口，不监听局域网地址。

## 构建

在任意 Go 开发机交叉编译无控制台窗口的 Windows EXE：

```bash
mkdir -p bin
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" \
  -o bin/idata-client-windows-amd64.exe ./cmd/idata-client
```

## 配置与启动

程序会自动读取与可执行文件位于同一目录的 `idata-client.json`。这适合 Windows 双击
启动场景；命令行参数优先于环境变量，环境变量优先于配置文件。示例见
`deploy/idata-client.json.example`。

| 环境变量 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `IDATA_SERVER_URL` | 否 | 无 | 为界面的 IP/端口提供初始值；成功连接后自动更新 |
| `IDATA_AGENT_TOKEN` | 是 | 无 | 管理员预置的客户端连接凭据，不在普通界面显示 |
| `IDATA_CLIENT_ID` | 否 | 当前 hostname | 稳定且唯一的设备 ID |
| `IDATA_DEVICE_TOKEN` | 否 | 自动生成 | 每台设备唯一的 Web 配对凭据，至少 32 字符 |
| `IDATA_OUTPUT_LIMIT` | 否 | `1048576` | stdout 和 stderr 各自的最大字节数 |
| `IDATA_ALLOW_INSECURE` | 否 | `true` | 是否允许对非本机地址使用明文 `ws://` |
| `IDATA_BROWSER_BRIDGE_ADDR` | 否 | `127.0.0.1:17891` | 浏览器配对回环地址；设为 `off` 可关闭 |
| `IDATA_CONFIRM_BROWSER_PAIRING` | 否 | `false` | 是否兼容 v0.4 的 Windows 本机确认请求 |
| `IDATA_REGISTER_URL_PROTOCOL` | 否 | `true` | 当前 Windows 用户注册 `idata://` 免密登录唤起协议 |

管理员也可以通过 Windows PowerShell 预置隐藏配置：

```powershell
$env:IDATA_SERVER_URL = 'ws://服务器IP/ws/agent'
$env:IDATA_AGENT_TOKEN = '...'
$env:IDATA_CLIENT_ID = 'office-windows'
.\idata-client-windows-amd64.exe
```

双击 EXE 后会显示由 EXE 内部直接创建的原生 Windows 登录窗口，不依赖 PowerShell UI
子进程。普通用户只需填写服务器 IP 和端口并点击
“建立连接”；真正建立连接后，窗口切换到状态页，可“中断连接”返回登录页，或“最小化到
托盘”并继续在线。窗口右上角保留标准最小化和关闭按钮，托盘菜单可恢复窗口或退出。

上一次成功建立连接的 IP 和端口会写入 EXE 同目录的 `idata-client.json`，下次启动自动填入；
连接失败的地址不会覆盖记录。`agent_token` 仍须由管理员提前写入该文件或环境变量，普通界面
不会显示或记录用户输入的认证信息。Client 会自动生成并补存每台设备独立的 `device_token`。

连接 v0.5.0 或更新 Server 后，从 Windows PC 打开 Server 根地址：

- “新用户下载 Client”会跳转到 idata-client 最新 GitHub Release。
- “老用户免密登录”会打开 `idata://login`。Windows 可能先显示“是否打开 iData Client”
  的系统提示；允许后，本 EXE 会启动。如果 Client 已经运行，新启动的唤起进程会检测本机
  loopback 服务并立即退出，不会建立第二条 agent 连接。
- Server 发现与浏览器直接来源 IP 相同的在线 Client 后完成免密登录，并列出该 IP 下的
  全部 Client，均可选择操作。

Client 每次正常启动都会在当前 Windows 用户的 `HKCU\Software\Classes\idata` 注册 URL
协议；不需要管理员权限。设置 `IDATA_REGISTER_URL_PROTOCOL=false` 可禁止后续注册，执行
`idata-client.exe --unregister-url-protocol` 可删除现有注册。移动或重命名 EXE 后需重新
运行一次，以更新协议指向的新路径。

授权后页面会创建一个持续 Shell。连续输入 `cd`、`set` 等 shell 内建
命令时状态会保留，stdout/stderr 会实时返回。页面关闭或连接断开时，客户端会结束该 Shell
及其进程树。第一版使用持久管道 Shell，不支持 `vim`、`top` 等依赖真实 PTY/ConPTY 的
全屏程序，也不能可靠传递 Ctrl+C；这些能力需要后续终端后端升级。

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
- client ID 不是密钥；真正的认证凭据是 agent token。
- 当前静态 token 对所有客户端共享。大规模部署下一步应升级为每设备凭据或 mTLS。
- 普通 Web 免密登录按 Server 看到的直接来源 IP 划分范围；共享 NAT、VPN 或代理出口下的
  用户会看到并能操作该出口下的全部 Client。这只适合小规模可信公司内网。
- `idata://` 只负责启动 Client，不携带 agent token、device token 或浏览器 Cookie。
- 浏览器配对 HTTP 接口只绑定 `127.0.0.1`，只允许配置的 Server Origin 读取，并可关闭。
- `device_token` 是可控制本机终端的 Bearer 凭据；不要在不同 Client 之间复用或对外发送。
- `idata-client.json` 中的 token 是明文凭据，应限制该文件仅授权管理员和运行账户可读，
  不要提交到版本库或通过不安全渠道传输。
- 卸载只需停止托管服务、删除可执行文件和相应环境配置；本程序不会自行注册持久化。
