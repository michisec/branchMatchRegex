package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name     string
		branch   string
		patterns []string
		template string
		want     string
	}{
		{
			name:     "default template",
			branch:   "feature/test",
			patterns: []string{"feature/.*"},
			want:     "Branch `feature/test` does not match any of the configured patterns.",
		},
		{
			name:     "custom template",
			branch:   "feature/test",
			patterns: []string{"feature/.*", "fix/.*"},
			template: "Branch {branch} does not match: {patterns}",
			want:     "Branch feature/test does not match: `feature/.*`, `fix/.*`",
		},
		{
			name:     "empty patterns",
			branch:   "feature/test",
			template: "Patterns: {patterns}",
			want:     "Patterns: _(none)_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(tt.branch, tt.patterns, tt.template); got != tt.want {
				t.Errorf("render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPullRequestNumber(t *testing.T) {
	writeEvent := func(t *testing.T, content string) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), "event.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write event: %v", err)
		}
		return path
	}

	t.Run("reads a pull request event", func(t *testing.T) {
		path := writeEvent(t, `{"pull_request":{"number":42}}`)
		if got, ok := pullRequestNumber(path, ""); !ok || got != 42 {
			t.Errorf("pullRequestNumber() = %d, %t; want 42, true", got, ok)
		}
	})

	t.Run("reads an issue comment event", func(t *testing.T) {
		path := writeEvent(t, `{"issue":{"number":7,"pull_request":{}}}`)
		if got, ok := pullRequestNumber(path, ""); !ok || got != 7 {
			t.Errorf("pullRequestNumber() = %d, %t; want 7, true", got, ok)
		}
	})

	t.Run("ignores a plain issue", func(t *testing.T) {
		path := writeEvent(t, `{"issue":{"number":7}}`)
		if _, ok := pullRequestNumber(path, ""); ok {
			t.Error("expected no pull request")
		}
	})

	t.Run("ignores a null pull request", func(t *testing.T) {
		path := writeEvent(t, `{"issue":{"number":7,"pull_request":null}}`)
		if _, ok := pullRequestNumber(path, ""); ok {
			t.Error("expected no pull request")
		}
	})

	t.Run("falls back to the ref", func(t *testing.T) {
		for _, ref := range []string{"refs/pull/123/merge", "refs/pull/123/head"} {
			if got, ok := pullRequestNumber("", ref); !ok || got != 123 {
				t.Errorf("pullRequestNumber(%q) = %d, %t; want 123, true", ref, got, ok)
			}
		}
	})

	t.Run("ignores a branch ref", func(t *testing.T) {
		if _, ok := pullRequestNumber("", "refs/heads/main"); ok {
			t.Error("expected no pull request")
		}
	})

	t.Run("falls back when the event file is missing", func(t *testing.T) {
		if got, ok := pullRequestNumber(filepath.Join(t.TempDir(), "missing.json"), "refs/pull/9/merge"); !ok || got != 9 {
			t.Errorf("pullRequestNumber() = %d, %t; want 9, true", got, ok)
		}
	})

	t.Run("ignores invalid event JSON", func(t *testing.T) {
		path := writeEvent(t, "not json")
		if _, ok := pullRequestNumber(path, ""); ok {
			t.Error("expected no pull request")
		}
	})
}

