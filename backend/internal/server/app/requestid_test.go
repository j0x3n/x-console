package app_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B41: every response carries X-Request-Id and error bodies repeat it.
func TestErrorCarriesRequestID(t *testing.T) {
	env := testutil.New(t)
	req, err := http.NewRequest(http.MethodGet, env.URL("/no-such-endpoint"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body struct {
		Code      string
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	header := resp.Header.Get("X-Request-Id")
	if header == "" || body.RequestID != header || body.Code != "not_found" {
		t.Fatalf("header %q body %+v", header, body)
	}
}

func TestClientErrorsAreRateLimited(t *testing.T) {
	env := testutil.New(t)
	report := map[string]string{"title": "保存失败", "message": "boom", "page": "/notes", "detail": "x"}
	for i := 0; i < 40; i++ {
		// Over the limit the server drops the report but still answers 204.
		env.MustDo(http.MethodPost, "/client-errors", report, nil)
	}
	if status, _ := env.Do(http.MethodPost, "/client-errors", map[string]string{"message": "no title"}, nil); status != http.StatusNoContent && status != http.StatusBadRequest {
		t.Fatalf("missing title: %d", status)
	}
}
