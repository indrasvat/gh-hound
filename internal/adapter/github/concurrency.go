package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type concurrencyGroupsResponse struct {
	TotalCount        int                   `json:"total_count"`
	ConcurrencyGroups []concurrencyGroupDTO `json:"concurrency_groups"`
}

type concurrencyGroupDTO struct {
	GroupName      string                 `json:"group_name"`
	GroupURL       string                 `json:"group_url"`
	LastAcquiredAt *time.Time             `json:"last_acquired_at"`
	TotalCount     int                    `json:"total_count"`
	GroupMembers   []concurrencyMemberDTO `json:"group_members"`
}

type concurrencyMemberDTO struct {
	RunID       int64        `json:"run_id"`
	RunName     string       `json:"run_name"`
	RunURL      string       `json:"run_url"`
	RunHTMLURL  string       `json:"run_html_url"`
	JobID       *int64       `json:"job_id"`
	JobName     string       `json:"job_name"`
	JobURL      string       `json:"job_url"`
	JobHTMLURL  string       `json:"job_html_url"`
	Status      model.Status `json:"status"`
	Position    *int         `json:"position"`
	PositionURL string       `json:"position_url"`
}

// ListConcurrencyGroups implements the repository-wide Actions
// concurrency group list. The endpoint uses cursor pagination rather
// than page numbers, so the adapter follows rel="next" links and
// forwards only caller-supplied cursors on the first request.
func (c *Client) ListConcurrencyGroups(ctx context.Context, repo string, opts usecase.ConcurrencyListOptions) ([]model.ConcurrencyGroupSummary, error) {
	values := concurrencyCursorValues(opts, true)
	resource := resourcePath(repo, "actions/concurrency_groups")
	var groups []model.ConcurrencyGroupSummary
	for {
		var decoded concurrencyGroupsResponse
		next, err := c.getJSONWithNextCursor(ctx, resource, values, &decoded)
		if err != nil {
			return nil, err
		}
		for _, dto := range decoded.ConcurrencyGroups {
			groups = append(groups, model.ConcurrencyGroupSummary{
				GroupName:      dto.GroupName,
				GroupURL:       dto.GroupURL,
				LastAcquiredAt: dto.LastAcquiredAt,
			})
		}
		if next == "" {
			return groups, nil
		}
		values = url.Values{"after": []string{next}}
		if opts.PerPage > 0 {
			values.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
}

// GetConcurrencyGroup reads the live queue for one group. A 404 stays
// a typed API not_found so the CLI/usecase layer can map named-group
// inspection to inactive without confusing missing runs/jobs.
func (c *Client) GetConcurrencyGroup(ctx context.Context, repo, groupName string, filter usecase.ConcurrencyAheadFilter) (model.ConcurrencyGroup, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return model.ConcurrencyGroup{}, fmt.Errorf("concurrency group name is required")
	}
	values := url.Values{}
	switch {
	case filter.AheadOfRun > 0 && filter.AheadOfJob > 0:
		return model.ConcurrencyGroup{}, fmt.Errorf("ahead_of_run and ahead_of_job are mutually exclusive")
	case filter.AheadOfRun > 0:
		values.Set("ahead_of_run", strconv.FormatInt(filter.AheadOfRun, 10))
	case filter.AheadOfJob > 0:
		values.Set("ahead_of_job", strconv.FormatInt(filter.AheadOfJob, 10))
	}
	resource := resourcePath(repo, "actions/concurrency_groups/"+url.PathEscape(groupName))
	var decoded concurrencyGroupDTO
	if err := c.getJSON(ctx, resource, values, &decoded); err != nil {
		return model.ConcurrencyGroup{}, err
	}
	return mapConcurrencyGroup(decoded, nil), nil
}

// ListRunConcurrencyGroups reads the configured groups associated with
// one workflow run. Its total_count is the number of configured groups,
// not active members, so that count is carried separately on every
// returned group.
func (c *Client) ListRunConcurrencyGroups(ctx context.Context, repo string, runID int64, opts usecase.ConcurrencyListOptions) ([]model.ConcurrencyGroup, error) {
	if runID <= 0 {
		return nil, fmt.Errorf("run id must be positive")
	}
	values := concurrencyCursorValues(opts, true)
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/concurrency_groups")
	var groups []model.ConcurrencyGroup
	configuredCount := 0
	for {
		var decoded concurrencyGroupsResponse
		next, err := c.getJSONWithNextCursor(ctx, resource, values, &decoded)
		if err != nil {
			return nil, err
		}
		configuredCount = decoded.TotalCount
		for _, dto := range decoded.ConcurrencyGroups {
			groups = append(groups, mapConcurrencyGroup(dto, &configuredCount))
		}
		if next == "" {
			return groups, nil
		}
		values = url.Values{"after": []string{next}}
		if opts.PerPage > 0 {
			values.Set("per_page", strconv.Itoa(opts.PerPage))
		}
	}
}

func concurrencyCursorValues(opts usecase.ConcurrencyListOptions, includeBefore bool) url.Values {
	values := url.Values{}
	if opts.PerPage > 0 {
		values.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	if includeBefore && opts.Before != "" {
		values.Set("before", opts.Before)
	}
	if opts.After != "" {
		values.Set("after", opts.After)
	}
	return values
}

func mapConcurrencyGroup(dto concurrencyGroupDTO, configuredCount *int) model.ConcurrencyGroup {
	members := make([]model.ConcurrencyMember, 0, len(dto.GroupMembers))
	for _, member := range dto.GroupMembers {
		members = append(members, model.ConcurrencyMember{
			RunID:       member.RunID,
			RunName:     member.RunName,
			RunURL:      member.RunURL,
			RunHTMLURL:  member.RunHTMLURL,
			JobID:       cloneInt64(member.JobID),
			JobName:     member.JobName,
			JobURL:      member.JobURL,
			JobHTMLURL:  member.JobHTMLURL,
			Status:      member.Status,
			Position:    cloneInt(member.Position),
			PositionURL: member.PositionURL,
		})
	}
	group := model.ConcurrencyGroup{
		GroupName:            dto.GroupName,
		GroupURL:             dto.GroupURL,
		LastAcquiredAt:       dto.LastAcquiredAt,
		ConfiguredGroupCount: cloneInt(configuredCount),
		ActiveMemberCount:    len(members),
		Members:              members,
	}
	if configuredCount == nil && dto.TotalCount > 0 {
		group.ActiveMemberCount = dto.TotalCount
	}
	return group
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (c *Client) getJSONWithNextCursor(ctx context.Context, resource string, query url.Values, out any) (string, error) {
	var next string
	err := c.getJSONWithResponse(ctx, resource, query, out, func(resp *http.Response) {
		next = nextCursor(resp.Header.Get("Link"))
	})
	return next, err
}

func nextCursor(linkHeader string) string {
	for part := range strings.SplitSeq(linkHeader, ",") {
		link, rel, ok := parseLinkPart(part)
		if !ok || rel != "next" {
			continue
		}
		parsed, err := url.Parse(link)
		if err != nil {
			return ""
		}
		return parsed.Query().Get("after")
	}
	return ""
}

func parseLinkPart(part string) (link, rel string, ok bool) {
	part = strings.TrimSpace(part)
	if !strings.HasPrefix(part, "<") {
		return "", "", false
	}
	end := strings.Index(part, ">")
	if end < 0 {
		return "", "", false
	}
	link = part[1:end]
	for param := range strings.SplitSeq(part[end+1:], ";") {
		param = strings.TrimSpace(param)
		value, found := strings.CutPrefix(param, `rel=`)
		if found {
			rel = strings.Trim(value, `"`)
		}
	}
	return link, rel, link != "" && rel != ""
}
