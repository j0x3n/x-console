package notifyshow

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type Input = protocol.NotifyShowParams
type commandRunner func(context.Context, string, []string, []string) error

func Register(c *conn.Client) {
	register(c, Show)
}

func register(c *conn.Client, show func(context.Context, Input) error) {
	c.Handle(protocol.MethodNotifyShow, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in Input
		if err := rpcutil.Decode(raw, &in); err != nil {
			return nil, err
		}
		if err := validate(in); err != nil {
			return nil, err
		}
		if err := show(ctx, in); err != nil {
			return nil, err
		}
		return map[string]bool{"shown": true}, nil
	})
}

func validate(in Input) error {
	if strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 200 || utf8.RuneCountInString(in.Body) > 4000 || strings.ContainsRune(in.Title, '\x00') || strings.ContainsRune(in.Body, '\x00') || !utf8.ValidString(in.Title) || !utf8.ValidString(in.Body) {
		return rpcutil.BadParams("notification text is invalid")
	}
	return nil
}

func commandFor(platform string, in Input) (string, []string, []string, error) {
	if err := validate(in); err != nil {
		return "", nil, nil, err
	}
	switch platform {
	case "linux":
		return "notify-send", []string{"--app-name=X Console", "--", in.Title, in.Body}, nil, nil
	case "darwin":
		return "osascript", []string{"-e", "on run argv\ndisplay notification (item 2 of argv) with title (item 1 of argv)\nend run", "--", in.Title, in.Body}, nil, nil
	case "windows":
		var title, body bytes.Buffer
		if err := xml.EscapeText(&title, []byte(in.Title)); err != nil {
			return "", nil, nil, err
		}
		if err := xml.EscapeText(&body, []byte(in.Body)); err != nil {
			return "", nil, nil, err
		}
		toast := `<toast><visual><binding template="ToastGeneric"><text>` + title.String() + `</text><text>` + body.String() + `</text></binding></visual></toast>`
		return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", windowsToast}, []string{"XC_NOTIFY_XML=" + toast, "XC_NOTIFY_APP=XConsole.Agent"}, nil
	default:
		return "", nil, nil, rpcutil.Unsupported("system notification")
	}
}

const windowsToast = `$ErrorActionPreference='Stop'; $app='HKCU:\Software\Classes\AppUserModelId\XConsole.Agent'; New-Item -Path $app -Force > $null; New-ItemProperty -Path $app -Name DisplayName -Value 'X Console' -PropertyType String -Force > $null; [Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime] > $null; [Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom.XmlDocument,ContentType=WindowsRuntime] > $null; $xml=New-Object Windows.Data.Xml.Dom.XmlDocument; $xml.LoadXml($env:XC_NOTIFY_XML); $toast=[Windows.UI.Notifications.ToastNotification]::new($xml); [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($env:XC_NOTIFY_APP).Show($toast)`

func showWith(ctx context.Context, platform string, in Input, run commandRunner) error {
	name, args, env, err := commandFor(platform, in)
	if err != nil {
		return err
	}
	if err := run(ctx, name, args, env); err != nil {
		return rpcutil.Failed("system notification failed: %v", err)
	}
	return nil
}

func Show(ctx context.Context, in Input) error {
	if !Available() {
		return rpcutil.Unsupported("system notification in this session")
	}
	return showWith(ctx, runtime.GOOS, in, func(ctx context.Context, name string, args, env []string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = append(os.Environ(), env...)
		prepareCommand(cmd)
		return cmd.Run()
	})
}
