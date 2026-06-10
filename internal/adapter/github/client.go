package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	trace   traceOptions
	metaMu  sync.RWMutex
	meta    map[string]usecase.RequestMeta
	logMu   sync.RWMutex
	logMeta map[int64]usecase.LogRefetchNotice
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	return NewClientWithOptions(baseURL, httpClient, ClientOptions{})
}

type ClientOptions struct {
	TraceHTTP bool
	Logger    *slog.Logger
}

type traceOptions struct {
	enabled bool
	logger  *slog.Logger
}

func NewClientWithOptions(baseURL string, httpClient *http.Client, options ClientOptions) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    httpClient,
		cache:   NewCache(),
		queue:   NewQueue(),
		trace:   traceOptions{enabled: options.TraceHTTP, logger: logger},
		meta:    map[string]usecase.RequestMeta{},
		logMeta: map[int64]usecase.LogRefetchNotice{},
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
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/jobs")
	return c.listJobsPaginated(ctx, resource, url.Values{"filter": []string{"latest"}})
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

func (c *Client) FetchWorkflowFile(ctx context.Context, repo, workflowPath string) (string, error) {
	workflowPath = strings.TrimSpace(workflowPath)
	if workflowPath == "" {
		return "", fmt.Errorf("workflow path is required")
	}
	body, err := c.getRaw(ctx, resourcePath(repo, "contents/"+workflowPath), nil)
	if err != nil {
		return "", err
	}
	return string(body), nil
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

func (c *Client) getRaw(ctx context.Context, resource string, query url.Values) ([]byte, error) {
	var body []byte
	err := c.queue.Do(ctx, func(ctx context.Context) error {
		start := time.Now()
		reqURL := c.baseURL + resource
		if len(query) > 0 {
			reqURL += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github.raw")
		req.Header.Set("X-GitHub-Api-Version", APIVersion)
		resp, err := c.http.Do(req)
		if err != nil {
			c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Duration: time.Since(start), Err: err.Error()})
			return usecase.APIError{Kind: usecase.APIErrorNetwork, Method: req.Method, Resource: resource, Message: err.Error()}
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), RateRemaining: resp.Header.Get("X-RateLimit-Remaining")})
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return mapReadHTTPError(req.Method, resource, resp.StatusCode, resp.Header, bytes.TrimSpace(limited))
		}
		body, err = io.ReadAll(resp.Body)
		return err
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) getJSON(ctx context.Context, resource string, query url.Values, out any) error {
	var body []byte
	err := c.queue.Do(ctx, func(ctx context.Context) error {
		start := time.Now()
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
		ifNoneMatch := ""
		if etag, ok := c.cache.ETag(reqURL); ok {
			req.Header.Set("If-None-Match", etag)
			ifNoneMatch = etag
		}
		resp, err := c.http.Do(req)
		if err != nil {
			c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Duration: time.Since(start), IfNoneMatch: ifNoneMatch, Err: err.Error()})
			return usecase.APIError{Kind: usecase.APIErrorNetwork, Method: req.Method, Resource: resource, Message: err.Error()}
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode == http.StatusNotModified {
			c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), IfNoneMatch: ifNoneMatch, RateRemaining: resp.Header.Get("X-RateLimit-Remaining"), Cache: "hit"})
			cached, ok := c.cache.Body(reqURL)
			if !ok {
				return fmt.Errorf("not modified without cached body for %s", reqURL)
			}
			body = cached
			return nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), IfNoneMatch: ifNoneMatch, RateRemaining: resp.Header.Get("X-RateLimit-Remaining"), Cache: "miss"})
			limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return mapReadHTTPError(req.Method, resource, resp.StatusCode, resp.Header, bytes.TrimSpace(limited))
		}
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		etag := resp.Header.Get("ETag")
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), IfNoneMatch: ifNoneMatch, ETag: etag, RateRemaining: resp.Header.Get("X-RateLimit-Remaining"), Cache: "miss"})
		if etag != "" {
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

type traceRecord struct {
	Method        string
	Resource      string
	Status        int
	Duration      time.Duration
	ETag          string
	IfNoneMatch   string
	RateRemaining string
	Cache         string
	Err           string
}

func (c *Client) traceHTTP(ctx context.Context, record traceRecord) {
	if record.Status != 0 && record.Resource != "" {
		c.storeRequestMeta(record)
	}
	if !c.trace.enabled || c.trace.logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("method", record.Method),
		slog.String("resource", record.Resource),
		slog.Int64("duration_ms", record.Duration.Milliseconds()),
	}
	if record.Status != 0 {
		attrs = append(attrs, slog.Int("status", record.Status))
	}
	if record.ETag != "" {
		attrs = append(attrs, slog.String("etag", record.ETag))
	}
	if record.IfNoneMatch != "" {
		attrs = append(attrs, slog.String("if_none_match", record.IfNoneMatch))
	}
	if record.RateRemaining != "" {
		attrs = append(attrs, slog.String("rate_remaining", record.RateRemaining))
	}
	if record.Cache != "" {
		attrs = append(attrs, slog.String("cache", record.Cache))
	}
	if record.Err != "" {
		attrs = append(attrs, slog.String("error", record.Err))
	}
	c.trace.logger.LogAttrs(ctx, slog.LevelDebug, "github_http", attrs...)
}

