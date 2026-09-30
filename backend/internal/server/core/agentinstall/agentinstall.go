// Package agentinstall makes the install scripts and the Windows setup
// program that the panel hands out (B30).
package agentinstall

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

//go:embed install.sh.tmpl
var installSh string

//go:embed install.ps1.tmpl
var installPs1 string

//go:embed uninstall.sh
var uninstallSh string

// SetupMarker starts the data appended to setup.exe. The agent looks for the
// last one in the file and reads a JSON object after it.
const SetupMarker = "\nXC-SETUP:"

var (
	serverRe = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~/\-]*)?$|^https?://\[[0-9A-Fa-f:.]+\](:[0-9]{1,5})?$`)
	// CodeRe is the shape of a pairing code as it is normalized: capital
	// letters, digits and a dash.
	CodeRe = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`)
)

// CheckServer allows a panel address that is safe inside a quoted shell or
// PowerShell string.
func CheckServer(server string) error {
	if !serverRe.MatchString(server) || len(server) > 200 {
		return errors.New("面板地址不正确")
	}
	return nil
}

func render(text, server, code string) ([]byte, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if err := CheckServer(server); err != nil {
		return nil, err
	}
	if !CodeRe.MatchString(code) {
		return nil, errors.New("配对码格式不正确")
	}
	t, err := template.New("script").Delims("{{", "}}").Parse(text)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, map[string]string{"Server": server, "Code": code}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Script is install.sh for a panel and a pairing code.
func Script(server, code string) ([]byte, error) { return render(installSh, server, code) }

// PowerShell is install.ps1.
func PowerShell(server, code string) ([]byte, error) { return render(installPs1, server, code) }

// Uninstall is uninstall.sh. It has no parameters.
func Uninstall() []byte { return []byte(strings.ReplaceAll(uninstallSh, "\r\n", "\n")) }

// AppendSetup adds the panel address to the end of a program file.
func AppendSetup(exe []byte, server string) ([]byte, error) {
	if err := CheckServer(server); err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]string{"server": server})
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(exe)+len(SetupMarker)+len(data))
	out = append(out, exe...)
	out = append(out, SetupMarker...)
	return append(out, data...), nil
}

// SetupTrailer is the bytes AppendSetup adds, for streaming the file.
func SetupTrailer(server string) ([]byte, error) {
	out, err := AppendSetup(nil, server)
	if err != nil {
		return nil, fmt.Errorf("setup trailer: %w", err)
	}
	return out, nil
}
