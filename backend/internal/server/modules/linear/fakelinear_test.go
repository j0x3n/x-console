package linear_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"
)

// fakeLinear is a tiny Linear GraphQL API. It dispatches on operationName.
type fakeLinear struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	key       string
	states    []fakeState
	issues    map[string]*fakeIssue
	mutations []map[string]any // inputs of issueUpdate
	failing   bool
	failures  int
}

type fakeState struct {
	ID, Name, Type string
	Position       float64
}

type fakeIssue struct {
	ID, Identifier, Title, Description string
	Priority                           int
	DueDate                            string
	StateID                            string
	UpdatedAt                          time.Time
}

const teamID = "team-eng"

func newFakeLinear(t *testing.T) *fakeLinear {
	f := &fakeLinear{
		t: t, key: "lin_api_test_key_5678", issues: map[string]*fakeIssue{},
		states: []fakeState{
			{"s-backlog", "Backlog", "backlog", 0}, {"s-todo", "Todo", "unstarted", 1},
			{"s-review", "In Review", "started", 3}, {"s-prog", "In Progress", "started", 2},
			{"s-done", "Done", "completed", 4}, {"s-cancel", "Canceled", "canceled", 5},
		},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLinear) URL() string { return f.srv.URL + "/graphql" }

func (f *fakeLinear) set(fn func(f *fakeLinear)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeLinear) state(id string) fakeState {
	for _, s := range f.states {
		if s.ID == id {
			return s
		}
	}
	return fakeState{}
}

func (f *fakeLinear) node(is *fakeIssue) map[string]any {
	st := f.state(is.StateID)
	var due, desc any
	if is.DueDate != "" {
		due = is.DueDate
	}
	if is.Description != "" {
		desc = is.Description
	}
	return map[string]any{
		"id": is.ID, "identifier": is.Identifier, "title": is.Title, "description": desc, "priority": float64(is.Priority),
		"dueDate": due, "updatedAt": is.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"), "url": "https://linear.app/acme/issue/" + is.Identifier,
		"team":  map[string]any{"id": teamID},
		"state": map[string]any{"id": st.ID, "name": st.Name, "type": st.Type, "position": st.Position},
	}
}

func (f *fakeLinear) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.failing {
		f.failures++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"errors":[{"message":"down for maintenance"}]}`)
		return
	}
	if r.Header.Get("Authorization") != f.key {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":[{"message":"Authentication required","extensions":{"code":"AUTHENTICATION_ERROR"}}]}`)
		return
	}
	var req struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	var data any
	switch req.OperationName {
	case "Viewer":
		data = map[string]any{"viewer": map[string]any{"id": "u1", "name": "Jo", "email": "jo@example.com"}}
	case "Teams":
		data = map[string]any{"teams": map[string]any{"nodes": []any{
			map[string]any{"id": teamID, "key": "ENG", "name": "Engineering"},
			map[string]any{"id": "team-ops", "key": "OPS", "name": "Operations"},
		}}}
	case "TeamStates":
		var nodes []any
		for _, s := range f.states {
			nodes = append(nodes, map[string]any{"id": s.ID, "name": s.Name, "type": s.Type, "position": s.Position})
		}
		data = map[string]any{"team": map[string]any{"states": map[string]any{"nodes": nodes}}}
	case "Issues":
		since, _ := time.Parse(time.RFC3339Nano, req.Variables["since"].(string))
		var list []*fakeIssue
		for _, is := range f.issues {
			if req.Variables["teamId"] == teamID && is.UpdatedAt.After(since) {
				list = append(list, is)
			}
		}
		// Linear lists newest first; the module must not rely on the order.
		sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
		nodes := []any{}
		for _, is := range list {
			nodes = append(nodes, f.node(is))
		}
		data = map[string]any{"issues": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}
	case "Issue":
		is, ok := f.issues[req.Variables["id"].(string)]
		if !ok {
			_, _ = io.WriteString(w, `{"errors":[{"message":"Entity not found"}],"data":null}`)
			return
		}
		data = map[string]any{"issue": f.node(is)}
	case "UpdateIssue":
		is, ok := f.issues[req.Variables["id"].(string)]
		if !ok {
			_, _ = io.WriteString(w, `{"errors":[{"message":"Entity not found"}],"data":null}`)
			return
		}
		input := req.Variables["input"].(map[string]any)
		f.mutations = append(f.mutations, input)
		for k, v := range input {
			switch k {
			case "title":
				is.Title = v.(string)
			case "description":
				is.Description = v.(string)
			case "priority":
				is.Priority = int(v.(float64))
			case "dueDate":
				if v == nil {
					is.DueDate = ""
				} else {
					is.DueDate = v.(string)
				}
			case "stateId":
				is.StateID = v.(string)
			}
		}
		is.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
		data = map[string]any{"issueUpdate": map[string]any{"success": true, "issue": f.node(is)}}
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":[{"message":"unknown operation"}]}`)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func (f *fakeLinear) mutationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.mutations)
}

func (f *fakeLinear) lastMutation() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.mutations) == 0 {
		return nil
	}
	return f.mutations[len(f.mutations)-1]
}

func (f *fakeLinear) issue(id string) fakeIssue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.issues[id]
}
