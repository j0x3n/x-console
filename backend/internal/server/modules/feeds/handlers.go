package feeds

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/feeds/api"
)

var (
	newsCursor  = regexp.MustCompile(`^[0-9]{1,18}$`)
	tweetCursor = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// Limits on the text parameters. They match the small service.
const (
	maxQuery  = 200
	maxFilter = 100
)

// TestFeeds implements api.ServerInterface: GET /v1/health with the saved settings.
func (m *Module) TestFeeds(w http.ResponseWriter, r *http.Request) {
	var out api.FeedsHealth
	if err := m.get(r.Context(), "/v1/health", nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ListFeedsNews implements api.ServerInterface. Every parameter is checked and
// copied by name into a new query, so nothing else reaches the service.
func (m *Module) ListFeedsNews(w http.ResponseWriter, r *http.Request, p api.ListFeedsNewsParams) {
	q := url.Values{}
	var err error
	err = firstErr(err, text(q, "q", p.Q, maxQuery))
	err = firstErr(err, text(q, "tag", p.Tag, maxFilter))
	err = firstErr(err, text(q, "category", p.Category, maxFilter))
	err = firstErr(err, number(q, "days", p.Days, 1, 3650))
	err = firstErr(err, number(q, "minImportance", p.MinImportance, 1, 5))
	err = firstErr(err, number(q, "limit", p.Limit, 1, 100))
	err = firstErr(err, cursor(q, p.Before, newsCursor))
	if p.Pushed != nil {
		q.Set("pushed", strconv.FormatBool(*p.Pushed))
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var out api.FeedsNewsPage
	if err := m.get(r.Context(), "/v1/news", q, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if out.Items == nil {
		out.Items = []api.FeedsNewsItem{}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// GetFeedsNewsFacets implements api.ServerInterface.
func (m *Module) GetFeedsNewsFacets(w http.ResponseWriter, r *http.Request) {
	var out api.FeedsNewsFacets
	if err := m.get(r.Context(), "/v1/news/facets", nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if out.Categories == nil {
		out.Categories = []string{}
	}
	if out.Tags == nil {
		out.Tags = []struct {
			Count int    `json:"count"`
			Tag   string `json:"tag"`
		}{}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// GetFeedsNews implements api.ServerInterface. The path is built from the
// number the router parsed, never from the raw request path.
func (m *Module) GetFeedsNews(w http.ResponseWriter, r *http.Request, id int64) {
	if id < 1 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var out api.FeedsNewsDetail
	if err := m.get(r.Context(), "/v1/news/"+strconv.FormatInt(id, 10), nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if out.Media == nil {
		out.Media = []api.FeedsNewsMedia{}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ListFeedsTweets implements api.ServerInterface.
func (m *Module) ListFeedsTweets(w http.ResponseWriter, r *http.Request, p api.ListFeedsTweetsParams) {
	q := url.Values{}
	var err error
	err = firstErr(err, text(q, "q", p.Q, maxQuery))
	err = firstErr(err, text(q, "author", p.Author, maxFilter))
	err = firstErr(err, number(q, "limit", p.Limit, 1, 100))
	err = firstErr(err, cursor(q, p.Before, tweetCursor))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var out api.FeedsTweetsPage
	if err := m.get(r.Context(), "/v1/tweets", q, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if out.Items == nil {
		out.Items = []api.FeedsTweet{}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// text copies a trimmed, non-empty string parameter.
func text(q url.Values, name string, v *string, max int) error {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	if len([]rune(s)) > max {
		return httpx.Invalid(name + " 太长了")
	}
	q.Set(name, s)
	return nil
}

func number(q url.Values, name string, v *int, lo, hi int) error {
	if v == nil {
		return nil
	}
	if *v < lo || *v > hi {
		return httpx.Invalid(name + " 超出范围")
	}
	q.Set(name, strconv.Itoa(*v))
	return nil
}

func cursor(q url.Values, v *string, re *regexp.Regexp) error {
	if v == nil || *v == "" {
		return nil
	}
	if !re.MatchString(*v) {
		return httpx.Invalid("before 格式不对")
	}
	q.Set("before", *v)
	return nil
}
