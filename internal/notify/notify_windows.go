//go:build windows

package notify

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"

	"netpilot/internal/branding"
	"netpilot/internal/execx"
	"netpilot/internal/platform"
)

func available() bool {
	_, err := exec.LookPath("powershell.exe")
	return err == nil
}

func send(title, body string) error {
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		return fmt.Errorf("Windows PowerShell is unavailable: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-NoProfile", "-NonInteractive", "-STA", "-EncodedCommand",
		encodePowerShell(notificationScript(title, body)))
	execx.PrepareCommand(cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("Windows notification service timed out")
		}
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return fmt.Errorf("Windows notification: %s", detail)
		}
		return fmt.Errorf("Windows notification: %w", err)
	}
	return nil
}

func notificationScript(title, body string) string {
	var payload strings.Builder
	payload.WriteString(`<toast><visual><binding template="ToastGeneric"><text>`)
	_ = xml.EscapeText(&payload, []byte(title))
	payload.WriteString(`</text><text>`)
	_ = xml.EscapeText(&payload, []byte(body))
	payload.WriteString(`</text></binding></visual></toast>`)
	encode := func(value string) string { return "'" + base64.StdEncoding.EncodeToString([]byte(value)) + "'" }

	return `
$ErrorActionPreference = 'Stop'
function Decode-Text([string] $value) {
    [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($value))
}
try {
    $appID = Decode-Text ` + encode(branding.AppID) + `
    $registered = @()
    if (Get-Command Get-StartApps -ErrorAction SilentlyContinue) {
        $registered = @(Get-StartApps -ErrorAction SilentlyContinue | Where-Object { $_.AppID -eq $appID })
    }
    if ($registered.Count -gt 0) {
        $null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
        $null = [Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime]
        $null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]
        $notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($appID)
        if ($null -eq $notifier) { throw 'Windows could not open the NetPilot toast service.' }
        $setting = [string] $notifier.Setting
        if ($setting -ne 'Enabled' -and $setting -ne '0') {
            if ([string]::IsNullOrEmpty($setting)) { throw 'Windows did not report the NetPilot notification permission.' }
            throw "Windows blocks NetPilot notifications ($setting). Check System notification settings."
        }
        $document = [Windows.Data.Xml.Dom.XmlDocument]::new()
        $document.LoadXml((Decode-Text ` + encode(payload.String()) + `))
        $toast = [Windows.UI.Notifications.ToastNotification]::new($document)
        $notifier.Show($toast)
        return
    }

    # A portable copy has no registered Start menu identity. Deliver through the
    # Windows notification area without writing a shortcut or registry entry.
    if (-not [Environment]::UserInteractive) { throw 'Open NetPilot from your Windows desktop session to show a notification.' }
    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing
    $icon = [System.Windows.Forms.NotifyIcon]::new()
    try {
        $icon.Icon = [System.Drawing.SystemIcons]::Information
        $icon.Text = 'NetPilot'
        $icon.Visible = $true
        $icon.ShowBalloonTip(5000, (Decode-Text ` + encode(title) + `), (Decode-Text ` + encode(body) + `), [System.Windows.Forms.ToolTipIcon]::Info)
        Start-Sleep -Milliseconds 5000
    } finally {
        $icon.Dispose()
    }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`
}

func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(buf[i*2:], u)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func openSettings() error { return platform.OpenURL("ms-settings:notifications") }