func TestUpsertPullRequestComment(t *testing.T) {
	t.Run("creates a comment", func(t *testing.T) {
		var created string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("unexpected authorization header: %q", r.Header.Get("Authorization"))
			}

			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte("[]"))
			case http.MethodPost:
				var payload struct {
					Body string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("failed to decode payload: %v", err)
				}
				created = payload.Body
				w.WriteHeader(http.StatusCreated)
			default:
				t.Errorf("unexpected method: %s", r.Method)
			}
		}))
		defer server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: server.URL, Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "hello"); err != nil {
			t.Fatalf("UpsertPullRequestComment() returned error: %v", err)
		}
		if want := marker + "\nhello"; created != want {
			t.Errorf("created comment = %q, want %q", created, want)
		}
	})

	t.Run("updates an existing comment", func(t *testing.T) {
		var updated string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`[{"id":99,"body":"<!-- branchMatchRegex:unmatched -->\nold"}]`))
			case http.MethodPatch:
				var payload struct {
					Body string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("failed to decode payload: %v", err)
				}
				updated = payload.Body
			default:
				t.Errorf("unexpected method: %s", r.Method)
			}
		}))
		defer server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: server.URL, Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "new"); err != nil {
			t.Fatalf("UpsertPullRequestComment() returned error: %v", err)
		}
		if want := marker + "\nnew"; updated != want {
			t.Errorf("updated comment = %q, want %q", updated, want)
		}
	})

	t.Run("paginates comments", func(t *testing.T) {
		var updated bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				if r.URL.Query().Get("page") == "1" {
					comments := make([]comment, commentsPerPage)
					for i := range comments {
						comments[i] = comment{ID: int64(i + 1), Body: fmt.Sprintf("comment %d", i)}
					}
					_ = json.NewEncoder(w).Encode(comments)
					return
				}
				_, _ = w.Write([]byte(`[{"id":999,"body":"<!-- branchMatchRegex:unmatched -->\nold"}]`))
			case http.MethodPatch:
				updated = true
			default:
				t.Errorf("unexpected method: %s", r.Method)
			}
		}))
		defer server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: server.URL, Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "new"); err != nil {
			t.Fatalf("UpsertPullRequestComment() returned error: %v", err)
		}
		if !updated {
			t.Error("expected the existing comment to be updated")
		}
	})

	t.Run("searches all pages before creating a comment", func(t *testing.T) {
		var updated bool
		pages := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				pages++
				page, err := strconv.Atoi(r.URL.Query().Get("page"))
				if err != nil {
					t.Errorf("invalid page: %v", err)
				}
				if page < 11 {
					comments := make([]comment, commentsPerPage)
					for i := range comments {
						comments[i] = comment{ID: int64(page*commentsPerPage + i + 1), Body: fmt.Sprintf("comment %d", i)}
					}
					_ = json.NewEncoder(w).Encode(comments)
					return
				}
				_, _ = w.Write([]byte(`[{"id":999,"body":"<!-- branchMatchRegex:unmatched -->\nold"}]`))
			case http.MethodPatch:
				updated = true
			default:
				t.Errorf("unexpected method: %s", r.Method)
			}
		}))
		defer server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: server.URL, Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "new"); err != nil {
			t.Fatalf("UpsertPullRequestComment() returned error: %v", err)
		}
		if !updated {
			t.Error("expected the existing comment to be updated")
		}
		if pages != 11 {
			t.Errorf("searched %d pages, want 11", pages)
		}
	})

	t.Run("returns an API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
		}))
		defer server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: server.URL, Token: "token", Repository: "owner/repo"}
		err := client.UpsertPullRequestComment(context.Background(), 5, "hello")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "403"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
		if !IsForbidden(err) {
			t.Errorf("IsForbidden() = false, want true for %v", err)
		}
	})

	t.Run("returns an error for invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: server.URL, Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "hello"); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error for an invalid base URL", func(t *testing.T) {
		client := &Client{BaseURL: "://bad", Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "hello"); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error when the API is unreachable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := server.URL
		server.Close()

		client := &Client{HTTPClient: server.Client(), BaseURL: url, Token: "token", Repository: "owner/repo"}
		if err := client.UpsertPullRequestComment(context.Background(), 5, "hello"); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestPostUnmatched(t *testing.T) {
	eventPath := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(eventPath, []byte(`{"pull_request":{"number":5}}`), 0o600); err != nil {
		t.Fatalf("failed to write event: %v", err)
	}

	t.Run("requires a token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		err := PostUnmatched("", "feature/test", nil, "")
		if err == nil || !strings.Contains(err.Error(), "missing github token") {
			t.Errorf("PostUnmatched() error = %v, want missing github token", err)
		}
	})

	t.Run("uses the token from the environment", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "env-token")
		t.Setenv("GITHUB_REPOSITORY", "")
		t.Setenv("GITHUB_EVENT_PATH", "")
		t.Setenv("GITHUB_REF", "refs/heads/main")

		err := PostUnmatched("", "feature/test", nil, "")
		if err == nil || !strings.Contains(err.Error(), "missing GITHUB_REPOSITORY") {
			t.Errorf("PostUnmatched() error = %v, want missing GITHUB_REPOSITORY", err)
		}
	})

	t.Run("requires a repository", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "")

		err := PostUnmatched("token", "feature/test", nil, "")
		if err == nil || !strings.Contains(err.Error(), "missing GITHUB_REPOSITORY") {
			t.Errorf("PostUnmatched() error = %v, want missing GITHUB_REPOSITORY", err)
		}
	})

	t.Run("requires a pull request", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GITHUB_EVENT_PATH", "")
		t.Setenv("GITHUB_REF", "refs/heads/main")

		err := PostUnmatched("token", "feature/test", nil, "")
		if err == nil || !strings.Contains(err.Error(), "no pull request") {
			t.Errorf("PostUnmatched() error = %v, want no pull request", err)
		}
	})

	t.Run("posts a comment", func(t *testing.T) {
		var posted string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte("[]"))
			case http.MethodPost:
				var payload struct {
					Body string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("failed to decode payload: %v", err)
				}
				posted = payload.Body
				w.WriteHeader(http.StatusCreated)
			}
		}))
		defer server.Close()

		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GITHUB_EVENT_PATH", eventPath)
		t.Setenv("GITHUB_API_URL", server.URL)

		if err := PostUnmatched("token", "feature/test", []string{"release/.*"}, "Blocked {branch}: {patterns}"); err != nil {
			t.Fatalf("PostUnmatched() returned error: %v", err)
		}
		if want := "Blocked feature/test: `release/.*`"; !strings.Contains(posted, want) {
			t.Errorf("comment does not contain %q:\n%s", want, posted)
		}
	})
}

func TestAPIBaseURL(t *testing.T) {
	t.Run("defaults to github.com", func(t *testing.T) {
		t.Setenv("GITHUB_API_URL", "")
		if got := apiBaseURL(); got != "https://api.github.com" {
			t.Errorf("apiBaseURL() = %q, want %q", got, "https://api.github.com")
		}
	})

	t.Run("removes a trailing slash", func(t *testing.T) {
		t.Setenv("GITHUB_API_URL", "https://github.example.com/api/v3/")
		if got := apiBaseURL(); got != "https://github.example.com/api/v3" {
			t.Errorf("apiBaseURL() = %q, want %q", got, "https://github.example.com/api/v3")
		}
	})
}
