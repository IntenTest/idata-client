# idata-client 开发指南

先阅读仓库根目录 `AGENTS.md` 和 `docs/protocol.md`。

## 职责

- `cmd/idata-client`：配置、生命周期、信号处理。
- `internal/agent`：连接、hello、重连、收取命令和回传结果。
- `internal/executor`：跨平台 shell 调用、超时、退出码和输出限制。
- `internal/protocol`：线上 JSON 类型；任何改动都要同步协议文档和 server。

## 不变量

- 客户端到 Server 只建立出站连接。浏览器配对接口只能绑定数字 loopback 地址、校验精确
  Server Origin，并可通过配置关闭；不得监听局域网或公网地址。
- Server 发来的浏览器配对请求只能在 Windows 上通过置顶可见窗口人工确认，必须校验限时
  请求字段并要求完整短语精确匹配；同一时间只允许一个窗口，功能必须可配置关闭。
- token 不可出现在日志、进程参数示例或错误详情里；推荐仅通过环境变量注入。
- macOS 使用 `/bin/sh -c`，Windows 使用 `cmd.exe /D /S /C`；平台差异要有测试。
- 每条命令都必须有超时和输出上限；取消连接不应留下可控范围内的孤儿进程。
- 不加入静默安装、隐藏窗口、安全软件绕过或提权机制。
- 重连使用带上限的指数退避，不可形成紧密重试循环。

## 验证

在本目录运行：

```bash
go test ./...
go vet ./...
GOOS=windows GOARCH=amd64 go build ./cmd/idata-client
GOOS=darwin GOARCH=arm64 go build ./cmd/idata-client
```
