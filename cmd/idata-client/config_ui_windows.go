//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

type clientUIInitial struct {
	ServerIP    string `json:"server_ip"`
	ServerPort  string `json:"server_port"`
	AutoConnect bool   `json:"auto_connect"`
	SetupError  string `json:"setup_error,omitempty"`
}

type clientUIAction struct {
	Action     string `json:"action"`
	ServerIP   string `json:"server_ip,omitempty"`
	ServerPort string `json:"server_port,omitempty"`
}

type clientUIUpdate struct {
	State      string `json:"state"`
	ServerIP   string `json:"server_ip,omitempty"`
	ServerPort string `json:"server_port,omitempty"`
	Message    string `json:"message,omitempty"`
}

type clientUI struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	actions  chan clientUIAction
	done     chan error
	readDone chan struct{}
	logger   *slog.Logger
	mu       sync.Mutex
}

func startClientUI(initial clientUIInitial, logger *slog.Logger, logFile *os.File) (*clientUI, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", clientWindowScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create client window input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("create client window output: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("create client window error output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open client window: %w", err)
	}
	logCheckpoint(logger, logFile, "client UI process started", "pid", cmd.Process.Pid)

	ui := &clientUI{
		cmd: cmd, stdin: stdin, actions: make(chan clientUIAction, 8),
		done: make(chan error, 1), readDone: make(chan struct{}), logger: logger,
	}
	if err := ui.send(initial); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	logCheckpoint(logger, logFile, "initial UI configuration sent")
	go ui.readActions(stdout)
	go func() {
		message, _ := io.ReadAll(io.LimitReader(stderr, 64<<10))
		err := cmd.Wait()
		<-ui.readDone
		if len(message) > 0 {
			err = fmt.Errorf("client window stopped (%v): %s", err, message)
		}
		logger.Info("client UI process exited", "pid", cmd.Process.Pid, "error", err)
		ui.done <- err
		close(ui.done)
	}()
	return ui, nil
}

func (ui *clientUI) readActions(reader io.Reader) {
	defer close(ui.actions)
	defer close(ui.readDone)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var action clientUIAction
		if json.Unmarshal(scanner.Bytes(), &action) == nil && action.Action != "" {
			ui.actions <- action
		} else {
			output := strings.TrimSpace(scanner.Text())
			if len(output) > 1024 {
				output = output[:1024] + "…"
			}
			ui.logger.Warn("unexpected client UI output", "output", output)
		}
	}
	if err := scanner.Err(); err != nil {
		ui.logger.Warn("reading client UI output failed", "error", err)
	}
}

func (ui *clientUI) send(value any) error {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := ui.stdin.Write(encoded); err != nil {
		return fmt.Errorf("update client window: %w", err)
	}
	return nil
}

func (ui *clientUI) update(update clientUIUpdate) error { return ui.send(update) }
func (ui *clientUI) close()                             { _ = ui.stdin.Close() }

