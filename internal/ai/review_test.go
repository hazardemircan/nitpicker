package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// fakeOpenAI starts a local HTTP server that mimics the OpenAI chat completions endpoint.
// It returns the provided findings as the model's JSON response.
func fakeOpenAI(t *testing.T, findings []Finding) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Minimal validation: must be a POST to /chat/completions.
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		// Encode findings as the JSON content the model "returns".
		inner, _ := json.Marshal(reviewResponse{Findings: findings})

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": string(inner)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	t.Cleanup(srv.Close)
	return srv
}

func TestReviewFile_ReturnsFindingsFromModel(t *testing.T) {
	want := []Finding{
		{Line: 42, Severity: "blocker", Message: "SQL injection via string concatenation"},
		{Line: 55, Severity: "minor", Message: "Missing documentation comment"},
	}

	srv := fakeOpenAI(t, want)
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	reviewer := NewReviewer("test-key", "gpt-4o", nil)
	got, err := reviewer.ReviewFile("internal/db/query.go", "+[42] db.Query(\"SELECT * FROM users WHERE id='\" + id + \"'\")")
	if err != nil {
		t.Fatalf("ReviewFile returned error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d", len(got), len(want))
	}
	for i, f := range got {
		if f.Line != want[i].Line || f.Severity != want[i].Severity || f.Message != want[i].Message {
			t.Errorf("findings[%d] = %+v, want %+v", i, f, want[i])
		}
	}
}

func TestReviewFile_EmptyFindingsIsValid(t *testing.T) {
	srv := fakeOpenAI(t, []Finding{})
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	reviewer := NewReviewer("test-key", "gpt-4o", nil)
	got, err := reviewer.ReviewFile("clean.go", "+[1] // well written code")
	if err != nil {
		t.Fatalf("ReviewFile returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 findings, got %d", len(got))
	}
}

func TestReviewFile_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid api key"}}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	reviewer := NewReviewer("bad-key", "gpt-4o", nil)
	_, err := reviewer.ReviewFile("foo.go", "+[1] code")
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	// The caller relies on the typed error to tell an un-reviewed file apart
	// from a clean review, and on Permanent() to stop early on unrecoverable errors.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
	if !apiErr.Permanent() {
		t.Error("401 should be treated as a permanent (non-recoverable) error")
	}
}

func TestAPIError_Permanent(t *testing.T) {
	cases := []struct {
		name          string
		code          int
		body          string
		wantPermanent bool
		wantCode      string
	}{
		{"quota exhausted", http.StatusTooManyRequests, `{"error":{"code":"insufficient_quota"}}`, true, "insufficient_quota"},
		{"transient rate limit", http.StatusTooManyRequests, `{"error":{"code":"rate_limit_exceeded"}}`, false, "rate_limit_exceeded"},
		{"unauthorized", http.StatusUnauthorized, `bad key`, true, ""},
		{"forbidden", http.StatusForbidden, `no access`, true, ""},
		{"server error", http.StatusInternalServerError, `oops`, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAPIError(tc.code, []byte(tc.body))
			if e.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", e.Code, tc.wantCode)
			}
			if got := e.Permanent(); got != tc.wantPermanent {
				t.Errorf("Permanent() = %v, want %v", got, tc.wantPermanent)
			}
		})
	}
}

func TestReviewFile_SystemPromptIncludesRules(t *testing.T) {
	rules := []string{"No panic in production", "Check all errors"}

	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		msgs := payload["messages"].([]any)
		capturedBody = msgs[0].(map[string]any)["content"].(string)

		inner, _ := json.Marshal(reviewResponse{})
		resp := map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(inner)}}}}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	// Temporarily clear any pre-existing env override so NewReviewer picks up our server.
	os.Unsetenv("OPENAI_BASE_URL")
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	reviewer := NewReviewer("test-key", "gpt-4o", rules)
	reviewer.ReviewFile("x.go", "+[1] code")

	for _, rule := range rules {
		if !contains(capturedBody, rule) {
			t.Errorf("system prompt missing rule %q\nprompt:\n%s", rule, capturedBody)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsRune(s, substr))
}

func containsRune(s, substr string) bool {
	for i := range s {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
