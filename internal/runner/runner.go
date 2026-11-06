package runner

import (
	"fmt"
	"os"
	"regexp"

	githubactions "github.com/sethvargo/go-githubactions"

	"github.com/michisec/branchMatchRegex/internal/patterns"
)

type Inputs struct {
	Patterns            string
	Path                string
	Branch              string
	FailOnUnmatched     bool
	UseWildcardPatterns bool
	UseDefaultPatterns  bool
}

func ReadInputs(action *githubactions.Action) Inputs {
	return Inputs{
		Patterns:            action.GetInput("patterns"),
		Path:                action.GetInput("path"),
		Branch:              action.GetInput("branch"),
		FailOnUnmatched:     action.GetInput("failOnUnmatchedPattern") == "true",
		UseWildcardPatterns: action.GetInput("useWildcardPatterns") == "true",
		UseDefaultPatterns:  action.GetInput("useDefaultPatterns") == "true",
	}
}

func Run(action *githubactions.Action, defaultPatterns []byte) error {
	warnDeprecatedTag(action)

	inputs := ReadInputs(action)
	logInputs(action, inputs)
	action.Infof("Checking branch '%s' against provided patterns...", inputs.Branch)

	allPatterns, err := Collect(inputs, defaultPatterns)
	if err != nil {
		return err
	}

	for _, pattern := range allPatterns {
		action.Infof("Checking branch '%s' against pattern: %s", inputs.Branch, pattern)

		matched, err := Match(pattern, inputs.Branch)
		if err != nil {
			return err
		}

		if matched {
			action.Infof("Branch '%s' matches pattern: %s", inputs.Branch, pattern)
			return nil
		}
	}

	if inputs.FailOnUnmatched {
		return fmt.Errorf("Branch '%s' does not match any patterns", inputs.Branch)
	}

	action.Warningf("Branch '%s' does not match any patterns", inputs.Branch)
	return nil
}

func Collect(inputs Inputs, defaultPatterns []byte) ([]string, error) {
	var allPatterns []string

	allPatterns = append(allPatterns, patterns.ParseInline(inputs.Patterns)...)

	if inputs.UseDefaultPatterns {
		defaults, err := patterns.Parse(defaultPatterns, inputs.UseWildcardPatterns)
		if err != nil {
			return nil, fmt.Errorf("Failed to get default patterns: %w", err)
		}
		allPatterns = append(allPatterns, defaults...)
	}

	if inputs.Path != "" {
		fromPath, err := patterns.Load(inputs.Path, inputs.UseWildcardPatterns)
		if err != nil {
			return nil, err
		}
		allPatterns = append(allPatterns, fromPath...)
	}

	return allPatterns, nil
}

func Match(pattern, branch string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(branch), nil
}

func warnDeprecatedTag(action *githubactions.Action) {
	ref := os.Getenv("GITHUB_ACTION_REF")
	if ref != "v1" && ref != "v2" {
		return
	}

	action.Warningf("You are using a deprecated floating tag (%s). This tag is no longer updated. "+
		"Please pin to a specific SemVer tag (e.g., v3.0.0) or a commit SHA for better security and stability.", ref)
}

func logInputs(action *githubactions.Action, inputs Inputs) {
	action.Debugf("Patterns: %s", inputs.Patterns)
	action.Debugf("Path: %s", inputs.Path)
	action.Debugf("Fail on unmatched pattern: %t", inputs.FailOnUnmatched)
	action.Debugf("Use wildcard patterns: %t", inputs.UseWildcardPatterns)
	action.Debugf("Use default patterns: %t", inputs.UseDefaultPatterns)
	action.Debugf("Branch: %s", inputs.Branch)
}
