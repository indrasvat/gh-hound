package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

const APIVersion = "2026-03-10"

type Client struct {
	baseURL string
	http    *http.Client
	cache   *Cache
	queue   *Queue
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    httpClient,
		cache:   NewCache(),
		queue:   NewQueue(),
	}
}

func (c *Client) ListRuns(ctx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	values := url.Values{}
	if filter.Branch != "" {
		values.Set("branch", filter.Branch)
	}
	if filter.Status != "" {
		values.Set("status", filter.Status)
	}
	if filter.Event != "" {
		values.Set("event", filter.Event)
	}
	if filter.Actor != "" {
		values.Set("actor", filter.Actor)
	}
	if filter.HeadSHA != "" {
		values.Set("head_sha", filter.HeadSHA)
	}
	if filter.PerPage > 0 {
		values.Set("per_page", strconv.Itoa(filter.PerPage))
	}
	if filter.Page > 0 {
		values.Set("page", strconv.Itoa(filter.Page))
	}

	var decoded runsResponse
	if err := c.getJSON(ctx, resourcePath(filter.Repo, "actions/runs"), values, &decoded); err != nil {
		return nil, err
	}
	return mapRuns(decoded.WorkflowRuns)
}

func (c *Client) GetRun(ctx context.Context, repo string, runID int64) (model.Run, error) {
	var dto runDTO
	if err := c.getJSON(ctx, resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)), nil, &dto); err != nil {
		return model.Run{}, err
	}
	run, err := mapRun(dto)
	if err != nil {
		return model.Run{}, err
	}
	return run, nil
}

func (c *Client) ListJobs(ctx context.Context, repo string, runID int64) ([]model.Job, error) {
	values := url.Values{"filter": []string{"latest"}}
	var decoded jobsResponse
	if err := c.getJSON(ctx, resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/jobs"), values, &decoded); err != nil {
		return nil, err
	}
	return mapJobs(decoded.Jobs)
}

func (c *Client) GetJob(ctx context.Context, repo string, jobID int64) (model.Job, error) {
	var dto jobDTO
	if err := c.getJSON(ctx, resourcePath(repo, "actions/jobs/"+strconv.FormatInt(jobID, 10)), nil, &dto); err != nil {
		return model.Job{}, err
	}
	job, err := mapJob(dto)
	if err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func (c *Client) ListWorkflows(ctx context.Context, repo string) ([]model.Workflow, error) {
	var decoded workflowsResponse
	if err := c.getJSON(ctx, resourcePath(repo, "actions/workflows"), nil, &decoded); err != nil {
		return nil, err
	}
	workflows := make([]model.Workflow, 0, len(decoded.Workflows))
	for _, workflow := range decoded.Workflows {
		workflows = append(workflows, model.Workflow{
			ID:      workflow.ID,
			Name:    workflow.Name,
			Path:    workflow.Path,
			State:   workflow.State,
			HTMLURL: workflow.HTMLURL,
		})
	}
	return workflows, nil
}

func (c *Client) ListAnnotations(ctx context.Context, repo string, job model.Job) ([]model.Annotation, error) {
	checkID := strconv.FormatInt(job.ID, 10)
	if job.CheckRunURL != "" {
		parts := strings.Split(strings.TrimRight(job.CheckRunURL, "/"), "/")
		checkID = parts[len(parts)-1]
	}
	var decoded []annotationDTO
	if err := c.getJSON(ctx, resourcePath(repo, "check-runs/"+checkID+"/annotations"), nil, &decoded); err != nil {
		return nil, err
	}
	annotations := make([]model.Annotation, 0, len(decoded))
	for _, annotation := range decoded {
		annotations = append(annotations, model.Annotation{
			Path:      annotation.Path,
			StartLine: annotation.StartLine,
			EndLine:   annotation.EndLine,
			Level:     annotation.Level,
			Message:   annotation.Message,
			Title:     annotation.Title,
		})
	}
	return annotations, nil
}

func (c *Client) getJSON(ctx context.Context, resource string, query url.Values, out any) error {
	var body []byte
	err := c.queue.Do(ctx, func(ctx context.Context) error {
		reqURL := c.baseURL + resource
		if len(query) > 0 {
			reqURL += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", APIVersion)
		if etag, ok := c.cache.ETag(reqURL); ok {
			req.Header.Set("If-None-Match", etag)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode == http.StatusNotModified {
			cached, ok := c.cache.Body(reqURL)
			if !ok {
				return fmt.Errorf("not modified without cached body for %s", reqURL)
			}
			body = cached
			return nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return fmt.Errorf("github api %s %s: %s", req.Method, resource, bytes.TrimSpace(limited))
		}
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if etag := resp.Header.Get("ETag"); etag != "" {
			c.cache.Store(reqURL, etag, body)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode github response %s: %w", resource, err)
	}
	return nil
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	etag string
	body []byte
}

func NewCache() *Cache {
	return &Cache{entries: map[string]cacheEntry{}}
}

func (c *Cache) ETag(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	return entry.etag, ok
}

func (c *Cache) Body(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), entry.body...), true
}

func (c *Cache) Store(key, etag string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{etag: etag, body: append([]byte(nil), body...)}
}

type Queue struct {
	mu sync.Mutex
}

func NewQueue() *Queue {
	return &Queue{}
}

func (q *Queue) Do(ctx context.Context, fn func(context.Context) error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx)
}

type Poller struct {
	Fast time.Duration
	Slow time.Duration
}

func (p Poller) Next(runs []model.Run, previous time.Duration) time.Duration {
	for _, run := range runs {
		if run.Status == model.StatusInProgress || run.Status == model.StatusQueued || run.Status == model.StatusPending || run.Status == model.StatusRequested || run.Status == model.StatusWaiting {
			return p.Fast
		}
	}
	if previous <= 0 {
		return p.Slow
	}
	next := previous * 2
	if next > p.Slow {
		return p.Slow
	}
	return next
}

func resourcePath(repo, suffix string) string {
	return path.Join("/repos", repo, suffix)
}
