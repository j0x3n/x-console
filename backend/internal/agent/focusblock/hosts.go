package focusblock

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// The agent only ever touches the lines between these two markers. Everything
// else in the hosts file stays as it was.
const (
	beginMarker = "# >>> x-console focus block"
	endMarker   = "# <<< x-console focus block"
)

// DefaultHostsPath is the hosts file of this system.
func DefaultHostsPath() string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return filepath.Join(root, "System32", "drivers", "etc", "hosts")
	}
	return "/etc/hosts"
}

// splitLines cuts content into lines that keep their line ending.
func splitLines(content string) []string {
	var out []string
	for len(content) > 0 {
		i := strings.IndexByte(content, '\n')
		if i < 0 {
			out = append(out, content)
			break
		}
		out = append(out, content[:i+1])
		content = content[i+1:]
	}
	return out
}

func isBegin(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), beginMarker) }
func isEnd(line string) bool   { return strings.HasPrefix(strings.TrimSpace(line), endMarker) }

// isLoopbackLine is a line this agent would write: a loopback address and one name.
func isLoopbackLine(line string) bool {
	f := strings.Fields(line)
	return len(f) == 2 && (f[0] == "127.0.0.1" || f[0] == "::1") && !strings.HasPrefix(f[1], "#")
}

// removeBlocks takes every marked block out of content. A block whose end
// marker is missing (a crash, or someone's edit) loses its begin marker and
// the loopback lines right after it, and nothing else. It reports whether
// anything was removed.
func removeBlocks(content string) (string, bool) {
	lines := splitLines(content)
	out := make([]string, 0, len(lines))
	removed := false
	for i := 0; i < len(lines); i++ {
		switch {
		case isBegin(lines[i]):
			removed = true
			end := -1
			for j := i + 1; j < len(lines); j++ {
				if isEnd(lines[j]) {
					end = j
					break
				}
				if isBegin(lines[j]) {
					break // the next block starts, this one was never closed
				}
			}
			if end >= 0 {
				i = end
				continue
			}
			for i+1 < len(lines) && isLoopbackLine(lines[i+1]) {
				i++
			}
		case isEnd(lines[i]):
			removed = true // an end marker with no begin
		default:
			out = append(out, lines[i])
		}
	}
	return strings.Join(out, ""), removed
}

func lineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	if content == "" && runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// buildBlock is the text of the marked block.
func buildBlock(names []string, ipv6 bool, eol string) string {
	var b strings.Builder
	b.WriteString(beginMarker + " (added while you focus, removed automatically)" + eol)
	for _, n := range names {
		b.WriteString("127.0.0.1 " + n + eol)
		if ipv6 {
			b.WriteString("::1 " + n + eol)
		}
	}
	b.WriteString(endMarker + eol)
	return b.String()
}

// withBlock returns content with exactly one marked block for names, or with
// no block when names is empty. The rest of the content is unchanged, except
// that a missing line break at the end of the file is added before the block.
func withBlock(content string, names []string, ipv6 bool) string {
	rest, _ := removeBlocks(content)
	if len(names) == 0 {
		return rest
	}
	eol := lineEnding(content)
	if rest != "" && !strings.HasSuffix(rest, "\n") {
		rest += eol
	}
	return rest + buildBlock(names, ipv6, eol)
}

// readHosts returns the file, or an empty string when it does not exist.
func readHosts(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

// syncHosts makes the file hold exactly one block for names (none when names
// is empty). It writes only when the content differs, so a clean file is
// never touched and needs no rights. It reports whether it wrote.
func syncHosts(path string, names []string, ipv6 bool) (bool, error) {
	have, err := readHosts(path)
	if err != nil {
		return false, err
	}
	want := withBlock(have, names, ipv6)
	if have == want {
		return false, nil
	}
	return true, writeAtomic(path, []byte(want))
}

// writeAtomic writes a temporary file next to the target and renames it over
// the target, so a failure never leaves half a file behind.
func writeAtomic(path string, data []byte) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".xc-focus-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(name, mode)
	if err := os.Rename(name, target); err != nil {
		// A hosts file that is a mount point (a container) cannot be replaced.
		// Writing into it keeps working, and the new content is complete in
		// one write.
		if errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EXDEV) {
			return os.WriteFile(target, data, mode)
		}
		return err
	}
	renamed = true
	return nil
}

// hostsResult maps a write error to the status the panel shows.
func hostsResult(err error) string {
	switch {
	case err == nil:
		return protocol.FocusBlockHostsOK
	case errors.Is(err, fs.ErrPermission):
		return protocol.FocusBlockHostsPermission
	}
	return protocol.FocusBlockHostsFailed
}
