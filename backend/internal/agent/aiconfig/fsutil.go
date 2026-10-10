package aiconfig

import (
	"io"
	"os"
	"path/filepath"
)

// readText returns the file's text with line breaks as \n, and whether it used
// \r\n. A file that does not exist reads as empty.
func readText(path string) (text string, crlf bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if isNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	s := string(raw)
	if containsCRLF(s) {
		return replaceCRLF(s), true, nil
	}
	return s, false, nil
}

func containsCRLF(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '\r' && s[i+1] == '\n' {
			return true
		}
	}
	return false
}

func replaceCRLF(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func toCRLF(s string) string {
	out := make([]byte, 0, len(s)+len(s)/20)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, '\r')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// writeFileAtomic replaces the file through a temporary file in the same
// directory. A symbolic link is followed, so a file managed by a dotfiles tool
// stays a link. The permissions of an existing file are kept. With backup the
// first change also leaves <file>.x-console.bak.
func writeFileAtomic(path string, data []byte, mode os.FileMode, backup bool) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
		if backup {
			if err := copyOnce(path, path+".x-console.bak", mode); err != nil {
				return err
			}
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".x-console-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func copyOnce(src, dst string, mode os.FileMode) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
