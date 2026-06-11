package usecase

import (
	"context"
	"slices"
	"strings"

	"github.com/indrasvat/gh-hound/internal/model"
)

// CacheCapFallbackBytes is the documented default per-repo Actions
// cache cap (10 GB) at which GitHub starts LRU-evicting. The live cap
// comes from the storage-limit endpoint via CacheCapProvider
// (live-verified on github.com 2026-06-11); this fallback covers
// adapters and hosts without it.
const CacheCapFallbackBytes = int64(10) << 30

// cacheWarnPressure is the gauge threshold past which the kennel
// warns about imminent eviction.
const cacheWarnPressure = 0.9

type CacheSort string

const (
	CacheSortSize     CacheSort = "size"
	CacheSortLastUsed CacheSort = "last_used"
)

// CachesService lists the kennel, reads usage-vs-cap, and digs up
// stale entries. Deletes are paced through the shared MutationLimiter
// exactly like rerun/cancel.
type CachesService struct {
	GitHub  GitHubCaches
	Limiter *MutationLimiter
}

func (s CachesService) List(ctx context.Context, repo string, filter CacheFilter) ([]model.Cache, error) {
	return s.GitHub.ListCaches(ctx, repo, filter)
}

func (s CachesService) Usage(ctx context.Context, repo string) (model.CacheUsage, error) {
	return s.GitHub.CacheUsage(ctx, repo)
}

// Cap resolves the effective eviction cap: the repo's configured
// storage limit when the adapter exposes it, else the documented 10 GB
// fallback. A lookup failure falls back too — the gauge must render
// even when the cap endpoint is missing (older GHES) or denied.
func (s CachesService) Cap(ctx context.Context, repo string) int64 {
	provider, ok := s.GitHub.(CacheCapProvider)
	if !ok {
		return CacheCapFallbackBytes
	}
	limit, err := provider.CacheStorageLimit(ctx, repo)
	if err != nil || limit <= 0 {
		return CacheCapFallbackBytes
	}
	return limit
}

func (s CachesService) DeleteByID(ctx context.Context, repo string, id int64) (int, error) {
	if id <= 0 {
		return 0, ActionError{Kind: ActionErrorValidation, Field: "id", Message: "a positive cache ID is required"}
	}
	if err := s.pace(ctx); err != nil {
		return 0, err
	}
	return s.GitHub.DeleteCacheByID(ctx, repo, id)
}

func (s CachesService) DeleteByKey(ctx context.Context, repo, key, ref string) (int, error) {
	if strings.TrimSpace(key) == "" {
		return 0, ActionError{Kind: ActionErrorValidation, Field: "key", Message: "a cache key is required"}
	}
	if err := s.pace(ctx); err != nil {
		return 0, err
	}
	return s.GitHub.DeleteCachesByKey(ctx, repo, key, ref)
}

func (s CachesService) pace(ctx context.Context) error {
	if s.Limiter == nil {
		return ctx.Err()
	}
	return s.Limiter.Wait(ctx)
}

// CachePressure reports usage against the effective cap as a ratio.
// It deliberately does not clamp at 1.0: eviction is asynchronous and
// live repos report usage above the cap.
func CachePressure(usage model.CacheUsage, cap int64) float64 {
	if cap <= 0 {
		cap = CacheCapFallbackBytes
	}
	return float64(usage.ActiveSizeInBytes) / float64(cap)
}

// CacheNearCap reports whether the kennel is past the 90% eviction
// warning threshold.
func CacheNearCap(usage model.CacheUsage, cap int64) bool {
	return CachePressure(usage, cap) > cacheWarnPressure
}

// FilterCachesByKey is the TUI's client-side `/` filter: a
// case-insensitive key substring match. A blank query keeps the whole
// kennel.
func FilterCachesByKey(caches []model.Cache, query string) []model.Cache {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return caches
	}
	out := make([]model.Cache, 0, len(caches))
	for _, cache := range caches {
		if strings.Contains(strings.ToLower(cache.Key), query) {
			out = append(out, cache)
		}
	}
	return out
}

// SortCaches returns a sorted copy: size puts the biggest first
// (where the bytes go), last_used puts the stalest first (what the
// LRU evictor will take next — and what a human prunes first).
func SortCaches(caches []model.Cache, by CacheSort) []model.Cache {
	out := slices.Clone(caches)
	switch by {
	case CacheSortLastUsed:
		slices.SortStableFunc(out, func(a, b model.Cache) int {
			return a.LastAccessedAt.Compare(b.LastAccessedAt)
		})
	default:
		slices.SortStableFunc(out, func(a, b model.Cache) int {
			switch {
			case a.SizeInBytes > b.SizeInBytes:
				return -1
			case a.SizeInBytes < b.SizeInBytes:
				return 1
			default:
				return 0
			}
		})
	}
	return out
}
