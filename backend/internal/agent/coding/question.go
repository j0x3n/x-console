package coding

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func readQuestion(wt string) (*protocol.CodingQuestion, error) {
	path := filepath.Join(wt, ".xc-question.md")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return nil, errors.New("question file must be a regular file under 64 KiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, actual) {
		return nil, errors.New("question file changed")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return nil, err
	}
	if len(raw) > 65536 {
		return nil, errors.New("question is too long")
	}
	var q protocol.CodingQuestion
	if json.Unmarshal(raw, &q) != nil {
		title, detail, _ := strings.Cut(strings.TrimSpace(string(raw)), "\n")
		q.Title = strings.TrimSpace(strings.TrimLeft(title, "#"))
		q.Detail = strings.TrimSpace(detail)
	}
	q.Title = strings.TrimSpace(q.Title)
	if q.Title == "" || len(q.Title) > 500 || len(q.Options) > 20 {
		return nil, errors.New("invalid question")
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Remove(path); err != nil {
		return nil, err
	}
	return &q, nil
}