const clientWindowScript = `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()
$initialLine = [Console]::In.ReadLine()
if ([string]::IsNullOrWhiteSpace($initialLine)) { exit 1 }
$initial = $initialLine | ConvertFrom-Json

$form = New-Object System.Windows.Forms.Form
$form.Text = 'iData Client'
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedSingle'
$form.MaximizeBox = $false
$form.MinimizeBox = $true
$form.ClientSize = New-Object System.Drawing.Size(390, 500)
$form.BackColor = [System.Drawing.Color]::FromArgb(246, 248, 252)
$form.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 9)

$brand = New-Object System.Windows.Forms.Panel
$brand.Dock = 'Top'
$brand.Height = 142
$brand.BackColor = [System.Drawing.Color]::FromArgb(18, 113, 232)
$form.Controls.Add($brand)
$logo = New-Object System.Windows.Forms.Label
$logo.Text = 'iD'
$logo.TextAlign = 'MiddleCenter'
$logo.Font = New-Object System.Drawing.Font('Segoe UI', 24, [System.Drawing.FontStyle]::Bold)
$logo.ForeColor = [System.Drawing.Color]::White
$logo.BackColor = [System.Drawing.Color]::FromArgb(39, 133, 243)
$logo.Location = New-Object System.Drawing.Point(160, 22)
$logo.Size = New-Object System.Drawing.Size(70, 70)
$brand.Controls.Add($logo)
$title = New-Object System.Windows.Forms.Label
$title.Text = 'iData Client'
$title.TextAlign = 'MiddleCenter'
$title.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 12, [System.Drawing.FontStyle]::Bold)
$title.ForeColor = [System.Drawing.Color]::White
$title.Location = New-Object System.Drawing.Point(90, 100)
$title.Size = New-Object System.Drawing.Size(210, 28)
$brand.Controls.Add($title)

$loginPanel = New-Object System.Windows.Forms.Panel
$loginPanel.Location = New-Object System.Drawing.Point(0, 142)
$loginPanel.Size = New-Object System.Drawing.Size(390, 358)
$loginPanel.BackColor = $form.BackColor
$form.Controls.Add($loginPanel)
$hint = New-Object System.Windows.Forms.Label
$hint.Text = '连接到管理服务器'
$hint.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 11, [System.Drawing.FontStyle]::Bold)
$hint.ForeColor = [System.Drawing.Color]::FromArgb(45, 55, 72)
$hint.Location = New-Object System.Drawing.Point(48, 30)
$hint.Size = New-Object System.Drawing.Size(294, 28)
$loginPanel.Controls.Add($hint)

function Add-FieldLabel($text, $y) {
  $label = New-Object System.Windows.Forms.Label
  $label.Text = $text
  $label.ForeColor = [System.Drawing.Color]::FromArgb(90, 101, 120)
  $label.Location = New-Object System.Drawing.Point(48, $y)
  $label.Size = New-Object System.Drawing.Size(294, 22)
  $loginPanel.Controls.Add($label)
}
function Add-Field($text, $y) {
  $box = New-Object System.Windows.Forms.TextBox
  $box.Text = [string]$text
  $box.BorderStyle = 'FixedSingle'
  $box.Font = New-Object System.Drawing.Font('Segoe UI', 11)
  $box.Location = New-Object System.Drawing.Point(48, $y)
  $box.Size = New-Object System.Drawing.Size(294, 30)
  $loginPanel.Controls.Add($box)
  return $box
}
Add-FieldLabel '服务器 IP' 76
$serverIP = Add-Field $initial.server_ip 100
Add-FieldLabel '端口' 150
$serverPort = Add-Field $initial.server_port 174
$loginStatus = New-Object System.Windows.Forms.Label
$loginStatus.TextAlign = 'MiddleCenter'
$loginStatus.ForeColor = [System.Drawing.Color]::Firebrick
$loginStatus.Location = New-Object System.Drawing.Point(38, 216)
$loginStatus.Size = New-Object System.Drawing.Size(314, 44)
$loginPanel.Controls.Add($loginStatus)
$connect = New-Object System.Windows.Forms.Button
$connect.Text = '建立连接'
$connect.FlatStyle = 'Flat'
$connect.FlatAppearance.BorderSize = 0
$connect.BackColor = [System.Drawing.Color]::FromArgb(18, 113, 232)
$connect.ForeColor = [System.Drawing.Color]::White
$connect.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 10, [System.Drawing.FontStyle]::Bold)
$connect.Location = New-Object System.Drawing.Point(48, 274)
$connect.Size = New-Object System.Drawing.Size(294, 42)
$loginPanel.Controls.Add($connect)
$form.AcceptButton = $connect

$connectedPanel = New-Object System.Windows.Forms.Panel
$connectedPanel.Location = New-Object System.Drawing.Point(0, 142)
$connectedPanel.Size = New-Object System.Drawing.Size(390, 358)
$connectedPanel.BackColor = $form.BackColor
$connectedPanel.Visible = $false
$form.Controls.Add($connectedPanel)
$okIcon = New-Object System.Windows.Forms.Label
$okIcon.Text = [char]0x2713
$okIcon.TextAlign = 'MiddleCenter'
$okIcon.Font = New-Object System.Drawing.Font('Segoe UI', 26, [System.Drawing.FontStyle]::Bold)
$okIcon.ForeColor = [System.Drawing.Color]::White
$okIcon.BackColor = [System.Drawing.Color]::FromArgb(34, 180, 105)
$okIcon.Location = New-Object System.Drawing.Point(160, 28)
$okIcon.Size = New-Object System.Drawing.Size(70, 70)
$connectedPanel.Controls.Add($okIcon)
$connectedTitle = New-Object System.Windows.Forms.Label
$connectedTitle.Text = '已连接'
$connectedTitle.TextAlign = 'MiddleCenter'
$connectedTitle.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 13, [System.Drawing.FontStyle]::Bold)
$connectedTitle.ForeColor = [System.Drawing.Color]::FromArgb(38, 49, 66)
$connectedTitle.Location = New-Object System.Drawing.Point(50, 112)
$connectedTitle.Size = New-Object System.Drawing.Size(290, 30)
$connectedPanel.Controls.Add($connectedTitle)
$connectedAddress = New-Object System.Windows.Forms.Label
$connectedAddress.TextAlign = 'MiddleCenter'
$connectedAddress.ForeColor = [System.Drawing.Color]::FromArgb(95, 106, 124)
$connectedAddress.Location = New-Object System.Drawing.Point(35, 148)
$connectedAddress.Size = New-Object System.Drawing.Size(320, 24)
$connectedPanel.Controls.Add($connectedAddress)
$connectedStatus = New-Object System.Windows.Forms.Label
$connectedStatus.Text = '连接正常，客户端保持在线'
$connectedStatus.TextAlign = 'MiddleCenter'
$connectedStatus.ForeColor = [System.Drawing.Color]::FromArgb(34, 145, 88)
$connectedStatus.Location = New-Object System.Drawing.Point(35, 178)
$connectedStatus.Size = New-Object System.Drawing.Size(320, 38)
$connectedPanel.Controls.Add($connectedStatus)
$disconnect = New-Object System.Windows.Forms.Button
$disconnect.Text = '中断连接'
$disconnect.FlatStyle = 'Flat'
$disconnect.FlatAppearance.BorderColor = [System.Drawing.Color]::FromArgb(210, 216, 226)
$disconnect.BackColor = [System.Drawing.Color]::White
$disconnect.ForeColor = [System.Drawing.Color]::FromArgb(55, 65, 81)
$disconnect.Location = New-Object System.Drawing.Point(48, 232)
$disconnect.Size = New-Object System.Drawing.Size(140, 42)
$connectedPanel.Controls.Add($disconnect)
$toTray = New-Object System.Windows.Forms.Button
$toTray.Text = '最小化到托盘'
$toTray.FlatStyle = 'Flat'
$toTray.FlatAppearance.BorderSize = 0
$toTray.BackColor = [System.Drawing.Color]::FromArgb(18, 113, 232)
$toTray.ForeColor = [System.Drawing.Color]::White
$toTray.Location = New-Object System.Drawing.Point(202, 232)
$toTray.Size = New-Object System.Drawing.Size(140, 42)
$connectedPanel.Controls.Add($toTray)

$tray = New-Object System.Windows.Forms.NotifyIcon
$tray.Text = 'iData Client'
$tray.Icon = [System.Drawing.SystemIcons]::Application
$tray.Visible = $true
$menu = New-Object System.Windows.Forms.ContextMenuStrip
$showItem = $menu.Items.Add('显示主窗口')
$exitItem = $menu.Items.Add('退出')
$tray.ContextMenuStrip = $menu

function Send-Action($payload) {
  [Console]::Out.WriteLine(($payload | ConvertTo-Json -Compress))
  [Console]::Out.Flush()
}
function Show-Login {
  $connectedPanel.Visible = $false
  $loginPanel.Visible = $true
  $serverIP.Enabled = $true
  $serverPort.Enabled = $true
  $connect.Enabled = $true
  $connect.Text = '建立连接'
  $loginStatus.Text = ''
  $serverIP.Focus()
}
function Start-Connect {
  if ([string]::IsNullOrWhiteSpace($serverIP.Text) -or [string]::IsNullOrWhiteSpace($serverPort.Text)) {
    $loginStatus.Text = '请输入服务器 IP 和端口。'
    return
  }
  if (-not [string]::IsNullOrWhiteSpace([string]$initial.setup_error)) {
    $loginStatus.Text = [string]$initial.setup_error
    return
  }
  $serverIP.Enabled = $false
  $serverPort.Enabled = $false
  $connect.Text = '取消连接'
  $loginStatus.ForeColor = [System.Drawing.Color]::FromArgb(90, 101, 120)
  $loginStatus.Text = '正在连接服务器…'
  Send-Action ([pscustomobject]@{ action='connect'; server_ip=$serverIP.Text.Trim(); server_port=$serverPort.Text.Trim() })
}
$connect.Add_Click({
  if ($connect.Text -eq '取消连接') {
    Send-Action ([pscustomobject]@{ action='disconnect' })
    Show-Login
  } else { Start-Connect }
})
$disconnect.Add_Click({ Send-Action ([pscustomobject]@{ action='disconnect' }); Show-Login })
$toTray.Add_Click({ $form.Hide(); $tray.ShowBalloonTip(1500, 'iData Client', '客户端仍在后台保持连接。', 'Info') })
$showItem.Add_Click({ $form.Show(); $form.WindowState = 'Normal'; $form.Activate() })
$tray.Add_DoubleClick({ $form.Show(); $form.WindowState = 'Normal'; $form.Activate() })
$exitItem.Add_Click({ Send-Action ([pscustomobject]@{ action='quit' }); $form.Close() })
$form.Add_FormClosing({ Send-Action ([pscustomobject]@{ action='quit' }); $tray.Visible = $false })

$script:pendingRead = [Console]::In.ReadLineAsync()
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 100
$timer.Add_Tick({
  if (-not $script:pendingRead.IsCompleted) { return }
  $line = $script:pendingRead.Result
  if ($null -eq $line) { $form.Close(); return }
  try {
    $update = $line | ConvertFrom-Json
    switch ([string]$update.state) {
      'connected' {
        $loginPanel.Visible = $false
        $connectedPanel.Visible = $true
        $connectedAddress.Text = ([string]$update.server_ip) + ':' + ([string]$update.server_port)
        $connectedStatus.ForeColor = [System.Drawing.Color]::FromArgb(34, 145, 88)
        $connectedStatus.Text = '连接正常，客户端保持在线'
      }
      'retrying' {
        if ($connectedPanel.Visible) {
          $connectedStatus.ForeColor = [System.Drawing.Color]::DarkOrange
          $connectedStatus.Text = [string]$update.message
        } else {
          $loginStatus.ForeColor = [System.Drawing.Color]::Firebrick
          $loginStatus.Text = [string]$update.message
        }
      }
      'idle' { Show-Login }
      'error' {
        $loginStatus.ForeColor = [System.Drawing.Color]::Firebrick
        $loginStatus.Text = [string]$update.message
        $serverIP.Enabled = $true
        $serverPort.Enabled = $true
        $connect.Enabled = $true
        $connect.Text = '建立连接'
      }
    }
  } catch {}
  $script:pendingRead = [Console]::In.ReadLineAsync()
})
$timer.Start()
$form.Add_Shown({
  Send-Action ([pscustomobject]@{ action='ready' })
  if (-not [string]::IsNullOrWhiteSpace([string]$initial.setup_error)) {
    $loginStatus.Text = [string]$initial.setup_error
  } elseif ([bool]$initial.auto_connect -and -not [string]::IsNullOrWhiteSpace($serverIP.Text) -and -not [string]::IsNullOrWhiteSpace($serverPort.Text)) {
    Start-Connect
  }
})
[void][System.Windows.Forms.Application]::Run($form)
$tray.Dispose()
`
