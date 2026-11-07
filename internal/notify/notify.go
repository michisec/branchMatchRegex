package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	marker          = "<!-- branchMatchRegex:unmatched -->"
	defaultText     = "Branch `{branch}` does not match any of the configured patterns."
	commentsPerPage = 100
	maxCommentPages = 10
)

var pullRequestRef = regexp.MustCompile(`^refs/pull/(\d+)/(?:merge|head)$`)

type comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

type event struct {
	PullRequest *struct {
		Number int `json:"number"`
	} `json:"pull_request"`
	Issue *struct {
		Number      int             `json:"number"`
		PullRequest json.RawMessage `json:"pull_request"`
	} `json:"issue"`
}

// Client posts comments through the GitHub REST API.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
	Repository string
}

// PostUnmatched posts or updates the unmatched-branch comment on the pull
// request associated with the current workflow run.
func PostUnmatched(token, branch string, patterns []string, template string) error {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return errors.New("missing github token")
	}

	repository := os.Getenv("GITHUB_REPOSITORY")
	if repository == "" {
		return errors.New("missing GITHUB_REPOSITORY")
	}

	number, ok := pullRequestNumber(os.Getenv("GITHUB_EVENT_PATH"), os.Getenv("GITHUB_REF"))
	if !ok {
		return errors.New("no pull request found for this run")
	}

	client := &Client{
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		BaseURL:    apiBaseURL(),
		Token:      token,
		Repository: repository,
	}

	return client.UpsertPullRequestComment(context.Background(), number, render(branch, patterns, template))
}

// UpsertPullRequestComment creates a comment or updates an existing comment
// carrying the action marker.
func (c *Client) UpsertPullRequestComment(ctx context.Context, number int, body string) error {
	body = marker + "\n" + body

	id, found, err := c.findComment(ctx, number)
	if err != nil {
		return err
	}
	if found {
		return c.updateComment(ctx, id, body)
	}
	return c.createComment(ctx, number, body)
}

func (c *Client) findComment(ctx context.Context, number int) (int64, bool, error) {
	for page := 1; page <= maxCommentPages; page++ {
		path := fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=%d&page=%d", c.Repository, number, commentsPerPage, page)

		var comments []comment
		if err := c.do(ctx, http.MethodGet, path, nil, &comments); err != nil {
			return 0, false, err
		}

		for _, item := range comments {
			if strings.Contains(item.Body, marker) {
				return item.ID, true, nil
			}
		}

		if len(comments) < commentsPerPage {
			break
		}
	}

	return 0, false, nil
}

func (c *Client) createComment(ctx context.Context, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", c.Repository, number)
	return c.do(ctx, http.MethodPost, path, map[string]string{"body": body}, nil)
}

func (c *Client) updateComment(ctx context.Context, id int64, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/comments/%d", c.Repository, id)
	return c.do(ctx, http.MethodPatch, path, map[string]string{"body": body}, nil)
}

func (c *Client) do(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request github api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github api %s %s returned %s: %s", method, path, resp.Status, strings.TrimSpace(string(message)))
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func render(branch string, patterns []string, template string) string {
	if template == "" {
		template = defaultText
	}

	quoted := make([]string, len(patterns))
	for i, pattern := range patterns {
		quoted[i] = "`" + pattern + "`"
	}

	list := strings.Join(quoted, ", ")
	if list == "" {
		list = "_(none)_"
	}

	return strings.NewReplacer("{branch}", branch, "{patterns}", list).Replace(template)
}

func pullRequestNumber(eventPath, ref string) (int, bool) {
	if eventPath != "" {
		if data, err := os.ReadFile(eventPath); err == nil {
			var e event
			if json.Unmarshal(data, &e) == nil {
				if e.PullRequest != nil && e.PullRequest.Number > 0 {
					return e.PullRequest.Number, true
				}
				if e.Issue != nil && e.Issue.Number > 0 && isPullRequest(e.Issue.PullRequest) {
					return e.Issue.Number, true
				}
			}
		}
	}

	if match := pullRequestRef.FindStringSubmatch(ref); match != nil {
		if number, err := strconv.Atoi(match[1]); err == nil && number > 0 {
			return number, true
		}
	}

	return 0, false
}

func isPullRequest(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func apiBaseURL() string {
	if base := os.Getenv("GITHUB_API_URL"); base != "" {
		return strings.TrimRight(base, "/")
	}
	return "https://api.github.com"
}
