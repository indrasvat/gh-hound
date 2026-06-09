package github

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func (c *Client) FetchJobLog(ctx context.Context, repo string, jobID int64) (string, error) {
	resource := resourcePath(repo, "actions/jobs/"+strconv.FormatInt(jobID, 10)+"/logs")
	endpoint := c.baseURL + resource
	body, status, err := c.fetchLogURL(ctx, endpoint, resource)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound || status == http.StatusGone {
		expiredStatus := status
		body, status, err = c.fetchLogURL(ctx, endpoint, resource)
		if err != nil {
			return "", err
		}
		if status >= 200 && status < 300 {
			c.storeLogRefetch(usecase.LogRefetchNotice{
				JobID:         jobID,
				Attempts:      2,
				ExpiredStatus: expiredStatus,
				Message:       "link had expired; re-requested job log",
			})
		}
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("github log download failed for job %d: status %d", jobID, status)
	}
	return string(body), nil
}

func (c *Client) storeLogRefetch(notice usecase.LogRefetchNotice) {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if c.logMeta == nil {
		c.logMeta = map[int64]usecase.LogRefetchNotice{}
	}
	c.logMeta[notice.JobID] = notice
}

func (c *Client) LastLogRefetch(jobID int64) (usecase.LogRefetchNotice, bool) {
	c.logMu.RLock()
	defer c.logMu.RUnlock()
	notice, ok := c.logMeta[jobID]
	return notice, ok
}

func (c *Client) fetchLogURL(ctx context.Context, rawURL, resource string) ([]byte, int, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	resp, err := noRedirectClient(c.http).Do(req)
	if err != nil {
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Duration: time.Since(start), Err: err.Error()})
		return nil, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), RateRemaining: resp.Header.Get("X-RateLimit-Remaining")})
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
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, resp.StatusCode, fmt.Errorf("github log url %s returned %d: %s", rawURL, resp.StatusCode, bytes.TrimSpace(limited))
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func (c *Client) fetchRedirectedLog(ctx context.Context, rawURL string) ([]byte, int, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: "github-actions-log-download", Duration: time.Since(start), Err: err.Error()})
		return nil, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: "github-actions-log-download", Status: resp.StatusCode, Duration: time.Since(start)})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			return nil, resp.StatusCode, nil
		}
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, resp.StatusCode, fmt.Errorf("github redirected log url %s returned %d: %s", rawURL, resp.StatusCode, bytes.TrimSpace(limited))
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
