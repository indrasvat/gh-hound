package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// ListWorkflowRuns implements usecase.WorkflowRunHistory: run history
// scoped to one workflow via GET
// /repos/{o}/{r}/actions/workflows/{id}/runs. The workflow segment
// accepts a numeric ID or the workflow file name (live-verified:
// "ci.yml" and 290736476 both resolve on api.github.com).
func (c *Client) ListWorkflowRuns(ctx context.Context, repo, workflow string, filter usecase.RunFilter) ([]model.Run, error) {
	workflow = strings.TrimSpace(workflow)
	if workflow == "" {
		return nil, fmt.Errorf("workflow is required")
	}
	values := url.Values{}
	if filter.Branch != "" {
		values.Set("branch", filter.Branch)
	}
	if filter.Status != "" {
		values.Set("status", filter.Status)
	}
	if filter.PerPage > 0 {
		values.Set("per_page", strconv.Itoa(filter.PerPage))
	}
	if filter.Page > 0 {
		values.Set("page", strconv.Itoa(filter.Page))
	}
	var decoded runsResponse
	resource := resourcePath(repo, "actions/workflows/"+workflow+"/runs")
	if err := c.getJSON(ctx, resource, values, &decoded); err != nil {
		return nil, err
	}
	return mapRuns(decoded.WorkflowRuns)
}

type compareResponse struct {
	Status       string             `json:"status"`
	TotalCommits int                `json:"total_commits"`
	HTMLURL      string             `json:"html_url"`
	Commits      []compareCommitDTO `json:"commits"`
}

type compareCommitDTO struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commit"`
	Author *loginDTO `json:"author"`
}

// CompareCommits implements usecase.CommitComparer via GET
// /repos/{o}/{r}/compare/{base}...{head}. One call, per_page=100 on
// the commits list; TotalCommits reports the full range even when the
// page truncates it. Messages are reduced to the subject line — commit
// bodies never leak into suspect lists.
func (c *Client) CompareCommits(ctx context.Context, repo, base, head string) (model.CommitRange, error) {
	base = strings.TrimSpace(base)
	head = strings.TrimSpace(head)
	if base == "" || head == "" {
		return model.CommitRange{}, fmt.Errorf("compare needs both base and head SHAs")
	}
	values := url.Values{}
	values.Set("per_page", "100")
	var decoded compareResponse
	resource := resourcePath(repo, "compare/"+base+"..."+head)
	if err := c.getJSON(ctx, resource, values, &decoded); err != nil {
		return model.CommitRange{}, err
	}
	commits := make([]model.Commit, 0, len(decoded.Commits))
	for _, dto := range decoded.Commits {
		author := ""
		if dto.Author != nil {
			author = dto.Author.Login
		}
		if author == "" {
			author = dto.Commit.Author.Name
		}
		subject, _, _ := strings.Cut(dto.Commit.Message, "\n")
		commits = append(commits, model.Commit{
			SHA:     dto.SHA,
			Author:  author,
			Message: strings.TrimSpace(subject),
		})
	}
	return model.CommitRange{
		TotalCommits: decoded.TotalCommits,
		HTMLURL:      decoded.HTMLURL,
		Commits:      commits,
	}, nil
}
