package readlater

import (
	"context"
	"errors"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// B211: other modules read a public web page through readlater. The page goes
// through fetchPage, so addresses on the local machine and the internal
// network are refused. Nothing is saved to the read-later list.

type pageReader struct{ m *Module }

var _ contracts.PageReader = pageReader{}

func (p pageReader) Read(ctx context.Context, rawURL string) (contracts.WebPage, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return contracts.WebPage{}, errors.New("网址要以 http:// 或 https:// 开头")
	}
	pg, err := p.m.fetchPage(ctx, rawURL)
	if err != nil {
		return contracts.WebPage{}, err
	}
	text := strings.TrimSpace(pg.Content)
	if text == "" {
		text = strings.TrimSpace(pg.Excerpt)
	}
	return contracts.WebPage{Title: strings.TrimSpace(pg.Title), Text: text}, nil
}
