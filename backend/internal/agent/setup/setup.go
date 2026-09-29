// Package setup is the install mode of the Windows agent (B30). When the
// program is started by double click under the name the panel gave it,
// x-console-agent-setup-ABCD-EFGH.exe, it installs itself, pairs with the
// panel and starts. The panel address is written after the program bytes.
package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Marker starts the data the panel appends to the program. It must be the
// same text as agentinstall.SetupMarker in the server.
const Marker = "\nXC-SETUP:"

// tailSize is how much of the end of the file is searched for the marker.
const tailSize = 4096

// nameRe matches x-console-agent-setup.exe and x-console-agent-setup-ABCD-EFGH.exe,
// with the " (1)" a browser adds to a second download.
var nameRe = regexp.MustCompile(`(?i)^x-console-agent-setup(?:-([A-Z0-9]{4}-[A-Z0-9]{4}))?(?: \(\d+\))?\.exe$`)

// ParseName tells whether a program path is the setup program, and which
// pairing code its name carries (empty when it has none).
func ParseName(path string) (code string, isSetup bool) {
	base := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		base = path[i+1:]
	}
	m := nameRe.FindStringSubmatch(base)
	if m == nil {
		return "", false
	}
	return strings.ToUpper(m[1]), true
}

// Trailer is what the panel appended.
type Trailer struct {
	Server string
	// Size is the length of the program without the appended data.
	Size int64
}

// ReadTrailer looks for the appended data at the end of a file.
func ReadTrailer(path string) (Trailer, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return Trailer{}, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Trailer{}, false, err
	}
	start := max(0, st.Size()-tailSize)
	buf := make([]byte, st.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return Trailer{}, false, err
	}
	i := bytes.LastIndex(buf, []byte(Marker))
	if i < 0 {
		return Trailer{}, false, nil
	}
	var data struct {
		Server string `json:"server"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf[i+len(Marker):]), &data); err != nil || !validServer(data.Server) {
		return Trailer{}, false, nil
	}
	return Trailer{Server: data.Server, Size: start + int64(i)}, true, nil
}

func validServer(s string) bool {
	return (strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")) && len(s) < 300 && !strings.ContainsAny(s, " \r\n\"'")
}

// CopyProgram copies the program to dst without the appended data. It
// writes to a new file first and then replaces dst.
func CopyProgram(src, dst string) error {
	size := int64(-1)
	if t, ok, err := ReadTrailer(src); err == nil && ok {
		size = t.Size
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	var r io.Reader = in
	if size >= 0 {
		r = io.LimitReader(in, size)
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		// Windows does not replace a running program; remove first.
		if rerr := os.Remove(dst); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, dst)
	}
	return nil
}

// Options are what Run needs from the caller.
type Options struct {
	// Pair pairs with the panel and saves the agent's config file.
	Pair func(server, code string) error
	// ConfigPath is the config file; when it exists the agent is already
	// paired and only the program is replaced.
	ConfigPath string
}
