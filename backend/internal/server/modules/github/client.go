package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// restClient calls the GitHub REST API. GET responses with an ETag are
// cached and revalidated with If-None-Match, so unchanged data costs no
// rate limit. When GitHub says the limit is used up, calls fail fast until
// the reset time.
type restClient struct {
	base  string
	token string
	hc    *http.Client
	rate  *rateState
	etag  *etagCache
	now   func() time.Time
}

// apiError is a non-2xx answer from GitHub.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "令牌无效或已过期"
	case http.StatusForbidden:
		return "令牌没有权限：" + e.Message
	case http.StatusNotFound:
		return "找不到，或者令牌没有权限访问"
	case http.StatusUnprocessableEntity:
		return "GitHub 拒绝了请求：" + e.Message
	}
	return fmt.Sprintf("GitHub 返回 %d：%s", e.Status, e.Message)
}

// rateLimitError means calls are paused until Until.
type rateLimitError struct{ Until time.Time }

func (e *rateLimitError) Error() string {
	return "GitHub 请求次数用完了，" + e.Until.Local().Format("15:04") + " 以后再试"
}

func isRateLimited(err error) bool {
	var rl *rateLimitError
	return errors.As(err, &rl)
}

func statusOf(err error) int {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

// rateState remembers the latest rate limit headers.
type rateState struct {
	mu           sync.Mutex
	remaining    int // -1 unknown
	reset        time.Time
	blockedUntil time.Time
}

func (r *rateState) snapshot() (remaining int, reset time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.remaining, r.reset
}

func (r *rateState) blocked(now time.Time) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Before(r.blockedUntil) {
		return r.blockedUntil, true
	}
	return time.Time{}, false
}

// observe reads the rate limit headers. It reports whether the response is
// a rate limit refusal.
func (r *rateState) observe(h http.Header, status int, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	remaining := -1
	if v, err := strconv.Atoi(h.Get("X-RateLimit-Remaining")); err == nil {
		remaining = v
		r.remaining = v
	}
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		r.reset = time.Unix(v, 0).UTC()
	}
	var retryAfter time.Duration
	if v, err := strconv.Atoi(h.Get("Retry-After")); err == nil && v >= 0 {
		retryAfter = time.Duration(v) * time.Second
	}
	limited := (status == http.StatusForbidden || status == http.StatusTooManyRequests) && (remaining == 0 || retryAfter > 0)
	switch {
	case limited && retryAfter > 0:
		r.blockedUntil = now.Add(retryAfter)
	case remaining == 0 && r.reset.After(now):
		// Used up (either refused now or the last allowed call).
		r.blockedUntil = r.reset
	case limited:
		r.blockedUntil = now.Add(time.Minute)
	}
	return limited
}

// etagCache stores GET bodies by URL.
type etagCache struct {
	mu      sync.Mutex
	entries map[string]etagEntry
}

type etagEntry struct {
	etag string
	body []byte
}

const maxETagEntries = 2000

func newETagCache() *etagCache { return &etagCache{entries: map[string]etagEntry{}} }

func (c *etagCache) get(key string) (etagEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *etagCache) put(key string, e etagEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxETagEntries {
		c.entries = map[string]etagEntry{}
	}
	c.entries[key] = e
}

func (c *etagCache) clear() {
	c.mu.Lock()
	c.entries = map[string]etagEntry{}
	c.mu.Unlock()
}

// get fetches path with query into out.
func (c *restClient) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

func (c *restClient) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out)
}

func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	if until, ok := c.rate.blocked(c.now()); ok {
		return &rateLimitError{Until: until}
	}
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "x-console")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// The token is part of the cache key: another token may see other data.
	cacheKey := c.token + " " + u
	cached, haveCached := etagEntry{}, false
	if method == http.MethodGet && c.etag != nil {
		if cached, haveCached = c.etag.get(cacheKey); haveCached {
			req.Header.Set("If-None-Match", cached.etag)
		}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("连不上 GitHub：%w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if c.rate.observe(resp.Header, resp.StatusCode, c.now()) {
		until, _ := c.rate.blocked(c.now())
		return &rateLimitError{Until: until}
	}
	switch {
	case resp.StatusCode == http.StatusNotModified && haveCached:
		raw = cached.body
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if tag := resp.Header.Get("ETag"); tag != "" && method == http.MethodGet && c.etag != nil {
			c.etag.put(cacheKey, etagEntry{etag: tag, body: raw})
		}
	default:
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		return &apiError{Status: resp.StatusCode, Message: e.Message}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("GitHub 返回的数据看不懂：%w", err)
	}
	return nil
}

// ---- response shapes ----

type ghUser struct {
	Login string `json:"login"`
}

type ghRepo struct {
	DefaultBranch string `json:"default_branch"`
}

type ghPull struct {
	Number             int       `json:"number"`
	Title              string    `json:"title"`
	HTMLURL            string    `json:"html_url"`
	Draft              bool      `json:"draft"`
	User               ghUser    `json:"user"`
	RequestedReviewers []ghUser  `json:"requested_reviewers"`
	RequestedTeams     []any     `json:"requested_teams"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Head               struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type ghReview struct {
	User  ghUser `json:"user"`
	State string `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING
}

type ghCombinedStatus struct {
	State      string `json:"state"` // success, pending, failure, error
	TotalCount int    `json:"total_count"`
}

type ghCheckRuns struct {
	TotalCount int `json:"total_count"`
	CheckRuns  []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"check_runs"`
}

type ghRun struct {
	ID         int64     `json:"id"`
	WorkflowID int64     `json:"workflow_id"`
	Name       string    `json:"name"`
	HeadBranch string    `json:"head_branch"`
	HeadSHA    string    `json:"head_sha"`
	Event      string    `json:"event"`
	Status     string    `json:"status"`
	Conclusion *string   `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ghRuns struct {
	WorkflowRuns []ghRun `json:"workflow_runs"`
}

type ghIssue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	HTMLURL   string   `json:"html_url"`
	User      ghUser   `json:"user"`
	Assignees []ghUser `json:"assignees"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest json.RawMessage `json:"pull_request"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
