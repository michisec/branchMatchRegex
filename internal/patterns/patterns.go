package patterns

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	wildcardSpecialChars = regexp.MustCompile(`[.+?^${}()|[\]\\]`)
	urlPattern           = regexp.MustCompile(`^https?://[^\s]+`)
)

func ParseInline(input string) []string {
	var result []string
	for _, p := range strings.Split(input, "\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "- ") {
			p = strings.TrimSpace(p[2:])
		}
		result = append(result, p)
	}
	return result
}

func Parse(content []byte, useWildcard bool) ([]string, error) {
	var raw []string
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	return Convert(raw, useWildcard), nil
}

func Load(path string, useWildcard bool) ([]string, error) {
	content, isURL, err := read(path)
	if err != nil {
		return nil, err
	}

	parsed, err := Parse(content, useWildcard)
	if err != nil {
		if isURL {
			return nil, fmt.Errorf("failed to parse patterns from URL: %w", err)
		}
		return nil, fmt.Errorf("failed to get patterns from file: %w", err)
	}
	return parsed, nil
}

func Convert(all []string, useWildcard bool) []string {
	converted := make([]string, 0, len(all))
	for _, p := range all {
		if useWildcard {
			p = ConvertWildcard(p)
		}
		converted = append(converted, p)
	}
	return converted
}

func ConvertWildcard(pattern string) string {
	cleanPattern := wildcardSpecialChars.ReplaceAllString(pattern, `\$0`)
	cleanPattern = strings.ReplaceAll(cleanPattern, "*", ".*")
	return "^" + cleanPattern + "$"
}

func read(path string) ([]byte, bool, error) {
	if urlPattern.MatchString(path) {
		resp, err := http.Get(path)
		if err != nil {
			return nil, true, fmt.Errorf("failed to fetch patterns from URL: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, true, fmt.Errorf("failed to fetch patterns from URL: %s", resp.Status)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, true, fmt.Errorf("failed to fetch patterns from URL: %w", err)
		}
		return content, true, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get patterns from file: %w", err)
	}
	return content, false, nil
}