func (c *Client) storeRequestMeta(record traceRecord) {
	c.metaMu.Lock()
	defer c.metaMu.Unlock()
	if c.meta == nil {
		c.meta = map[string]usecase.RequestMeta{}
	}
	c.meta[record.Resource] = usecase.RequestMeta{
		Resource:      record.Resource,
		Status:        record.Status,
		Cache:         record.Cache,
		RateRemaining: record.RateRemaining,
	}
}

func (c *Client) LastRequestMeta(resource string) (usecase.RequestMeta, bool) {
	c.metaMu.RLock()
	defer c.metaMu.RUnlock()
	meta, ok := c.meta[resource]
	return meta, ok
}

func mapReadHTTPError(method, resource string, status int, header http.Header, payload []byte) error {
	kind := usecase.APIErrorUnknown
	switch status {
	case http.StatusUnauthorized:
		kind = usecase.APIErrorAuth
	case http.StatusForbidden:
		if header.Get("X-RateLimit-Remaining") == "0" {
			kind = usecase.APIErrorRateLimit
		} else {
			kind = usecase.APIErrorPermission
		}
	case http.StatusNotFound, http.StatusGone:
		kind = usecase.APIErrorNotFound
	case http.StatusTooManyRequests:
		kind = usecase.APIErrorRateLimit
	}
	message := decodeErrorMessage(payload)
	if message == "" {
		message = fmt.Sprintf("github api returned status %d", status)
	}
	retryAfter, resetAt := parseRateLimitHeaders(header)
	return usecase.APIError{
		Kind:       kind,
		Method:     method,
		Resource:   resource,
		Status:     status,
		Message:    message,
		RetryAfter: retryAfter,
		ResetAt:    resetAt,
	}
}

func parseRateLimitHeaders(header http.Header) (time.Duration, time.Time) {
	var retryAfter time.Duration
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
			retryAfter = time.Duration(seconds) * time.Second
		}
	}
	var resetAt time.Time
	if raw := strings.TrimSpace(header.Get("X-RateLimit-Reset")); raw != "" {
		if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds > 0 {
			resetAt = time.Unix(seconds, 0).UTC()
		}
	}
	return retryAfter, resetAt
}

func decodeErrorMessage(payload []byte) string {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return ""
	}
	var decoded struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &decoded); err == nil && strings.TrimSpace(decoded.Message) != "" {
		return strings.TrimSpace(decoded.Message)
	}
	return trimmed
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
