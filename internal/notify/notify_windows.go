//go:build windows

package notify

import (
	"encoding/base64"
	"encoding/binary"
	"os/exec"
	"strings"
	"unicode/utf16"

	"netpilot/internal/execx"
)

func available() bool {
	_, err := exec.LookPath("powershell")
	return err == nil
}

func send(title, body string) error {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return err
	}
	script := `
$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$xml = $template.GetXml()
$texts = $xml.GetElementsByTagName('text')
$texts.Item(0).AppendChild($xml.CreateTextNode(` + psQuote(title) + `)) | Out-Null
$texts.Item(1).AppendChild($xml.CreateTextNode(` + psQuote(body) + `)) | Out-Null
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('NetPilot').Show($toast)
`
	encoded := encodePowerShell(script)
	cmd := exec.Command(path, "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded)
	execx.PrepareCommand(cmd) // CREATE_NO_WINDOW: a toast must never show a PowerShell window
	return cmd.Run()
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(buf[i*2:], u)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
