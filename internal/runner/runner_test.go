package runner

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	githubactions "github.com/sethvargo/go-githubactions"
)

var defaultPatterns = []byte("- feature/.*\n- bugfix/.*\n")

func setInputs(t *testing.T, inputs map[string]string) {
	t.Helper()

	names := []string{
		"patterns",
		"path",
		"failOnUnmatchedPattern",
		"commentOnUnmatchedPattern",
		"comment",
		"github-token",
		"useWildcardPatterns",
		"useDefaultPatterns",
		"branch",
	}
	for _, name := range names {
		t.Setenv("INPUT_"+strings.ToUpper(name), inputs[name])
	}

	for _, name := range []string{
		"GH_TOKEN",
		"GITHUB_API_URL",
		"GITHUB_EVENT_PATH",
		"GITHUB_REF",
		"GITHUB_REPOSITORY",
		"GITHUB_TOKEN",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("GITHUB_ACTION_REF", "")
}

func runAction(t *testing.T, inputs map[string]string, defaults []byte) (string, error) {
	t.Helper()

	setInputs(t, inputs)

	var out bytes.Buffer
	action := githubactions.New(githubactions.WithWriter(&out))
	err := Run(action, defaults)
	return out.String(), err
}

func runWithRef(t *testing.T, ref string) string {
	t.Helper()

	setInputs(t, map[string]string{})
	t.Setenv("GITHUB_ACTION_REF", ref)

	var out bytes.Buffer
	if err := Run(githubactions.New(githubactions.WithWriter(&out)), nil); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	return out.String()
}

func TestReadInputs(t *testing.T) {
	setInputs(t, map[string]string{
		"patterns":                  "feature/.*",
		"path":                      "patterns.yml",
		"failOnUnmatchedPattern":    "true",
		"commentOnUnmatchedPattern": "true",
		"comment":                   "custom comment",
		"github-token":              "token",
		"useWildcardPatterns":       "true",
		"useDefaultPatterns":        "true",
		"branch":                    "feature/test",
	})

	got := ReadInputs(githubactions.New())
	want := Inputs{
		Patterns:            "feature/.*",
		Path:                "patterns.yml",
		Branch:              "feature/test",
		Token:               "token",
		FailOnUnmatched:     true,
		CommentOnUnmatched:  true,
		Comment:             "custom comment",
		UseWildcardPatterns: true,
		UseDefaultPatterns:  true,
	}

	if got != want {
		t.Errorf("ReadInputs() = %+v, want %+v", got, want)
	}
}

func TestCollect(t *testing.T) {
	t.Run("collects inline patterns", func(t *testing.T) {
		got, err := Collect(Inputs{Patterns: "feature/.*\n- bugfix/.*"}, nil)
		if err != nil {
			t.Fatalf("Collect() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if !slices.Equal(got, want) {
			t.Errorf("Collect() = %v, want %v", got, want)
		}
	})

	t.Run("collects default patterns", func(t *testing.T) {
		got, err := Collect(Inputs{UseDefaultPatterns: true}, defaultPatterns)
		if err != nil {
			t.Fatalf("Collect() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if !slices.Equal(got, want) {
			t.Errorf("Collect() = %v, want %v", got, want)
		}
	})

	t.Run("collects patterns from a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "patterns.yml")
		if err := os.WriteFile(path, []byte("- bugfix/.*\n- hotfix/.*\n"), 0o600); err != nil {
			t.Fatalf("failed to write patterns file: %v", err)
		}

		got, err := Collect(Inputs{Path: path}, nil)
		if err != nil {
			t.Fatalf("Collect() returned unexpected error: %v", err)
		}
		want := []string{"bugfix/.*", "hotfix/.*"}
		if !slices.Equal(got, want) {
			t.Errorf("Collect() = %v, want %v", got, want)
		}
	})

	t.Run("returns an error for invalid default patterns", func(t *testing.T) {
		if _, err := Collect(Inputs{UseDefaultPatterns: true}, []byte("patterns:\n  - feature/.*\n")); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error when the patterns file cannot be read", func(t *testing.T) {
		if _, err := Collect(Inputs{Path: filepath.Join(t.TempDir(), "missing.yml")}, nil); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestMatch(t *testing.T) {
	t.Run("matches a branch", func(t *testing.T) {
		matched, err := Match("^feature/.*$", "feature/test")
		if err != nil {
			t.Fatalf("Match() returned unexpected error: %v", err)
		}
		if !matched {
			t.Error("expected a match")
		}
	})

	t.Run("does not match a branch", func(t *testing.T) {
		matched, err := Match("^fix/.*$", "feature/test")
		if err != nil {
			t.Fatalf("Match() returned unexpected error: %v", err)
		}
		if matched {
			t.Error("expected no match")
		}
	})

	t.Run("returns an error for an invalid regex", func(t *testing.T) {
		if _, err := Match("[", "feature/test"); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestRun(t *testing.T) {
	t.Run("matches branch with single inline pattern", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns": "feature/.*",
			"branch":   "feature/test",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Branch 'feature/test' matches pattern: feature/.*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("fails when branch does not match and failOnUnmatchedPattern is true", func(t *testing.T) {
		_, err := runAction(t, map[string]string{
			"patterns":               "release/.*",
			"failOnUnmatchedPattern": "true",
			"branch":                 "feature/test",
		}, nil)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "branch 'feature/test' does not match any patterns"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	})

	t.Run("does not fail or comment by default", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns": "release/.*",
			"branch":   "feature/test",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "::warning::Branch 'feature/test' does not match any patterns"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
		if strings.Contains(out, "Could not post comment") {
			t.Errorf("output unexpectedly contains a comment warning:\n%s", out)
		}
	})

	t.Run("matches branch with default patterns", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"useDefaultPatterns": "true",
			"branch":             "feature/test",
		}, defaultPatterns)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Branch 'feature/test' matches pattern: feature/.*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("matches branch with patterns from file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "patterns.yml")
		if err := os.WriteFile(path, []byte("- bugfix/.*\n- hotfix/.*\n"), 0o600); err != nil {
			t.Fatalf("failed to write patterns file: %v", err)
		}

		out, err := runAction(t, map[string]string{
			"path":   path,
			"branch": "bugfix/123",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Branch 'bugfix/123' matches pattern: bugfix/.*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("handles YAML-style pattern lists", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns": "- feature/.*\n- bugfix/.*",
			"branch":   "bugfix/123",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Branch 'bugfix/123' matches pattern: bugfix/.*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("handles empty patterns gracefully", func(t *testing.T) {
		out, err := runAction(t, map[string]string{}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "does not match any patterns"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("returns an error when the patterns file cannot be read", func(t *testing.T) {
		_, err := runAction(t, map[string]string{
			"path":   filepath.Join(t.TempDir(), "missing.yml"),
			"branch": "feature/test",
		}, nil)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "failed to get patterns from file"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	})

	t.Run("supports useWildcardPatterns", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns":            "feature/*",
			"useWildcardPatterns": "true",
			"branch":              "feature/123",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Branch 'feature/123' matches pattern: feature/*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("returns an error for an invalid regex pattern", func(t *testing.T) {
		_, err := runAction(t, map[string]string{
			"patterns": "[invalid-regex",
			"branch":   "feature/123",
		}, nil)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "error parsing regexp"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	})

	t.Run("posts a comment when the branch does not match", func(t *testing.T) {
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
					t.Errorf("failed to decode request: %v", err)
				}
				posted = payload.Body
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":1}`))
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		defer server.Close()

		eventPath := filepath.Join(t.TempDir(), "event.json")
		if err := os.WriteFile(eventPath, []byte(`{"pull_request":{"number":5}}`), 0o600); err != nil {
			t.Fatalf("failed to write event: %v", err)
		}

		setInputs(t, map[string]string{
			"patterns":                  "release/.*",
			"failOnUnmatchedPattern":    "false",
			"commentOnUnmatchedPattern": "true",
			"comment":                   "Branch {branch} does not match: {patterns}",
			"github-token":              "token",
			"branch":                    "feature/test",
		})
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GITHUB_EVENT_PATH", eventPath)
		t.Setenv("GITHUB_API_URL", server.URL)

		var out bytes.Buffer
		err := Run(githubactions.New(githubactions.WithWriter(&out)), nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Branch 'feature/test' does not match any patterns"; !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
		if want := "Branch feature/test does not match: `release/.*`"; !strings.Contains(posted, want) {
			t.Errorf("comment does not contain %q:\n%s", want, posted)
		}
		if want := "<!-- branchMatchRegex:unmatched -->"; !strings.Contains(posted, want) {
			t.Errorf("comment does not contain marker %q:\n%s", want, posted)
		}
	})

	t.Run("warns when the comment cannot be posted", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns":                  "release/.*",
			"failOnUnmatchedPattern":    "false",
			"commentOnUnmatchedPattern": "true",
			"branch":                    "feature/test",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Could not post comment on pull request: missing github token"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("explains read-only tokens when the comment is forbidden", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
		}))
		defer server.Close()

		eventPath := filepath.Join(t.TempDir(), "event.json")
		if err := os.WriteFile(eventPath, []byte(`{"pull_request":{"number":5}}`), 0o600); err != nil {
			t.Fatalf("failed to write event: %v", err)
		}

		setInputs(t, map[string]string{
			"patterns":                  "release/.*",
			"commentOnUnmatchedPattern": "true",
			"github-token":              "token",
			"branch":                    "feature/test",
		})
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GITHUB_EVENT_PATH", eventPath)
		t.Setenv("GITHUB_API_URL", server.URL)

		var out bytes.Buffer
		err := Run(githubactions.New(githubactions.WithWriter(&out)), nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "read-only for pull requests from forks"; !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	})

	t.Run("fails after posting a comment when configured", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns":                  "release/.*",
			"failOnUnmatchedPattern":    "true",
			"commentOnUnmatchedPattern": "true",
			"branch":                    "feature/test",
		}, nil)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "Could not post comment on pull request"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("does not post a comment when the branch matches", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns":                  "feature/.*",
			"commentOnUnmatchedPattern": "true",
			"branch":                    "feature/test",
		}, nil)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if want := "Could not post comment"; strings.Contains(out, want) {
			t.Errorf("output unexpectedly contains %q:\n%s", want, out)
		}
	})
}

func TestRunVersionWarning(t *testing.T) {
	for _, ref := range []string{"v1", "v2"} {
		t.Run("warns for "+ref, func(t *testing.T) {
			out := runWithRef(t, ref)
			if want := "You are using a deprecated floating tag (" + ref + ")"; !strings.Contains(out, want) {
				t.Errorf("output does not contain %q:\n%s", want, out)
			}
		})
	}

	t.Run("does not warn for a strict SemVer tag", func(t *testing.T) {
		out := runWithRef(t, "v3.0.0")
		if want := "deprecated floating tag"; strings.Contains(out, want) {
			t.Errorf("output unexpectedly contains %q:\n%s", want, out)
		}
	})
}
