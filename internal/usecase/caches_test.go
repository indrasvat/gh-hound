package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

type stubCaches struct {
	caches     []model.Cache
	usage      model.CacheUsage
	deleteID   int64
	deleteKey  string
	deleteRef  string
	deleteByID int
	deleteByKy int
	err        error
}

func (s *stubCaches) ListCaches(context.Context, string, CacheFilter) ([]model.Cache, error) {
	return s.caches, s.err
}

func (s *stubCaches) CacheUsage(context.Context, string) (model.CacheUsage, error) {
	return s.usage, s.err
}

func (s *stubCaches) DeleteCacheByID(_ context.Context, _ string, id int64) (int, error) {
	s.deleteByID++
	s.deleteID = id
	return 1, s.err
}

func (s *stubCaches) DeleteCachesByKey(_ context.Context, _ string, key, ref string) (int, error) {
	s.deleteByKy++
	s.deleteKey, s.deleteRef = key, ref
	return 3, s.err
}

func TestCachePressureUsesFallbackCapAndHandlesOverflow(t *testing.T) {
	usage := model.CacheUsage{ActiveSizeInBytes: CacheCapFallbackBytes / 2}
	if got := CachePressure(usage, 0); got != 0.5 {
		t.Fatalf("pressure = %v, want 0.5 via fallback cap", got)
	}
	// Eviction is asynchronous: live repos report usage above the cap
	// (openclaw was at 11.8 GB on 2026-06-10). Pressure must pass 1.0
	// rather than clamp and lie.
	over := model.CacheUsage{ActiveSizeInBytes: CacheCapFallbackBytes + CacheCapFallbackBytes/10}
	if got := CachePressure(over, 0); got <= 1.0 {
		t.Fatalf("over-cap pressure = %v, want > 1.0", got)
	}
}

func TestCacheNearCapWarnsPastNinetyPercent(t *testing.T) {
	cap := int64(1000)
	if CacheNearCap(model.CacheUsage{ActiveSizeInBytes: 900}, cap) {
		t.Fatal("exactly 90% is not yet near cap")
	}
	if !CacheNearCap(model.CacheUsage{ActiveSizeInBytes: 901}, cap) {
		t.Fatal("past 90% must warn")
	}
	if CacheNearCap(model.CacheUsage{}, 0) {
		t.Fatal("an empty kennel never warns")
	}
}

func TestFilterCachesByKeyIsCaseInsensitiveSubstring(t *testing.T) {
	caches := []model.Cache{
		{ID: 1, Key: "setup-go-Linux"},
		{ID: 2, Key: "go-mod-darwin"},
		{ID: 3, Key: "node-modules"},
	}
	got := FilterCachesByKey(caches, "GO-MOD")
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("filter = %#v, want only go-mod-darwin", got)
	}
	if got := FilterCachesByKey(caches, "  "); len(got) != 3 {
		t.Fatalf("blank query keeps everything, got %d", len(got))
	}
}

func TestSortCachesBySizeAndStaleness(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	caches := []model.Cache{
		{ID: 1, SizeInBytes: 10, LastAccessedAt: now.Add(-time.Hour)},
		{ID: 2, SizeInBytes: 30, LastAccessedAt: now.Add(-72 * time.Hour)},
		{ID: 3, SizeInBytes: 20, LastAccessedAt: now.Add(-time.Minute)},
	}
	bySize := SortCaches(caches, CacheSortSize)
	if bySize[0].ID != 2 || bySize[1].ID != 3 || bySize[2].ID != 1 {
		t.Fatalf("size sort wrong: %#v", bySize)
	}
	// Stalest first: the eviction workflow hunts least-recently-used.
	byUse := SortCaches(caches, CacheSortLastUsed)
	if byUse[0].ID != 2 || byUse[1].ID != 1 || byUse[2].ID != 3 {
		t.Fatalf("last-used sort wrong: %#v", byUse)
	}
	if caches[0].ID != 1 {
		t.Fatal("SortCaches must not mutate its input")
	}
}

func TestCachesServiceDeleteValidations(t *testing.T) {
	stub := &stubCaches{}
	service := CachesService{GitHub: stub}

	_, err := service.DeleteByID(context.Background(), "x/y", 0)
	if actionErr, ok := AsActionError(err); !ok || actionErr.Kind != ActionErrorValidation || actionErr.Field != "id" {
		t.Fatalf("zero id must refuse as validation/id, got %v", err)
	}
	_, err = service.DeleteByKey(context.Background(), "x/y", "  ", "")
	if actionErr, ok := AsActionError(err); !ok || actionErr.Kind != ActionErrorValidation || actionErr.Field != "key" {
		t.Fatalf("blank key must refuse as validation/key, got %v", err)
	}
	if stub.deleteByID != 0 || stub.deleteByKy != 0 {
		t.Fatal("validation refusals must not reach the adapter")
	}

	count, err := service.DeleteByID(context.Background(), "x/y", 42)
	if err != nil || count != 1 || stub.deleteID != 42 {
		t.Fatalf("delete by id passthrough wrong: %d %v %d", count, err, stub.deleteID)
	}
	count, err = service.DeleteByKey(context.Background(), "x/y", "go-mod", "refs/heads/main")
	if err != nil || count != 3 || stub.deleteKey != "go-mod" || stub.deleteRef != "refs/heads/main" {
		t.Fatalf("delete by key passthrough wrong: %d %v %#v", count, err, stub)
	}
}

func TestCachesServiceDeletesArePaced(t *testing.T) {
	clock := &fakeCacheClock{now: time.Unix(1000, 0)}
	service := CachesService{
		GitHub:  &stubCaches{},
		Limiter: &MutationLimiter{MinSpacing: time.Second, Clock: clock},
	}
	if _, err := service.DeleteByID(context.Background(), "x/y", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteByKey(context.Background(), "x/y", "k", ""); err != nil {
		t.Fatal(err)
	}
	if clock.slept == 0 {
		t.Fatal("back-to-back deletes must be paced through the limiter")
	}
}

type fakeCacheClock struct {
	now   time.Time
	slept time.Duration
}

func (c *fakeCacheClock) Now() time.Time {
	return c.now
}

func (c *fakeCacheClock) Sleep(d time.Duration) {
	c.slept += d
	c.now = c.now.Add(d)
}
