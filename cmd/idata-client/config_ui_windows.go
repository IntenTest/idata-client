//go:build windows

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

type clientUIInitial struct {
	ServerURL   string
	AgentToken  string
	DeviceToken string
	AutoConnect bool
}

type clientUIAction struct {
	Action      string
	ServerURL   string
	AgentToken  string
	DeviceToken string
}

type clientUIUpdate struct {
	State      string
	ServerIP   string
	ServerPort string
	Message    string
}

type clientUI struct {
	mainWindow       *walk.MainWindow
	loginPanel       *walk.Composite
	connectedPanel   *walk.Composite
	serverURL        *walk.LineEdit
	agentToken       *walk.LineEdit
	deviceToken      *walk.LineEdit
	showTokens       *walk.CheckBox
	loginStatus      *walk.Label
	connectButton    *walk.PushButton
	connectedAddress *walk.Label
	connectedStatus  *walk.Label
	notifyIcon       *walk.NotifyIcon
	actions          chan clientUIAction
	done             chan error
	logger           *slog.Logger
	connecting       bool
	closed           atomic.Bool
}

func startClientUI(initial clientUIInitial, logger *slog.Logger, logFile *os.File) (*clientUI, error) {
	ui := &clientUI{
		actions: make(chan clientUIAction, 16),
		done:    make(chan error, 1),
		logger:  logger,
	}
	started := make(chan error, 1)
	go ui.run(initial, started)
	if err := <-started; err != nil {
		return nil, err
	}
	logCheckpoint(logger, logFile, "native Windows UI started")
	return ui, nil
}

func (ui *clientUI) run(initial clientUIInitial, started chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	startedSent := false
	var runErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = fmt.Errorf("native Windows UI panic: %v", recovered)
			ui.logger.Error("native Windows UI panic", "error", recovered)
		}
		if !startedSent {
			started <- runErr
		}
		if ui.notifyIcon != nil {
			_ = ui.notifyIcon.SetVisible(false)
			_ = ui.notifyIcon.Dispose()
		}
		ui.closed.Store(true)
		close(ui.actions)
		ui.done <- runErr
		close(ui.done)
	}()

	if err := ui.createWindow(initial); err != nil {
		runErr = err
		return
	}
	started <- nil
	startedSent = true
	ui.emit(clientUIAction{Action: "ready"})
	if initial.AutoConnect && strings.TrimSpace(initial.ServerURL) != "" && strings.TrimSpace(initial.AgentToken) != "" && validDeviceToken(initial.DeviceToken) {
		ui.beginConnect()
	}
	ui.mainWindow.Run()
}

