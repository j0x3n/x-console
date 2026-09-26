// Package clipboard reads and writes the text clipboard (clipboard.get,
// clipboard.set). Only the Windows desktop agent supports it: it runs in the
// user's session, while a Linux server agent has no desktop.
package clipboard

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// MaxText caps clipboard.set.
const MaxText = 1 << 20

// Available reports whether this build can use the clipboard.
func Available() bool { return available() }

// Register adds clipboard.get and clipboard.set.
func Register(c *conn.Client) {
	c.Handle(protocol.MethodClipboardGet, func(ctx context.Context, _ json.RawMessage) (any, error) {
		text, err := get()
		if err != nil {
			return nil, err
		}
		return protocol.Clipboard{Text: text}, nil
	})
	c.Handle(protocol.MethodClipboardSet, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.Clipboard
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		text, err := Clean(p.Text)
		if err != nil {
			return nil, err
		}
		return nil, set(text)
	})
}

// Clean checks the size and drops NUL characters, which the Windows
// clipboard format cannot hold. Line endings become CRLF.
func Clean(text string) (string, error) {
	if len(text) > MaxText {
		return "", rpcutil.BadParams("text is larger than 1 MB")
	}
	text = strings.ReplaceAll(text, "\x00", "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n"), nil
}
