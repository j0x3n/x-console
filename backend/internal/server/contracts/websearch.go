package contracts

import (
	"context"
	"encoding/json"
	"errors"
)

// WebSearchKey is where the ai module provides WebSearch (B211). Modules
// that want facts from the internet call it; they never see the search key.
const WebSearchKey = "ai.web_search"

// ErrWebSearchOff means web search is switched off in the AI settings.
var ErrWebSearchOff = errors.New("还没有打开联网搜索，去 设置 → AI 里打开")

// WebSource is a page the answer was built from.
type WebSource struct{ Title, URL string }

// WebResearch is one research request.
type WebResearch struct {
	Purpose    string          // usage record reference, for example "recipes.discover"
	System     string          // what the model must do
	Task       string          // what to find this time
	Schema     json.RawMessage // JSON Schema of the answer
	MaxSources int             // pages to read at most; default 4, at most 8
}

// WebSearch looks things up on the internet and answers in a given shape.
type WebSearch interface {
	// Mode returns "off", "openai" or "brave".
	Mode(ctx context.Context) string
	// Research searches, then fills out (a pointer) from the JSON answer
	// that follows Schema. It returns the sources used. It returns
	// ErrWebSearchOff when search is off.
	Research(ctx context.Context, in WebResearch, out any) ([]WebSource, error)
}

// PageReaderKey is where the readlater module provides PageReader (B211).
const PageReaderKey = "readlater.page_reader"

// WebPage is the title and the text of a public web page.
type WebPage struct{ Title, Text string }

// PageReader reads a public web page. It refuses addresses on the local
// machine and the internal network, and it does not save the page.
type PageReader interface {
	Read(ctx context.Context, url string) (WebPage, error)
}
