package main

import (
	_ "embed"

	githubactions "github.com/sethvargo/go-githubactions"

	"github.com/IamPekka058/branchMatchRegex/internal/runner"
)

//go:embed assets/default-patterns.yml
var defaultPatternsYAML []byte

func main() {
	action := githubactions.New()
	if err := runner.Run(action, defaultPatternsYAML); err != nil {
		action.Fatalf("%s", err)
	}
}
