package main

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	githubactions "github.com/sethvargo/go-githubactions"
	"gopkg.in/yaml.v3"
)

// defaultPatternsYAML holds the default patterns, embedded so the action can be
// distributed as a single binary.
//
//go:embed assets/default-patterns.yml
var defaultPatternsYAML []byte

// wildcardSpecialChars matches every character that has a special meaning in a
// regular expression and must therefore be escaped when converting a wildcard
// pattern. The set mirrors the original TypeScript implementation.
var wildcardSpecialChars = regexp.MustCompile(`[.+?^${}()|[\]\\]`)

// urlPattern detects http(s) URLs, mirroring the original implementation.
var urlPattern = regexp.MustCompile(`^https?://[^\s]+`)

func main() {
	action := githubactions.New()

	// Warn when a deprecated floating tag is used.
	if ref := os.Getenv("GITHUB_ACTION_REF"); ref == "v1" || ref == "v2" {
		action.Warningf(
			"You are using a deprecated floating tag (%s). This tag is no longer updated. "+
				"Please pin to a specific SemVer tag (e.g., v3.0.0) or a commit SHA for better security and stability.",
			ref,
		)
	}

	patterns := action.GetInput("patterns")
	path := action.GetInput("path")
	failOnUnmatchedPattern := action.GetInput("failOnUnmatchedPattern") == "true"
	useWildcardPatterns := action.GetInput("useWildcardPatterns") == "true"
	useDefaultPatterns := action.GetInput("useDefaultPatterns") == "true"
	branch := action.GetInput("branch")

	action.Debugf("Patterns: %s", patterns)
	action.Debugf("Path: %s", path)
	action.Debugf("Fail on unmatched pattern: %t", failOnUnmatchedPattern)
	action.Debugf("Use wildcard patterns: %t", useWildcardPatterns)
	action.Debugf("Use default patterns: %t", useDefaultPatterns)
	action.Debugf("Branch: %s", branch)

	action.Infof("Checking branch '%s' against provided patterns...", branch)

	var allPatterns []string

	// 1. Inline patterns. They are always treated as regular expressions and
	// may be passed as a newline separated or YAML-style list.
	if patterns != "" {
		for _, p := range strings.Split(patterns, "\n") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.HasPrefix(p, "- ") {
				p = strings.TrimSpace(p[2:])
			}
			allPatterns = append(allPatterns, p)
		}
	}

	// 2. Default patterns, if enabled.
	if useDefaultPatterns {
		allPatterns = append(allPatterns, getDefaultPatterns(action, useWildcardPatterns)...)
	}

	// 3. Patterns from a file or URL, if a path was provided.
	if path != "" {
		pathPatterns, err := getPatternsFromPath(path, useWildcardPatterns)
		if err != nil {
			action.Fatalf("%s", err)
		}
		allPatterns = append(allPatterns, pathPatterns...)
	}

	// 4. Match the branch against the collected patterns. The action succeeds
	// as soon as one pattern matches.
	for _, pattern := range allPatterns {
		action.Infof("Checking branch '%s' against pattern: %s", branch, pattern)

		re, err := regexp.Compile(pattern)
		if err != nil {
			action.Fatalf("%s", err)
		}

		if re.MatchString(branch) {
			action.Infof("Branch '%s' matches pattern: %s", branch, pattern)
			return
		}
	}

	if failOnUnmatchedPattern {
		action.Fatalf("Branch '%s' does not match any patterns", branch)
	}
	action.Warningf("Branch '%s' does not match any patterns", branch)
}

// getDefaultPatterns returns the embedded default patterns, converting them to
// regular expressions when wildcard patterns are requested.
func getDefaultPatterns(action *githubactions.Action, useWildcard bool) []string {
	var raw []string
	if err := yaml.Unmarshal(defaultPatternsYAML, &raw); err != nil {
		action.Fatalf("Failed to get default patterns: %s", err)
	}
	return convertPatterns(raw, useWildcard)
}

// getPatternsFromPath reads patterns from a local file or an http(s) URL.
func getPatternsFromPath(path string, useWildcard bool) ([]string, error) {
	var content []byte

	if urlPattern.MatchString(path) {
		resp, err := http.Get(path)
		if err != nil {
			return nil, fmt.Errorf("Failed to fetch patterns from URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("Failed to fetch patterns from URL: %s", resp.Status)
		}

		content, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Failed to fetch patterns from URL: %w", err)
		}
	} else {
		var err error
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("Failed to get patterns from file: %w", err)
		}
	}

	var raw []string
	if err := yaml.Unmarshal(content, &raw); err != nil {
		if urlPattern.MatchString(path) {
			return nil, fmt.Errorf("Failed to parse patterns from URL: %w", err)
		}
		return nil, fmt.Errorf("Failed to get patterns from file: %w", err)
	}

	return convertPatterns(raw, useWildcard), nil
}

// convertPatterns optionally converts each pattern to a wildcard-based regular
// expression.
func convertPatterns(patterns []string, useWildcard bool) []string {
	converted := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if useWildcard {
			p = convertToWildcardPattern(p)
		}
		converted = append(converted, p)
	}
	return converted
}

// convertToWildcardPattern converts a wildcard pattern into an anchored regular
// expression, escaping regex metacharacters and turning "*" into ".*".
func convertToWildcardPattern(pattern string) string {
	cleanPattern := wildcardSpecialChars.ReplaceAllString(pattern, `\$0`)
	cleanPattern = strings.ReplaceAll(cleanPattern, "*", ".*")
	return "^" + cleanPattern + "$"
}
