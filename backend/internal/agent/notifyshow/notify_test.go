package notifyshow

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
)

func TestStructuredNotificationArguments(t *testing.T) {
	in := Input{Title: `a"; do shell script "touch /tmp/pwn"`, Body: "body $(evil) `evil`\n<&>"}
	name, args, env, err := commandFor("darwin", in)
	if err != nil || name != "osascript" || len(args) != 5 || args[3] != in.Title || args[4] != in.Body || strings.Contains(args[1], in.Title) {
		t.Fatalf("mac command: %s %q %v", name, args, err)
	}
	name, args, _, err = commandFor("linux", in)
	if err != nil || name != "notify-send" || args[len(args)-2] != in.Title || args[len(args)-1] != in.Body {
		t.Fatalf("linux command: %s %q %v", name, args, err)
	}
	name, args, env, err = commandFor("windows", in)
	if err != nil || name != "powershell.exe" || strings.Contains(strings.Join(args, " "), in.Title) || len(env) != 2 || !strings.Contains(env[0], "XC_NOTIFY_XML=") {
		t.Fatalf("windows command: %s %q %q %v", name, args, env, err)
	}
}

func TestWindowsToastEscapesData(t *testing.T) {
	in := Input{Title: `</text><script>$(evil)</script>`, Body: "a & b <c>\n中文"}
	_, _, env, err := commandFor("windows", in)
	if err != nil {
		t.Fatal(err)
	}
	var toast struct {
		Text []string `xml:"visual>binding>text"`
	}
	if err := xml.Unmarshal([]byte(strings.TrimPrefix(env[0], "XC_NOTIFY_XML=")), &toast); err != nil {
		t.Fatal(err)
	}
	if len(toast.Text) != 2 || toast.Text[0] != in.Title || toast.Text[1] != in.Body {
		t.Fatalf("toast: %+v", toast)
	}
}

func TestRunnerFailureAndUnsupported(t *testing.T) {
	in := Input{Title: "title"}
	if err := showWith(context.Background(), "linux", in, func(context.Context, string, []string, []string) error { return errors.New("failed") }); err == nil {
		t.Fatal("runner error ignored")
	}
	if _, _, _, err := commandFor("unknown", in); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestNotificationValidationAndRunner(t *testing.T) {
	for _, in := range []Input{{}, {Title: "x\x00y"}, {Title: strings.Repeat("x", 201)}, {Title: "valid", Body: strings.Repeat("x", 4001)}} {
		if validate(in) == nil {
			t.Fatalf("accepted: %+v", in)
		}
	}
	called := false
	err := showWith(context.Background(), "linux", Input{Title: "title", Body: "body"}, func(_ context.Context, name string, args, env []string) error {
		called = true
		if name != "notify-send" || len(env) != 0 {
			t.Fatal("runner")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("show: %v %v", called, err)
	}
}
