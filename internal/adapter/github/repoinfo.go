package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// DefaultBranch implements usecase.RepoInfoProvider.
func (c *Client) DefaultBranch(ctx context.Context, repo string) (string, error) {
	var payload struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.getJSON(ctx, resourcePath(repo, ""), nil, &payload); err != nil {
		return "", err
	}
	if payload.DefaultBranch == "" {
		return "", fmt.Errorf("github repo lookup for %s returned no default branch", repo)
	}
	return payload.DefaultBranch, nil
}

// RefExists implements usecase.RefValidator: branches first, then
// tags. Branch names may contain slashes and must be path-escaped.
func (c *Client) RefExists(ctx context.Context, repo, ref string) (bool, error) {
	exists, err := c.refProbe(ctx, resourcePath(repo, "branches/"+url.PathEscape(ref)))
	if err != nil || exists {
		return exists, err
	}
	return c.refProbe(ctx, resourcePath(repo, "git/ref/tags/"+url.PathEscape(ref)))
}

func (c *Client) refProbe(ctx context.Context, resource string) (bool, error) {
	var payload struct {
		Name string `json:"name"`
		Ref  string `json:"ref"`
	}
	err := c.getJSON(ctx, resource, nil, &payload)
	if err == nil {
		return true, nil
	}
	var apiErr usecase.APIError
	if errors.As(err, &apiErr) && apiErr.Kind == usecase.APIErrorNotFound {
		return false, nil
	}
	return false, err
}
