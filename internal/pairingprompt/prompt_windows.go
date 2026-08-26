//go:build windows

package pairingprompt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func Confirm(ctx context.Context, request Request) (bool, error) {
	input, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("encode pairing request: %w", err)
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", pairingDialogScript)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, fmt.Errorf("show pairing confirmation: %w", err)
	}
	switch strings.TrimSpace(stdout.String()) {
	case "approved":
		return true, nil
	case "denied":
		return false, nil
	default:
		return false, errors.New("pairing confirmation returned an invalid result")
	}
}

const pairingDialogScript = `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$json = [Console]::In.ReadToEnd()
$request = $json | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace([string]$request.challenge)) {
  [Console]::Out.Write('denied')
  exit 0
}

$ttl = [TimeSpan]::FromSeconds([double]$request.session_ttl_seconds)
if ($ttl.TotalHours -ge 1) {
  $duration = ('{0:0.#} 小时' -f $ttl.TotalHours)
} else {
  $duration = ('{0:0} 分钟' -f [Math]::Max(1, $ttl.TotalMinutes))
}

$form = New-Object System.Windows.Forms.Form
$form.Text = 'iData 本机浏览器授权'
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.TopMost = $true
$form.ClientSize = New-Object System.Drawing.Size(660, 470)
$form.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 9)

$title = New-Object System.Windows.Forms.Label
$title.Text = '有浏览器请求控制这台电脑'
$title.Location = New-Object System.Drawing.Point(28, 24)
$title.Size = New-Object System.Drawing.Size(600, 32)
$title.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 15, [System.Drawing.FontStyle]::Bold)
$form.Controls.Add($title)

$warning = New-Object System.Windows.Forms.Label
$warning.Text = "只有当你本人刚刚在浏览器中点击了“请求此电脑授权”时才继续。" + [Environment]::NewLine + "确认后，发起请求的浏览器将在最多 $duration 内免登录访问本机终端。" + [Environment]::NewLine + "如果这不是你发起的请求，请立即点击“拒绝”。"
$warning.Location = New-Object System.Drawing.Point(30, 72)
$warning.Size = New-Object System.Drawing.Size(600, 78)
$warning.ForeColor = [System.Drawing.Color]::Firebrick
$form.Controls.Add($warning)

$details = New-Object System.Windows.Forms.Label
$details.Text = "请求来源 IP：$($request.browser_ip)" + [Environment]::NewLine + "Server：$($request.server_host)"
$details.Location = New-Object System.Drawing.Point(30, 164)
$details.Size = New-Object System.Drawing.Size(600, 54)
$form.Controls.Add($details)

$instruction = New-Object System.Windows.Forms.Label
$instruction.Text = '请手动输入下面完整的确认文本（区分大小写）：'
$instruction.Location = New-Object System.Drawing.Point(30, 232)
$instruction.Size = New-Object System.Drawing.Size(600, 24)
$form.Controls.Add($instruction)

$challenge = New-Object System.Windows.Forms.Label
$challenge.Text = [string]$request.challenge
$challenge.Location = New-Object System.Drawing.Point(30, 263)
$challenge.Size = New-Object System.Drawing.Size(600, 38)
$challenge.Font = New-Object System.Drawing.Font('Consolas', 16, [System.Drawing.FontStyle]::Bold)
$challenge.ForeColor = [System.Drawing.Color]::DarkOrange
$form.Controls.Add($challenge)

$typed = New-Object System.Windows.Forms.TextBox
$typed.Location = New-Object System.Drawing.Point(30, 316)
$typed.Size = New-Object System.Drawing.Size(600, 30)
$typed.Font = New-Object System.Drawing.Font('Consolas', 12)
$form.Controls.Add($typed)

$status = New-Object System.Windows.Forms.Label
$status.Text = '确认按钮会在输入完全一致后启用。'
$status.Location = New-Object System.Drawing.Point(30, 356)
$status.Size = New-Object System.Drawing.Size(600, 24)
$status.ForeColor = [System.Drawing.Color]::DimGray
$form.Controls.Add($status)

$approve = New-Object System.Windows.Forms.Button
$approve.Text = '确认授权这台浏览器'
$approve.Location = New-Object System.Drawing.Point(360, 414)
$approve.Size = New-Object System.Drawing.Size(165, 32)
$approve.Enabled = $false
$approve.Add_Click({
  if ([string]::Equals($typed.Text, [string]$request.challenge, [System.StringComparison]::Ordinal)) {
    $form.DialogResult = [System.Windows.Forms.DialogResult]::OK
    $form.Close()
  }
})
$form.Controls.Add($approve)

$deny = New-Object System.Windows.Forms.Button
$deny.Text = '拒绝'
$deny.Location = New-Object System.Drawing.Point(540, 414)
$deny.Size = New-Object System.Drawing.Size(90, 32)
$deny.Add_Click({
  $form.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
  $form.Close()
})
$form.Controls.Add($deny)
$form.CancelButton = $deny

$typed.Add_TextChanged({
  $approve.Enabled = [string]::Equals($typed.Text, [string]$request.challenge, [System.StringComparison]::Ordinal)
})
$form.Add_Shown({ $typed.Focus() })

$result = $form.ShowDialog()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
  [Console]::Out.Write('approved')
} else {
  [Console]::Out.Write('denied')
}
`
