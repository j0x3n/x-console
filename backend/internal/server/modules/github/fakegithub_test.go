package github_test

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub is a tiny GitHub REST API with ETags and a rate limit switch.
type fakeGitHub struct {
	t   *testing.T
	srv *httptest.Server

	mu            sync.Mutex
	token         string
	login         string
	defaultBranch string
	pulls         map[string][]map[string]any // repo → pull objects
	reviews       map[string][]map[string]any // "repo#n" → reviews
	statuses      map[string]map[string]any   // sha → combined status
	checkRuns     map[string][]map[string]any // sha → check runs
	runs          map[string][]map[string]any // repo → runs, newest first
	issues        map[string][]map[string]any // repo → issues
	commits       map[string][]map[string]any
	jobs          map[string][]map[string]any
	releases      map[string]map[string]any
	created       []map[string]any // bodies of POST /pulls
	requests      int
	notModified   int
	rateLimited   bool
	nextPR        int
	userRepos     []map[string]any // GET /user/repos
	remaining     int              // X-RateLimit-Remaining, 4999 when zero
	limit         int              // X-RateLimit-Limit, 5000 when zero
	// B109: billing answers as raw JSON, or a status code when not zero.
	billingUsage, billingActions             string
	billingUsageStatus, billingActionsStatus int
	billingRequests                          int
	billingQuery                             string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{
		t: t, token: "ghp_test_token_1234", login: "jo", defaultBranch: "main",
		pulls: map[string][]map[string]any{}, reviews: map[string][]map[string]any{},
		statuses: map[string]map[string]any{}, checkRuns: map[string][]map[string]any{},
		runs: map[string][]map[string]any{}, issues: map[string][]map[string]any{}, nextPR: 100,
		commits: map[string][]map[string]any{}, jobs: map[string][]map[string]any{}, releases: map[string]map[string]any{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) URL() string { return f.srv.URL }

func (f *fakeGitHub) counts() (requests, notModified int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.notModified
}

func (f *fakeGitHub) set(fn func(f *fakeGitHub)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func pull(number int, title, head, sha string) map[string]any {
	return map[string]any{
		"number": number, "title": title, "html_url": fmt.Sprintf("https://github.com/acme/app/pull/%d", number),
		"draft": false, "state": "open", "user": map[string]any{"login": "jo"}, "requested_reviewers": []any{}, "requested_teams": []any{},
		"created_at": "2026-09-20T10:00:00Z", "updated_at": "2026-09-25T10:00:00Z",
		"head": map[string]any{"ref": head, "sha": sha}, "base": map[string]any{"ref": "main"},
	}
}

func run(id, workflow int, name, branch, status, conclusion string) map[string]any {
	var c any
	if conclusion != "" {
		c = conclusion
	}
	return map[string]any{
		"id": id, "workflow_id": workflow, "name": name, "head_branch": branch, "head_sha": "abc", "event": "push",
		"status": status, "conclusion": c, "html_url": fmt.Sprintf("https://github.com/acme/app/actions/runs/%d", id),
		"created_at": time.Date(2026, 9, 25, 10, 0, id, 0, time.UTC).Format(time.RFC3339), "updated_at": "2026-09-25T10:05:00Z",
	}
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	if f.rateLimited {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"API rate limit exceeded"}`)
		return
	}
	remaining, limit := 4999, 5000
	if f.remaining != 0 {
		remaining = f.remaining
	}
	if f.limit != 0 {
		limit = f.limit
	}
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	var body any
	switch {
	case r.URL.Path == "/user":
		body = map[string]any{"login": f.login}
	case len(parts) == 3 && parts[0] == "repos":
		body = map[string]any{"default_branch": f.defaultBranch}
	case len(parts) == 4 && parts[3] == "pulls" && r.Method == http.MethodPost:
		repo := parts[1] + "/" + parts[2]
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.created = append(f.created, in)
		f.nextPR++
		p := pull(f.nextPR, in["title"].(string), in["head"].(string), "newsha")
		f.pulls[repo] = append(f.pulls[repo], p)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
		return
	case r.URL.Path == "/users/"+f.login+"/settings/billing/usage" || r.URL.Path == "/users/"+f.login+"/settings/billing/actions":
		f.billingRequests++
		raw, status := f.billingActions, f.billingActionsStatus
		if parts[4] == "usage" {
			raw, status, f.billingQuery = f.billingUsage, f.billingUsageStatus, r.URL.RawQuery
		}
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"message":"no"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, raw)
		return
	case r.URL.Path == "/user/repos":
		body = f.page(w, r, f.userRepos)
	case len(parts) == 4 && parts[3] == "pulls":
		var filtered []map[string]any
		state := r.URL.Query().Get("state")
		for _, p := range f.pulls[parts[1]+"/"+parts[2]] {
			if state == "all" || p["state"] == state {
				filtered = append(filtered, p)
			}
		}
		body = f.page(w, r, filtered)
	case len(parts) == 6 && parts[3] == "pulls" && parts[5] == "reviews":
		body = orEmpty(f.reviews[parts[1]+"/"+parts[2]+"#"+parts[4]])
	case len(parts) == 6 && parts[3] == "commits" && parts[5] == "status":
		if st, ok := f.statuses[parts[4]]; ok {
			body = st
		} else {
			body = map[string]any{"state": "pending", "total_count": 0}
		}
	case len(parts) == 6 && parts[3] == "commits" && parts[5] == "check-runs":
		runs := orEmpty(f.checkRuns[parts[4]])
		body = map[string]any{"total_count": len(runs), "check_runs": runs}
	case len(parts) == 5 && parts[3] == "actions" && parts[4] == "runs":
		body = map[string]any{"workflow_runs": orEmpty(f.runs[parts[1]+"/"+parts[2]])}
	case len(parts) == 4 && parts[3] == "commits":
		body = f.page(w, r, f.commits[parts[1]+"/"+parts[2]])
	case len(parts) == 7 && parts[3] == "actions" && parts[6] == "jobs":
		body = map[string]any{"jobs": f.page(w, r, f.jobs[parts[5]])}
	case len(parts) == 5 && parts[3] == "releases" && parts[4] == "latest":
		if release, ok := f.releases[parts[1]+"/"+parts[2]]; ok {
			body = release
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	case len(parts) == 4 && parts[3] == "issues":
		var out []map[string]any
		for _, is := range f.issues[parts[1]+"/"+parts[2]] {
			if a := r.URL.Query().Get("assignee"); a != "" {
				found := false
				for _, x := range is["assignees"].([]any) {
					if x.(map[string]any)["login"] == a {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			if c := r.URL.Query().Get("creator"); c != "" && is["user"].(map[string]any)["login"] != c {
				continue
			}
			out = append(out, is)
		}
		body = orEmpty(out)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		return
	}
	raw, _ := json.Marshal(body)
	sum := sha1.Sum(raw)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		f.notModified++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// page cuts list by the page and per_page query parameters and sets the Link
// header when more follows, like GitHub does.
func (f *fakeGitHub) page(w http.ResponseWriter, r *http.Request, list []map[string]any) []map[string]any {
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if perPage <= 0 {
		perPage = 30
	}
	if page <= 0 {
		page = 1
	}
	from := min((page-1)*perPage, len(list))
	to := min(from+perPage, len(list))
	if to < len(list) {
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d>; rel="next"`, f.srv.URL, r.URL.Path, page+1))
	}
	return orEmpty(list[from:to])
}

func orEmpty(list []map[string]any) []map[string]any {
	if list == nil {
		return []map[string]any{}
	}
	return list
}
