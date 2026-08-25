package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

func promptForConfig(current clientFileConfig) (clientFileConfig, bool, error) {
	input, err := json.Marshal(current)
	if err != nil {
		return clientFileConfig{}, false, fmt.Errorf("encode config defaults: %w", err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", configDialogScript)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stdout.String() == "cancelled" {
			return current, false, nil
		}
		if stderr.Len() > 0 {
			return clientFileConfig{}, false, fmt.Errorf("open config window: %s", stderr.String())
		}
		return clientFileConfig{}, false, fmt.Errorf("open config window: %w", err)
	}
	if stdout.String() == "cancelled" {
		return current, false, nil
	}
	var config clientFileConfig
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return clientFileConfig{}, false, fmt.Errorf("read config window result: %w", err)
	}
	if config.ServerURL == "" || config.AgentToken == "" {
		return current, false, errors.New("server URL and agent token are required")
	}
	return config, true, nil
}

const configDialogScript = `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$json = [Console]::In.ReadToEnd()
if ([string]::IsNullOrWhiteSpace($json)) {
  $config = [pscustomobject]@{}
} else {
  $config = $json | ConvertFrom-Json
}

$form = New-Object System.Windows.Forms.Form
$form.Text = 'idata-client configuration'
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.ClientSize = New-Object System.Drawing.Size(520, 285)

$font = New-Object System.Drawing.Font('Segoe UI', 9)
$form.Font = $font

function Add-Label($text, $x, $y) {
  $label = New-Object System.Windows.Forms.Label
  $label.Text = $text
  $label.Location = New-Object System.Drawing.Point($x, $y)
  $label.Size = New-Object System.Drawing.Size(130, 24)
  $form.Controls.Add($label)
}

function Add-TextBox($x, $y, $text, $password) {
  $box = New-Object System.Windows.Forms.TextBox
  $box.Location = New-Object System.Drawing.Point($x, $y)
  $box.Size = New-Object System.Drawing.Size(330, 24)
  $box.Text = [string]$text
  $box.UseSystemPasswordChar = $password
  $form.Controls.Add($box)
  return $box
}

Add-Label 'Server URL' 24 28
$server = Add-TextBox 155 25 $config.server_url $false
Add-Label 'Agent token' 24 72
$token = Add-TextBox 155 69 $config.agent_token $true
Add-Label 'Client ID' 24 116
$client = Add-TextBox 155 113 $config.client_id $false

$allow = New-Object System.Windows.Forms.CheckBox
$allow.Text = 'Allow ws:// for non-local server'
$allow.Location = New-Object System.Drawing.Point(155, 153)
$allow.Size = New-Object System.Drawing.Size(260, 24)
$allow.Checked = if ($null -eq $config.allow_insecure) { $true } else { [bool]$config.allow_insecure }
$form.Controls.Add($allow)

$status = New-Object System.Windows.Forms.Label
$status.ForeColor = [System.Drawing.Color]::Firebrick
$status.Location = New-Object System.Drawing.Point(24, 190)
$status.Size = New-Object System.Drawing.Size(470, 24)
$form.Controls.Add($status)

$save = New-Object System.Windows.Forms.Button
$save.Text = 'Save and start'
$save.Location = New-Object System.Drawing.Point(260, 230)
$save.Size = New-Object System.Drawing.Size(115, 30)
$save.Add_Click({
  if ([string]::IsNullOrWhiteSpace($server.Text) -or [string]::IsNullOrWhiteSpace($token.Text)) {
    $status.Text = 'Server URL and agent token are required.'
    return
  }
  $form.DialogResult = [System.Windows.Forms.DialogResult]::OK
  $form.Close()
})
$form.Controls.Add($save)

$cancel = New-Object System.Windows.Forms.Button
$cancel.Text = 'Cancel'
$cancel.Location = New-Object System.Drawing.Point(390, 230)
$cancel.Size = New-Object System.Drawing.Size(85, 30)
$cancel.Add_Click({
  $form.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
  $form.Close()
})
$form.Controls.Add($cancel)
$form.AcceptButton = $save
$form.CancelButton = $cancel

$result = $form.ShowDialog()
if ($result -ne [System.Windows.Forms.DialogResult]::OK) {
  [Console]::Out.Write('cancelled')
  exit 1
}

$output = [pscustomobject]@{
  server_url = $server.Text.Trim()
  agent_token = $token.Text
  client_id = $client.Text.Trim()
  allow_insecure = [bool]$allow.Checked
}
$output | ConvertTo-Json -Compress
`
