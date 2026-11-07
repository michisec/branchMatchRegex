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
>   reproducible pins.
> - **Behavior change:** unmatched branches no longer fail the workflow by
>   default. Set `failOnUnmatchedPattern: true` to keep the v2 behavior.
>   Optionally set `commentOnUnmatchedPattern: true` to post a configurable
>   comment on the pull request.

> **What's new in v2?**
>
> - All pattern sources (`patterns`, `useDefaultPatterns`, `path`) are now
>   combined (cumulative), not mutually exclusive.
> - Input names have changed: use `patterns` instead of `regex`, and
>   `useWildcardPatterns` instead of `useWildcard`.
> - You can now combine inline patterns, default patterns, and file/URL patterns
>   in any combination.

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
| `commentOnUnmatchedPattern` | Post or update a pull request comment if the branch does not match any pattern.                | No       | false                    |
| `comment`                   | Comment template. Supports `{branch}` and `{patterns}`. Empty uses the default text.           | No       | ""                       |
| `github-token`              | Token used to post the comment.                                                                | No       | `${{ github.token }}`    |
| `useWildcardPatterns`       | Treat patterns as wildcards (e.g. `feature/*`) instead of regex.                               | No       | false                    |
| `branch`                    | The branch to check.                                                                           | Yes      | `${{ github.head_ref }}` |

**Note:** All specified pattern sources (`patterns`, `useDefaultPatterns`,
`path`) are combined. The order is: `patterns`, then default patterns, then
file/URL. You can use any combination of sources at the same time.

### Unmatched branches

By default, a branch that does not match any pattern does **not** fail the
workflow. The action logs a warning and continues without posting a comment.
Commenting is opt-in: set `commentOnUnmatchedPattern: true` to post or update a
comment on the pull request.

`failOnUnmatchedPattern` and `commentOnUnmatchedPattern` are independent:

| `failOnUnmatchedPattern` | `commentOnUnmatchedPattern` | Result                                            |
| ------------------------ | --------------------------- | ------------------------------------------------- |
| `false` (default)        | `false` (default)           | Workflow succeeds with a warning.                 |
| `false`                  | `true`                      | Comment is posted or updated; workflow succeeds.  |
| `true`                   | `false`                     | Workflow fails.                                   |
| `true`                   | `true`                      | Comment is posted or updated; workflow fails.     |

To post comments, enable `commentOnUnmatchedPattern` and grant
`pull-requests: write`. The `github-token` input defaults to
`${{ github.token }}`, so no additional token configuration is required:

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
      commentOnUnmatchedPattern: true
```

Comments are only posted for pull request events. If no pull request or token is
available, the action logs a warning and continues; the comment failure does not
change whether the workflow fails.

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
