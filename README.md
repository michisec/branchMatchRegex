# branchMatchRegex

![GitHub release (latest by date)](https://img.shields.io/github/v/release/michisec/branchMatchRegex)
![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/michisec/branchMatchRegex/.github/workflows/test-workflow.yml)
![GitHub](https://img.shields.io/github/license/michisec/branchMatchRegex)

> **What's new in v3?**
>
> - Rewritten in Go and evaluated with the RE2 engine. Lookarounds and
>   backreferences are not supported.
> - Runs from an exact version of the container image published to the GitHub
>   Container Registry, so it requires a Linux runner.
> - Version tags are immutable. Pin an exact release such as `@v3.0.0`; there is
>   no moving `v3` alias. The container image also publishes a mutable `latest`
>   tag that tracks the most recent build, so it is not suitable for
>   reproducible pins. Container image tags omit the `v` prefix (for example
>   `3.0.0`), matching the image referenced by `action.yml`.
> - **Behavior change:** unmatched branches no longer fail the workflow by
>   default. Set `failOnUnmatchedPattern: true` to keep the v2 behavior.
>   A configurable comment is posted on the pull request by default; set
>   `commentOnUnmatchedPattern: false` to disable it.

`branchMatchRegex` is a GitHub Action that checks if the current branch name
matches one or more specified patterns (regex or wildcard). This is especially
useful for enforcing branch naming conventions in your repositories.

> **Note:** The action is implemented in Go and evaluates patterns with the RE2
> engine. Lookarounds and backreferences are not supported; such patterns fail
> the action with an explicit error.

> **Note:** The action runs from a prebuilt container image published to the
> GitHub Container Registry, so it requires a Linux runner. The image is pulled
> automatically and matches the exact version tag you use for the action.

> If you use `useDefaultPatterns: true`, see
> [DEFAULT_PATTERNS.md](./DEFAULT_PATTERNS.md) for a detailed explanation of the
> default branch patterns.

## Inputs

| Name                        | Description                                                                                    | Required | Default                  |
| --------------------------- | ---------------------------------------------------------------------------------------------- | -------- | ------------------------ |
| `patterns`                  | The (regex or wildcard) patterns to match against the branch. Multiple patterns, one per line. | No       | ""                       |
| `path`                      | Path or URL to a file containing patterns (one per line).                                      | No       | ""                       |
| `useDefaultPatterns`        | Additionally use default patterns. See [DEFAULT_PATTERNS.md](./DEFAULT_PATTERNS.md).           | No       | false                    |
| `failOnUnmatchedPattern`    | Fail the action if the branch does not match any pattern.                                      | No       | false                    |
| `commentOnUnmatchedPattern` | Post or update a pull request comment if the branch does not match any pattern.                | No       | true                     |
| `comment`                   | Comment template. Supports `{branch}` and `{patterns}`. Empty uses the default text.           | No       | ""                       |
| `github-token`              | Token used to post the comment.                                                                | No       | `${{ github.token }}`    |
| `useWildcardPatterns`       | Treat patterns as wildcards (e.g. `feature/*`) instead of regex.                               | No       | false                    |
| `branch`                    | The branch to check.                                                                           | Yes      | `${{ github.head_ref }}` |

**Note:** All specified pattern sources (`patterns`, `useDefaultPatterns`,
`path`) are combined. The order is: `patterns`, then default patterns, then
file/URL. You can use any combination of sources at the same time.

### Unmatched branches

By default, a branch that does not match any pattern does **not** fail the
workflow. The action logs a warning and posts (or updates) a comment on the pull
request. Set `commentOnUnmatchedPattern: false` to disable the comment.

`failOnUnmatchedPattern` and `commentOnUnmatchedPattern` are independent:

| `failOnUnmatchedPattern` | `commentOnUnmatchedPattern` | Result                                            |
| ------------------------ | --------------------------- | ------------------------------------------------- |
| `false` (default)        | `true` (default)            | Comment is posted or updated; workflow succeeds.  |
| `false`                  | `false`                     | Workflow succeeds with a warning.                 |
| `true`                   | `true` (default)            | Comment is posted or updated; workflow fails.     |
| `true`                   | `false`                     | Workflow fails.                                   |

To post comments, grant `pull-requests: write`. The `github-token` input defaults
to `${{ github.token }}`, so no additional token configuration is required:

```yaml
permissions:
  contents: read
  pull-requests: write

steps:
  - uses: michisec/branchMatchRegex@v3.0.0
    with:
      patterns: |
        feature/.*
        fix/.*
```

Comments are only posted for pull request events. If no pull request or token is
available, the action logs a warning and continues; the comment failure does not
change whether the workflow fails.

### Commenting from fork pull requests

For pull requests from forks, the default `${{ github.token }}` on a
`pull_request` event is read-only. The GitHub API rejects the comment with
`403 Resource not accessible by integration`, so the action logs a warning and
the branch check still succeeds.

To post comments for fork pull requests, run the comment from a trusted,
write-enabled context. The action only reads its inputs and calls the GitHub
API, so it never executes code from the pull request. Common options are:

- **`pull_request_target`** – runs in the base repository context with a write
  token. Do not check out or run code from the pull request head in this
  workflow.
- **`workflow_run`** – run a separate workflow after the pull request workflow
  and post the comment from there.
- **`github-token` input** – pass a PAT or GitHub App token with
  `pull-requests: write`, stored as a repository secret.

Example using `pull_request_target`:

```yaml
on:
  pull_request_target:

permissions:
  contents: read
  pull-requests: write

jobs:
  branch:
    runs-on: ubuntu-latest
    steps:
      - uses: michisec/branchMatchRegex@v3.0.0
        with:
          branch: ${{ github.head_ref }}
          patterns: |
            feature/.*
            fix/.*
```

> **Security note:** `pull_request_target` has access to repository secrets and
> a write token. Never check out or execute code from the pull request in such a
> workflow.

The comment template supports `{branch}` and `{patterns}`:

```yaml
with:
  comment: |
    Branch `{branch}` does not match the allowed patterns: {patterns}
```

The default comment is:

> Branch `{branch}` does not match any of the configured patterns.

**Tip:** The `patterns` input supports both plain lists and YAML-style lists.
For example, both of the following are valid:

```yaml
patterns: |
  feature/*
  fix/*
```

or

```yaml
patterns: |
  - feature/*
  - fix/*
```

## Example Usage

### Example 1: Single inline pattern

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    patterns: "feature/.*"
```

### Example 2: Multiple inline patterns (one per line)

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    patterns: |
      feature/.*
      bugfix/.*
      hotfix/.*
```

### Example 3: Patterns from a file

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    path: "branch-patterns.yml"
```

The file `branch-patterns.yml` could look like this:

```
- feature/.*
- bugfix/.*
- hotfix/.*
```

### Example 4: Use default patterns additionally

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    useDefaultPatterns: true
```

### Example 5: Combine multiple sources

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    patterns: |
      feature/.*
      bugfix/.*
    useDefaultPatterns: true
    path: "branch-patterns.yml"
```

### Example 6: Use wildcard patterns

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    patterns: |
      feature/*
      bugfix/*
    useWildcardPatterns: true
```

### Example 7: Patterns from a URL

```yaml
- name: Run branchMatchRegex action
  uses: michisec/branchMatchRegex@v3.0.0
  with:
    path: "https://example.com/patterns.yml"
```

## Development

```bash
# Run the tests
go test ./internal/...

# Build the binary
go build -o branchmatchregex .

# Build the container image
docker build -t branchmatchregex .
```

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE)
file for details.

<div style="text-align: center;">
  <sub>Made with ❤️ in Bavaria</sub>
</div>
