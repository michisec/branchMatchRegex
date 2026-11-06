package patterns

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestConvertWildcard(t *testing.T) {
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
			if got := ConvertWildcard(tt.pattern); got != tt.want {
				t.Errorf("ConvertWildcard(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestParseInline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single pattern", "feature/*", []string{"feature/*"}},
		{"multiple lines", "feature/*\nfix/*", []string{"feature/*", "fix/*"}},
		{"yaml style", "- feature/*\n- fix/*", []string{"feature/*", "fix/*"}},
		{"trims whitespace and skips empty lines", "  feature/*  \n\n\tfix/*\t", []string{"feature/*", "fix/*"}},
		{"empty input", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInline(tt.input)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseInline(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Run("parses regex patterns", func(t *testing.T) {
		got, err := Parse([]byte("- feature/.*\n- bugfix/.*\n"), false)
		if err != nil {
			t.Fatalf("Parse() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if !slices.Equal(got, want) {
			t.Errorf("Parse() = %v, want %v", got, want)
		}
	})

	t.Run("parses and converts wildcard patterns", func(t *testing.T) {
		got, err := Parse([]byte("- feature/*\n- bugfix/*\n"), true)
		if err != nil {
			t.Fatalf("Parse() returned unexpected error: %v", err)
		}
		want := []string{"^feature/.*$", "^bugfix/.*$"}
		if !slices.Equal(got, want) {
			t.Errorf("Parse() = %v, want %v", got, want)
		}
	})

	t.Run("returns an error for invalid YAML", func(t *testing.T) {
		if _, err := Parse([]byte("patterns:\n  - feature/*\n"), false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestLoad(t *testing.T) {
	writeFile := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "patterns.yml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write patterns file: %v", err)
		}
		return path
	}

	t.Run("reads patterns from a file", func(t *testing.T) {
		got, err := Load(writeFile(t, "- feature/.*\n- bugfix/.*\n"), false)
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if !slices.Equal(got, want) {
			t.Errorf("Load() = %v, want %v", got, want)
		}
	})

	t.Run("reads wildcard patterns from a file", func(t *testing.T) {
		got, err := Load(writeFile(t, "- feature/*\n- bugfix/*\n"), true)
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		want := []string{"^feature/.*$", "^bugfix/.*$"}
		if !slices.Equal(got, want) {
			t.Errorf("Load() = %v, want %v", got, want)
		}
	})

	t.Run("reads patterns from a URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("- feature/.*\n- bugfix/.*\n"))
		}))
		defer server.Close()

		got, err := Load(server.URL, false)
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		want := []string{"feature/.*", "bugfix/.*"}
		if !slices.Equal(got, want) {
			t.Errorf("Load() = %v, want %v", got, want)
		}
	})

	t.Run("reads wildcard patterns from a URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("- feature/*\n- bugfix/*\n"))
		}))
		defer server.Close()

		got, err := Load(server.URL, true)
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		want := []string{"^feature/.*$", "^bugfix/.*$"}
		if !slices.Equal(got, want) {
			t.Errorf("Load() = %v, want %v", got, want)
		}
	})

	t.Run("returns an error when the URL request fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		if _, err := Load(server.URL, false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error when the URL cannot be reached", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("- feature/.*\n"))
		}))
		url := server.URL
		server.Close()

		if _, err := Load(url, false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error for invalid YAML from a URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("patterns:\n  - feature/.*\n"))
		}))
		defer server.Close()

		if _, err := Load(server.URL, false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error when the URL response is truncated", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "1024")
			_, _ = w.Write([]byte("- feature/.*\n"))
		}))
		defer server.Close()

		if _, err := Load(server.URL, false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error for invalid YAML in a file", func(t *testing.T) {
		if _, err := Load(writeFile(t, "patterns:\n  - feature/.*\n"), false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns an error when the file cannot be read", func(t *testing.T) {
		if _, err := Load(filepath.Join(t.TempDir(), "missing.yml"), false); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
