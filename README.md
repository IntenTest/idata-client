# idata-client

PC 端 agent，支持 macOS 和 Windows。它只发起出站 WebSocket 连接，不开放本地端口。

## 构建

在任意 Go 开发机交叉编译：

```bash
mkdir -p bin
GOOS=darwin GOARCH=arm64 go build -o bin/idata-client-darwin-arm64 ./cmd/idata-client
GOOS=darwin GOARCH=amd64 go build -o bin/idata-client-darwin-amd64 ./cmd/idata-client
GOOS=windows GOARCH=amd64 go build -o bin/idata-client-windows-amd64.exe ./cmd/idata-client
```

## 配置与启动

程序会自动读取与可执行文件位于同一目录的 `idata-client.json`。这适合 Windows 双击
启动场景；命令行参数优先于环境变量，环境变量优先于配置文件。示例见
`deploy/idata-client.json.example`。

| 环境变量 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `IDATA_SERVER_URL` | 是 | 无 | 端口 80 服务端地址，如 `ws://10.0.0.2/ws/agent` |
| `IDATA_AGENT_TOKEN` | 是 | 无 | 客户端连接凭据 |
| `IDATA_CLIENT_ID` | 否 | 当前 hostname | 稳定且唯一的设备 ID |
| `IDATA_OUTPUT_LIMIT` | 否 | `1048576` | stdout 和 stderr 各自的最大字节数 |
| `IDATA_ALLOW_INSECURE` | 否 | `true` | 是否允许对非本机地址使用明文 `ws://` |

macOS/Linux shell：

```bash
IDATA_SERVER_URL='ws://服务器IP/ws/agent' \
IDATA_AGENT_TOKEN='...' \
IDATA_CLIENT_ID='office-mac' \
./idata-client
```

Windows PowerShell：

```powershell
$env:IDATA_SERVER_URL = 'ws://服务器IP/ws/agent'
$env:IDATA_AGENT_TOKEN = '...'
$env:IDATA_CLIENT_ID = 'office-windows'
.\idata-client-windows-amd64.exe
```

Windows 也可以把下面两个文件放在同一目录后，直接双击 EXE：

```text
idata-client.exe
idata-client.json
```

标准构建会保留可见的控制台窗口，窗口关闭时客户端随之退出，不会静默安装或自行注册
开机启动。若需要系统级长期托管，应由管理员另行配置 Windows Service Wrapper，并保持
服务可见、可审计、可停止。

客户端断线后会自动指数退避重连。第一版建议先以前台方式验证，再由 macOS LaunchDaemon
或 Windows Service Wrapper 以一个权限受限、用途明确的系统账户托管。客户端以其运行账户的
权限执行命令；请遵循最小权限原则。

## 安全说明

- 当前 `ws://` 端口 80 方案没有传输加密，只适合受信任内网。
- client ID 不是密钥；真正的认证凭据是 agent token。
- 当前静态 token 对所有客户端共享。大规模部署下一步应升级为每设备凭据或 mTLS。
- `idata-client.json` 中的 token 是明文凭据，应限制该文件仅授权管理员和运行账户可读，
  不要提交到版本库或通过不安全渠道传输。
- 卸载只需停止托管服务、删除可执行文件和相应环境配置；本程序不会自行注册持久化。
