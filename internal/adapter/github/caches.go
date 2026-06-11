package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// cacheDTO mirrors the live wire shape (pinned 2026-06-10, API
// v2026-03-10). The payload's version hash is dropped at the DTO
// boundary on purpose.
type cacheDTO struct {
	ID             int64     `json:"id"`
	Ref            string    `json:"ref"`
	Key            string    `json:"key"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	CreatedAt      time.Time `json:"created_at"`
	SizeInBytes    int64     `json:"size_in_bytes"`
}

type cachesResponse struct {
	TotalCount    int        `json:"total_count"`
	ActionsCaches []cacheDTO `json:"actions_caches"`
}

// ListCaches walks every page so cache pressure cannot hide past page
// one; server-side key/ref/sort params are forwarded untouched.
func (c *Client) ListCaches(ctx context.Context, repo string, filter usecase.CacheFilter) ([]model.Cache, error) {
	resource := resourcePath(repo, "actions/caches")
	var caches []model.Cache
	for page := 1; ; page++ {
		values := url.Values{
			"per_page": []string{"100"},
			"page":     []string{strconv.Itoa(page)},
		}
		if filter.Key != "" {
			values.Set("key", filter.Key)
		}
		if filter.Ref != "" {
			values.Set("ref", filter.Ref)
		}
		if filter.Sort != "" {
			values.Set("sort", filter.Sort)
		}
		if filter.Direction != "" {
			values.Set("direction", filter.Direction)
		}
		var decoded cachesResponse
		if err := c.getJSON(ctx, resource, values, &decoded); err != nil {
			return nil, err
		}
		for _, dto := range decoded.ActionsCaches {
			caches = append(caches, mapCache(dto))
		}
		if len(decoded.ActionsCaches) == 0 || len(caches) >= decoded.TotalCount {
			return caches, nil
		}
	}
}

func mapCache(dto cacheDTO) model.Cache {
	return model.Cache{
		ID:             dto.ID,
		Key:            dto.Key,
		Ref:            dto.Ref,
		SizeInBytes:    dto.SizeInBytes,
		LastAccessedAt: dto.LastAccessedAt,
		CreatedAt:      dto.CreatedAt,
	}
}

func (c *Client) CacheUsage(ctx context.Context, repo string) (model.CacheUsage, error) {
	var dto struct {
		ActiveCachesSizeInBytes int64 `json:"active_caches_size_in_bytes"`
		ActiveCachesCount       int   `json:"active_caches_count"`
	}
	if err := c.getJSON(ctx, resourcePath(repo, "actions/cache/usage"), nil, &dto); err != nil {
		return model.CacheUsage{}, err
	}
	return model.CacheUsage{
		ActiveSizeInBytes: dto.ActiveCachesSizeInBytes,
		ActiveCount:       dto.ActiveCachesCount,
	}, nil
}

// CacheStorageLimit implements usecase.CacheCapProvider via GET
// …/actions/cache/storage-limit (live-verified on github.com,
// 2026-06-11: this repo answers max_cache_size_gb 10). Hosts without
// the endpoint report unknown (0) so callers use the fallback cap
// instead of failing the whole kennel.
func (c *Client) CacheStorageLimit(ctx context.Context, repo string) (int64, error) {
	var dto struct {
		MaxCacheSizeGB int64 `json:"max_cache_size_gb"`
	}
	if err := c.getJSON(ctx, resourcePath(repo, "actions/cache/storage-limit"), nil, &dto); err != nil {
		var apiErr usecase.APIError
		if errors.As(err, &apiErr) && apiErr.Kind == usecase.APIErrorNotFound {
			return 0, nil
		}
		return 0, err
	}
	return dto.MaxCacheSizeGB << 30, nil
}

// DeleteCacheByID evicts one cache. The endpoint returns 204 with no
// body, so an accepted delete always counts exactly 1.
func (c *Client) DeleteCacheByID(ctx context.Context, repo string, id int64) (int, error) {
	resource := resourcePath(repo, "actions/caches/"+strconv.FormatInt(id, 10))
	if err := c.deleteJSON(ctx, resource, nil, nil); err != nil {
		return 0, err
	}
	return 1, nil
}

// DeleteCachesByKey evicts every cache matching the key (optionally
// scoped to one ref) and reports how many were dug up. The API answers
// with the deleted entries; zero matches is a typed not_found so
// agents and the TUI never mistake a no-op for a clean delete.
func (c *Client) DeleteCachesByKey(ctx context.Context, repo, key, ref string) (int, error) {
	values := url.Values{"key": []string{key}}
	if ref != "" {
		values.Set("ref", ref)
	}
	var decoded cachesResponse
	if err := c.deleteJSON(ctx, resourcePath(repo, "actions/caches"), values, &decoded); err != nil {
		return 0, err
	}
	if decoded.TotalCount <= 0 {
		return 0, usecase.ActionError{
			Kind:    usecase.ActionErrorNotFound,
			Message: "no caches matched key " + strconv.Quote(key),
		}
	}
	return decoded.TotalCount, nil
}

// deleteJSON sends a DELETE through the serial queue with the mutation
// error taxonomy: 404 becomes a typed not_found, everything else flows
// through mapActionHTTPError exactly like the rerun/cancel verbs.
func (c *Client) deleteJSON(ctx context.Context, resource string, query url.Values, out any) error {
	return c.queue.Do(ctx, func(ctx context.Context) error {
		start := time.Now()
		reqURL := c.baseURL + resource
		if len(query) > 0 {
			reqURL += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", APIVersion)
		resp, err := c.http.Do(req)
		if err != nil {
			c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Duration: time.Since(start), Err: err.Error()})
			return usecase.ActionError{Kind: usecase.ActionErrorNetwork, Message: err.Error()}
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), RateRemaining: resp.Header.Get("X-RateLimit-Remaining")})
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil {
				return nil
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return json.Unmarshal(body, out)
		}
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		payload = bytes.TrimSpace(payload)
		if resp.StatusCode == http.StatusNotFound {
			message := decodeErrorMessage(payload)
			if message == "" || message == "Not Found" {
				message = "no matching caches in this kennel"
			}
			return usecase.ActionError{Kind: usecase.ActionErrorNotFound, Status: resp.StatusCode, Message: message}
		}
		return mapActionHTTPError(resp.StatusCode, resp.Header, payload)
	})
}
