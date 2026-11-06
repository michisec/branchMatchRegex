package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	githubactions "github.com/sethvargo/go-githubactions"
)

func setInputs(t *testing.T, inputs map[string]string) {
	t.Helper()

	names := []string{
		"patterns",
		"path",
		"failOnUnmatchedPattern",
		"useWildcardPatterns",
		"useDefaultPatterns",
		"branch",
	}
	for _, name := range names {
		t.Setenv("INPUT_"+strings.ToUpper(name), inputs[name])
	}
	t.Setenv("GITHUB_ACTION_REF", "")
}

func runAction(t *testing.T, inputs map[string]string) (string, error) {
	t.Helper()

	setInputs(t, inputs)

	var out bytes.Buffer
	action := githubactions.New(githubactions.WithWriter(&out))
	err := run(action)
	return out.String(), err
}

func runWithRef(t *testing.T, ref string) string {
	t.Helper()

	setInputs(t, map[string]string{})
	t.Setenv("GITHUB_ACTION_REF", ref)

	var out bytes.Buffer
	if err := run(githubactions.New(githubactions.WithWriter(&out))); err != nil {
		t.Fatalf("run() returned unexpected error: %v", err)
	}
	return out.String()
}

func TestRun(t *testing.T) {
	t.Run("matches branch with single inline pattern", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns": "feature/.*",
			"branch":   "feature/test",
		})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
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
		})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "Branch 'feature/test' does not match any patterns"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	})

	t.Run("does not fail when failOnUnmatchedPattern is false", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns":               "release/.*",
			"failOnUnmatchedPattern": "false",
			"branch":                 "feature/test",
		})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
		if want := "::warning::Branch 'feature/test' does not match any patterns"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("matches branch with default patterns", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"useDefaultPatterns": "true",
			"branch":             "feature/test",
		})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
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
		})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
		if want := "Branch 'bugfix/123' matches pattern: bugfix/.*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("handles YAML-style pattern lists", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns": "- feature/.*\n- bugfix/.*",
			"branch":   "bugfix/123",
		})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
		if want := "Branch 'bugfix/123' matches pattern: bugfix/.*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("handles empty patterns gracefully", func(t *testing.T) {
		out, err := runAction(t, map[string]string{})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
		if want := "does not match any patterns"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("returns an error when the patterns file cannot be read", func(t *testing.T) {
		_, err := runAction(t, map[string]string{
			"path":   filepath.Join(t.TempDir(), "missing.yml"),
			"branch": "feature/test",
		})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "Failed to get patterns from file"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	})

	t.Run("supports useWildcardPatterns", func(t *testing.T) {
		out, err := runAction(t, map[string]string{
			"patterns":            "feature/*",
			"useWildcardPatterns": "true",
			"branch":              "feature/123",
		})
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
		if want := "Branch 'feature/123' matches pattern: feature/*"; !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	})

	t.Run("returns an error for an invalid regex pattern", func(t *testing.T) {
		_, err := runAction(t, map[string]string{
			"patterns": "[invalid-regex",
			"branch":   "feature/123",
		})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if want := "error parsing regexp"; !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
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

func TestConvertToWildcardPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{"simple wildcard", "feature/*", "^feature/.*$"},
		{"multiple wildcards", "feature/*/test", "^feature/.*/test$"},
		{"special regex characters", "feature/(test)", `^feature/\(test\)$`},
		{"empty pattern", "", "^$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertToWildcardPattern(tt.pattern); got != tt.want {
				t.Errorf("convertToWildcardPattern(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestGetDefaultPatterns(t *testing.T) {
	t.Run("returns regex patterns", func(t *testing.T) {
		patterns, err := getDefaultPatterns(false)
		if err != nil {
			t.Fatalf("getDefaultPatterns(false) returned unexpected error: %v", err)
		}
		if len(patterns) == 0 {
			t.Fatal("expected default patterns, got none")
		}
		if got, want := patterns[0], "feature/.*"; got != want {
			t.Errorf("patterns[0] = %q, want %q", got, want)
		}
	})

	t.Run("returns wildcard patterns converted to regex", func(t *testing.T) {
		patterns, err := getDefaultPatterns(true)
		if err != nil {
			t.Fatalf("getDefaultPatterns(true) returned unexpected error: %v", err)
		}
		if len(patterns) == 0 {
			t.Fatal("expected default patterns, got none")
		}
		if got, want := patterns[0], `^feature/\..*$`; got != want {
			t.Errorf("patterns[0] = %q, want %q", got, want)
		}
	})
}

func TestParsePatterns(t *testing.T) {
	t.Run("parses regex patterns", func(t *testing.T) {
		patterns, err := parsePatterns([]byte("- feature/.*\n- bugfix/.*\n"), false)
		if err != nil {
			t.Fatalf("parsePatterns() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if strings.Join(patterns, ",") != strings.Join(want, ",") {
			t.Errorf("parsePatterns() = %v, want %v", patterns, want)
		}
	})

	t.Run("parses and converts wildcard patterns", func(t *testing.T) {
		patterns, err := parsePatterns([]byte("- feature/*\n- bugfix/*\n"), true)
		if err != nil {
			t.Fatalf("parsePatterns() returned unexpected error: %v", err)
		}
		want := []string{"^feature/.*$", "^bugfix/.*$"}
		if strings.Join(patterns, ",") != strings.Join(want, ",") {
			t.Errorf("parsePatterns() = %v, want %v", patterns, want)
		}
	})

	t.Run("returns an error for invalid YAML", func(t *testing.T) {
		if _, err := parsePatterns([]byte("patterns:\n  - feature/*\n"), false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestGetPatternsFromPath(t *testing.T) {
	t.Run("reads patterns from a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "patterns.yml")
		if err := os.WriteFile(path, []byte("- feature/.*\n- bugfix/.*\n"), 0o600); err != nil {
			t.Fatalf("failed to write patterns file: %v", err)
		}

		patterns, err := getPatternsFromPath(path, false)
		if err != nil {
			t.Fatalf("getPatternsFromPath() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if strings.Join(patterns, ",") != strings.Join(want, ",") {
			t.Errorf("getPatternsFromPath() = %v, want %v", patterns, want)
		}
	})

	t.Run("reads wildcard patterns from a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "patterns.yml")
		if err := os.WriteFile(path, []byte("- feature/*\n- bugfix/*\n"), 0o600); err != nil {
			t.Fatalf("failed to write patterns file: %v", err)
		}

		patterns, err := getPatternsFromPath(path, true)
		if err != nil {
			t.Fatalf("getPatternsFromPath() returned unexpected error: %v", err)
		}
		want := []string{"^feature/.*$", "^bugfix/.*$"}
		if strings.Join(patterns, ",") != strings.Join(want, ",") {
			t.Errorf("getPatternsFromPath() = %v, want %v", patterns, want)
		}
	})

	t.Run("reads patterns from a URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("- feature/.*\n- bugfix/.*\n"))
		}))
		defer server.Close()

		patterns, err := getPatternsFromPath(server.URL, false)
		if err != nil {
			t.Fatalf("getPatternsFromPath() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if strings.Join(patterns, ",") != strings.Join(want, ",") {
			t.Errorf("getPatternsFromPath() = %v, want %v", patterns, want)
		}
	})

	t.Run("reads wildcard patterns from a URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("- feature/*\n- bugfix/*\n"))
		}))
		defer server.Close()

		patterns, err := getPatternsFromPath(server.URL, true)
		if err != nil {
			t.Fatalf("getPatternsFromPath() returned unexpected error: %v", err)
		}
		want := []string{"^feature/.*$", "^bugfix/.*$"}
		if strings.Join(patterns, ",") != strings.Join(want, ",") {
			t.Errorf("getPatternsFromPath() = %v, want %v", patterns, want)
		}
	})

	t.Run("returns an error when the URL request fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		if _, err := getPatternsFromPath(server.URL, false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error when the file cannot be read", func(t *testing.T) {
		if _, err := getPatternsFromPath(filepath.Join(t.TempDir(), "missing.yml"), false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