func (ui *clientUI) createWindow(initial clientUIInitial) error {
	if err := initializeWindowsControls(); err != nil {
		return err
	}
	blue := walk.RGB(18, 113, 232)
	dark := walk.RGB(45, 55, 72)
	muted := walk.RGB(90, 101, 120)
	background := SolidColorBrush{Color: walk.RGB(246, 248, 252)}
	width, height := 450, 620
	x := (int(win.GetSystemMetrics(win.SM_CXSCREEN)) - width) / 2
	y := (int(win.GetSystemMetrics(win.SM_CYSCREEN)) - height) / 2

	window := MainWindow{
		AssignTo: &ui.mainWindow,
		Title:    "iData Client",
		Bounds:   Rectangle{X: x, Y: y, Width: width, Height: height},
		MinSize:  Size{Width: width, Height: height},
		MaxSize:  Size{Width: width, Height: height},
		Font:     Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			Composite{
				Background: SolidColorBrush{Color: blue},
				MinSize:    Size{Height: 112}, MaxSize: Size{Height: 112},
				Layout: VBox{Margins: Margins{Left: 24, Top: 18, Right: 24, Bottom: 14}, Spacing: 2,
					Alignment: AlignHCenterVCenter},
				Children: []Widget{
					Label{Text: "iD", Font: Font{Family: "Segoe UI", PointSize: 25, Bold: true},
						TextColor: walk.RGB(255, 255, 255), TextAlignment: AlignCenter, MinSize: Size{Height: 58}},
					Label{Text: "iData Client", Font: Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
						TextColor: walk.RGB(255, 255, 255), TextAlignment: AlignCenter, MinSize: Size{Height: 28}},
				},
			},
			Composite{
				AssignTo: &ui.loginPanel, Background: background,
				Layout: VBox{Margins: Margins{Left: 46, Top: 24, Right: 46, Bottom: 24}, Spacing: 7},
				Children: []Widget{
					Label{Text: "连接到管理服务器", Font: Font{Family: "Microsoft YaHei UI", PointSize: 11, Bold: true},
						TextColor: dark, MinSize: Size{Height: 30}},
					VSpacer{Size: 4},
					Label{Text: "Server URL", TextColor: muted},
					LineEdit{AssignTo: &ui.serverURL, Text: initial.ServerURL, CueBanner: "例如 ws://10.0.0.2/ws/agent", MinSize: Size{Height: 30}},
					VSpacer{Size: 4},
					Label{Text: "Agent Token", TextColor: muted},
					LineEdit{AssignTo: &ui.agentToken, Text: initial.AgentToken, PasswordMode: true, CueBanner: "请输入客户端连接凭据", MinSize: Size{Height: 30}},
					VSpacer{Size: 4},
					Label{Text: "Device Token", TextColor: muted},
					LineEdit{AssignTo: &ui.deviceToken, Text: initial.DeviceToken, PasswordMode: true, CueBanner: "至少 32 个字符", MinSize: Size{Height: 30}},
					CheckBox{AssignTo: &ui.showTokens, Text: "显示令牌", OnCheckedChanged: ui.toggleTokenVisibility},
					Label{AssignTo: &ui.loginStatus, TextColor: walk.RGB(178, 48, 48),
						TextAlignment: AlignCenter, MinSize: Size{Height: 42}},
					PushButton{AssignTo: &ui.connectButton, Text: "建立连接", MinSize: Size{Height: 40}, OnClicked: ui.toggleConnect},
					VSpacer{},
				},
			},
			Composite{
				AssignTo: &ui.connectedPanel, Background: background, Visible: false,
				Layout: VBox{Margins: Margins{Left: 46, Top: 30, Right: 46, Bottom: 28}, Spacing: 10,
					Alignment: AlignHCenterVCenter},
				Children: []Widget{
					Label{Text: "✓", Font: Font{Family: "Segoe UI", PointSize: 30, Bold: true}, TextColor: walk.RGB(34, 180, 105),
						TextAlignment: AlignCenter, MinSize: Size{Height: 66}},
					Label{Text: "已连接", Font: Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true},
						TextColor: dark, TextAlignment: AlignCenter, MinSize: Size{Height: 30}},
					Label{AssignTo: &ui.connectedAddress, TextColor: muted, TextAlignment: AlignCenter, MinSize: Size{Height: 28}},
					Label{AssignTo: &ui.connectedStatus, Text: "连接正常，客户端保持在线", TextColor: walk.RGB(34, 145, 88),
						TextAlignment: AlignCenter, MinSize: Size{Height: 38}},
					VSpacer{Size: 14},
					Composite{Layout: HBox{MarginsZero: true, Spacing: 12}, Children: []Widget{
						PushButton{Text: "中断连接", MinSize: Size{Height: 40}, OnClicked: ui.disconnect},
						PushButton{Text: "最小化到托盘", MinSize: Size{Height: 40}, OnClicked: ui.hideToTray},
					}},
					VSpacer{},
				},
			},
		},
	}
	if err := window.Create(); err != nil {
		return fmt.Errorf("create native Windows client window: %w", err)
	}

	style := uint32(win.GetWindowLong(ui.mainWindow.Handle(), win.GWL_STYLE))
	style &^= win.WS_MAXIMIZEBOX | win.WS_THICKFRAME
	win.SetWindowLong(ui.mainWindow.Handle(), win.GWL_STYLE, int32(style))
	_ = ui.mainWindow.SetIcon(walk.IconApplication())
	ui.mainWindow.Closing().Attach(func(_ *bool, _ walk.CloseReason) {
		ui.emit(clientUIAction{Action: "quit"})
	})

	notifyIcon, err := walk.NewNotifyIcon(ui.mainWindow)
	if err != nil {
		return fmt.Errorf("create notification icon: %w", err)
	}
	ui.notifyIcon = notifyIcon
	if err := notifyIcon.SetIcon(walk.IconApplication()); err != nil {
		return fmt.Errorf("set notification icon: %w", err)
	}
	if err := notifyIcon.SetToolTip("iData Client"); err != nil {
		return fmt.Errorf("set notification tooltip: %w", err)
	}
	showAction := walk.NewAction()
	_ = showAction.SetText("显示主窗口")
	showAction.Triggered().Attach(ui.showWindow)
	exitAction := walk.NewAction()
	_ = exitAction.SetText("退出")
	exitAction.Triggered().Attach(func() { ui.emit(clientUIAction{Action: "quit"}) })
	if err := notifyIcon.ContextMenu().Actions().Add(showAction); err != nil {
		return err
	}
	if err := notifyIcon.ContextMenu().Actions().Add(exitAction); err != nil {
		return err
	}
	notifyIcon.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			ui.showWindow()
		}
	})
	if err := notifyIcon.SetVisible(true); err != nil {
		return fmt.Errorf("show notification icon: %w", err)
	}
	ui.showWindow()
	return nil
}

func initializeWindowsControls() error {
	controls := win.INITCOMMONCONTROLSEX{
		DwSize: uint32(unsafe.Sizeof(win.INITCOMMONCONTROLSEX{})),
		DwICC:  win.ICC_WIN95_CLASSES | win.ICC_STANDARD_CLASSES,
	}
	if !win.InitCommonControlsEx(&controls) {
		return errors.New("initialize Windows Common Controls")
	}
	return nil
}

