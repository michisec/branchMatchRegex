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

//go:embed assets/default-patterns.yml
var defaultPatternsYAML []byte

var (
	wildcardSpecialChars = regexp.MustCompile(`[.+?^${}()|[\]\\]`)
	urlPattern           = regexp.MustCompile(`^https?://[^\s]+`)
)

func main() {
	action := githubactions.New()
	if err := run(action); err != nil {
		action.Fatalf("%s", err)
	}
}

func run(action *githubactions.Action) error {
	if ref := os.Getenv("GITHUB_ACTION_REF"); ref == "v1" || ref == "v2" {
		action.Warningf("You are using a deprecated floating tag (%s). This tag is no longer updated. "+
			"Please pin to a specific SemVer tag (e.g., v3.0.0) or a commit SHA for better security and stability.", ref)
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

	if useDefaultPatterns {
		defaultPatterns, err := getDefaultPatterns(useWildcardPatterns)
		if err != nil {
			return err
		}
		allPatterns = append(allPatterns, defaultPatterns...)
	}

	if path != "" {
		pathPatterns, err := getPatternsFromPath(path, useWildcardPatterns)
		if err != nil {
			return err
		}
		allPatterns = append(allPatterns, pathPatterns...)
	}

	for _, pattern := range allPatterns {
		action.Infof("Checking branch '%s' against pattern: %s", branch, pattern)

		re, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}

		if re.MatchString(branch) {
			action.Infof("Branch '%s' matches pattern: %s", branch, pattern)
			return nil
		}
	}

	if failOnUnmatchedPattern {
		return fmt.Errorf("Branch '%s' does not match any patterns", branch)
	}

	action.Warningf("Branch '%s' does not match any patterns", branch)
	return nil
}

func getDefaultPatterns(useWildcard bool) ([]string, error) {
	patterns, err := parsePatterns(defaultPatternsYAML, useWildcard)
	if err != nil {
		return nil, fmt.Errorf("Failed to get default patterns: %w", err)
	}
	return patterns, nil
}

func getPatternsFromPath(path string, useWildcard bool) ([]string, error) {
	var content []byte
	isURL := urlPattern.MatchString(path)

	if isURL {
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

	patterns, err := parsePatterns(content, useWildcard)
	if err != nil {
		if isURL {
			return nil, fmt.Errorf("Failed to parse patterns from URL: %w", err)
		}
		return nil, fmt.Errorf("Failed to get patterns from file: %w", err)
	}
	return patterns, nil
}

func parsePatterns(content []byte, useWildcard bool) ([]string, error) {
	var raw []string
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	return convertPatterns(raw, useWildcard), nil
}

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

func convertToWildcardPattern(pattern string) string {
	cleanPattern := wildcardSpecialChars.ReplaceAllString(pattern, `\$0`)
	cleanPattern = strings.ReplaceAll(cleanPattern, "*", ".*")
	return "^" + cleanPattern + "$"
}
