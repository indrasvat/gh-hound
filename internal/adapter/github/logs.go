package github

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) FetchJobLog(ctx context.Context, repo string, jobID int64) (string, error) {
	resource := resourcePath(repo, "actions/jobs/"+strconv.FormatInt(jobID, 10)+"/logs")
	endpoint := c.baseURL + resource
	body, status, err := c.fetchLogURL(ctx, endpoint)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		body, _, err = c.fetchLogURL(ctx, endpoint)
		if err != nil {
			return "", err
		}
	}
	return string(body), nil
}

func (c *Client) fetchLogURL(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	resp, err := noRedirectClient(c.http).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect {
		location, err := resp.Location()
		if err != nil {
			return nil, resp.StatusCode, err
		}
		if !location.IsAbs() {
			base, err := url.Parse(rawURL)
			if err != nil {
				return nil, resp.StatusCode, err
			}
			location = base.ResolveReference(location)
		}
		return c.fetchRedirectedLog(ctx, location.String())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func (c *Client) fetchRedirectedLog(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func noRedirectClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	copied := *client
	copied.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copied
}