func (ui *clientUI) emit(action clientUIAction) {
	select {
	case ui.actions <- action:
	default:
		ui.logger.Warn("native UI action queue is full", "action", action.Action)
	}
}

func (ui *clientUI) toggleConnect() {
	if ui.connecting {
		ui.emit(clientUIAction{Action: "disconnect"})
		ui.showLogin()
		return
	}
	ui.beginConnect()
}

func (ui *clientUI) beginConnect() {
	serverURL := strings.TrimSpace(ui.serverURL.Text())
	agentToken := strings.TrimSpace(ui.agentToken.Text())
	deviceToken := strings.TrimSpace(ui.deviceToken.Text())
	if serverURL == "" {
		_ = ui.loginStatus.SetText("请输入 Server URL。")
		return
	}
	if agentToken == "" {
		_ = ui.loginStatus.SetText("请输入 Agent Token。")
		return
	}
	if !validDeviceToken(deviceToken) {
		_ = ui.loginStatus.SetText("Device Token 长度必须为 32–256 个字符。")
		return
	}
	ui.connecting = true
	ui.setConfigurationEnabled(false)
	_ = ui.connectButton.SetText("取消连接")
	ui.loginStatus.SetTextColor(walk.RGB(90, 101, 120))
	_ = ui.loginStatus.SetText("正在连接服务器…")
	ui.emit(clientUIAction{Action: "connect", ServerURL: serverURL, AgentToken: agentToken, DeviceToken: deviceToken})
}

func (ui *clientUI) toggleTokenVisibility() {
	visible := ui.showTokens != nil && ui.showTokens.Checked()
	if ui.agentToken != nil {
		ui.agentToken.SetPasswordMode(!visible)
	}
	if ui.deviceToken != nil {
		ui.deviceToken.SetPasswordMode(!visible)
	}
}

func (ui *clientUI) setConfigurationEnabled(enabled bool) {
	ui.serverURL.SetEnabled(enabled)
	ui.agentToken.SetEnabled(enabled)
	ui.deviceToken.SetEnabled(enabled)
	ui.showTokens.SetEnabled(enabled)
}

func (ui *clientUI) disconnect() {
	ui.emit(clientUIAction{Action: "disconnect"})
	ui.showLogin()
}

func (ui *clientUI) showLogin() {
	ui.connecting = false
	ui.connectedPanel.SetVisible(false)
	ui.loginPanel.SetVisible(true)
	ui.setConfigurationEnabled(true)
	_ = ui.connectButton.SetText("建立连接")
	ui.loginStatus.SetTextColor(walk.RGB(178, 48, 48))
	_ = ui.loginStatus.SetText("")
	_ = ui.serverURL.SetFocus()
}

func (ui *clientUI) showConnected(update clientUIUpdate) {
	ui.connecting = false
	ui.loginPanel.SetVisible(false)
	ui.connectedPanel.SetVisible(true)
	_ = ui.connectedAddress.SetText(update.ServerIP + ":" + update.ServerPort)
	ui.connectedStatus.SetTextColor(walk.RGB(34, 145, 88))
	_ = ui.connectedStatus.SetText("连接正常，客户端保持在线")
}

func (ui *clientUI) hideToTray() {
	ui.mainWindow.Hide()
	if ui.notifyIcon != nil {
		_ = ui.notifyIcon.ShowInfo("iData Client", "客户端仍在后台保持连接。")
	}
}

func (ui *clientUI) showWindow() {
	if ui.mainWindow == nil || ui.mainWindow.Handle() == 0 {
		return
	}
	ui.mainWindow.Show()
	win.ShowWindow(ui.mainWindow.Handle(), win.SW_RESTORE)
	win.SetForegroundWindow(ui.mainWindow.Handle())
	if ui.loginPanel != nil && ui.loginPanel.Visible() {
		_ = ui.serverURL.SetFocus()
	}
}

func (ui *clientUI) update(update clientUIUpdate) error {
	if ui.closed.Load() || ui.mainWindow == nil {
		return errors.New("native Windows UI is closed")
	}
	ui.mainWindow.Synchronize(func() {
		switch update.State {
		case "connected":
			ui.showConnected(update)
		case "retrying":
			if ui.connectedPanel.Visible() {
				ui.connectedStatus.SetTextColor(walk.RGB(202, 121, 0))
				_ = ui.connectedStatus.SetText(update.Message)
			} else {
				ui.loginStatus.SetTextColor(walk.RGB(178, 48, 48))
				_ = ui.loginStatus.SetText(update.Message)
			}
		case "idle":
			ui.showLogin()
		case "error":
			ui.showLogin()
			_ = ui.loginStatus.SetText(update.Message)
		}
	})
	return nil
}

func (ui *clientUI) close() {
	if ui.closed.Load() || ui.mainWindow == nil {
		return
	}
	ui.mainWindow.Synchronize(func() {
		if ui.mainWindow.Handle() != 0 {
			_ = ui.mainWindow.Close()
		}
	})
}
