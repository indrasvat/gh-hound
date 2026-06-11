package caches

import (
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func testData() Data {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	return Data{
		Usage: model.CacheUsage{ActiveSizeInBytes: 3 << 30, ActiveCount: 3},
		Caches: []model.Cache{
			{ID: 1, Key: "setup-go-Linux-x64", Ref: "refs/heads/main", SizeInBytes: 2 << 30, LastAccessedAt: now.Add(-2 * time.Hour)},
			{ID: 2, Key: "go-mod-Linux", Ref: "refs/heads/main", SizeInBytes: 512 << 20, LastAccessedAt: now.Add(-90 * 24 * time.Hour)},
			{ID: 3, Key: "go-mod-Linux", Ref: "refs/pull/7/merge", SizeInBytes: 512 << 20, LastAccessedAt: now.Add(-time.Minute)},
		},
	}
}

func TestModelSortsBySizeByDefaultAndTogglesToStalest(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", testData())
	visible := m.VisibleCaches()
	if visible[0].ID != 1 {
		t.Fatalf("default sort must put the biggest first, got %#v", visible[0])
	}
	m = m.Update(KeyMsg{Key: "s"})
	if m.SortBy != usecase.CacheSortLastUsed {
		t.Fatalf("s must toggle sort, got %s", m.SortBy)
	}
	visible = m.VisibleCaches()
	if visible[0].ID != 2 {
		t.Fatalf("last-used sort must put the stalest first, got %#v", visible[0])
	}
}

func TestModelFilterIsClientSideKeySubstring(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", testData())
	for _, key := range []string{"/", "g", "o", "-", "m", "o", "d", "enter"} {
		m = m.Update(KeyMsg{Key: key})
	}
	if m.InputMode {
		t.Fatal("enter must commit the filter")
	}
	if got := len(m.VisibleCaches()); got != 2 {
		t.Fatalf("filtered rows = %d, want 2", got)
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.Filter != "" || len(m.VisibleCaches()) != 3 {
		t.Fatal("esc must clear the filter before acting as back")
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.Intent.Kind != IntentBack {
		t.Fatal("esc on an unfiltered kennel must intend back")
	}
}

func TestModelDeleteIntentsCarrySelectionAndMatchCount(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", testData())
	m = m.Update(KeyMsg{Key: "j"})
	cache, ok := m.SelectedCache()
	if !ok {
		t.Fatal("selection lost")
	}
	m = m.Update(KeyMsg{Key: "d"})
	if m.Intent.Kind != IntentDelete || m.Intent.CacheID != cache.ID {
		t.Fatalf("d intent wrong: %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "D"})
	if m.Intent.Kind != IntentDeleteKey || m.Intent.Key != cache.Key {
		t.Fatalf("D intent wrong: %#v", m.Intent)
	}
	if m.MatchCount("go-mod-Linux") != 2 {
		t.Fatalf("MatchCount = %d, want 2", m.MatchCount("go-mod-Linux"))
	}
	if m.KeyBytes("go-mod-Linux") != 1<<30 {
		t.Fatalf("KeyBytes = %d, want 1 GiB", m.KeyBytes("go-mod-Linux"))
	}
}

func TestModelFoldsDeletesIntoUsage(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", testData())
	m = m.WithoutCache(1)
	if len(m.Caches) != 2 || m.Usage.ActiveCount != 2 {
		t.Fatalf("WithoutCache wrong: %#v", m.Usage)
	}
	if m.Usage.ActiveSizeInBytes != (3<<30)-(2<<30) {
		t.Fatalf("usage must drop by the deleted size, got %d", m.Usage.ActiveSizeInBytes)
	}
	m = m.WithoutKey("go-mod-Linux")
	if len(m.Caches) != 0 || m.Usage.ActiveCount != 0 || m.Usage.ActiveSizeInBytes != 0 {
		t.Fatalf("WithoutKey wrong: %#v", m.Usage)
	}
}

func TestViewGaugeStates(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	calm := NewModel("indrasvat/gh-hound", testData())
	view := View(calm, 100, now)
	if !strings.Contains(view, "kennel: 3/10 GB") {
		t.Fatalf("usage headline missing:\n%s", view)
	}
	if strings.Contains(view, "kennel's almost full") {
		t.Fatalf("30%% must not warn:\n%s", view)
	}

	hot := calm
	hot.Usage.ActiveSizeInBytes = 9*(1<<30) + 700*(1<<20)
	view = View(hot, 100, now)
	if !strings.Contains(view, "kennel's almost full — GitHub starts evicting at 10 GB.") {
		t.Fatalf(">90%% must warn:\n%s", view)
	}
}

func TestViewEmptyKennelSpeaksHound(t *testing.T) {
	m := NewModel("indrasvat/gh-hound", Data{})
	view := View(m, 80, time.Now())
	if !strings.Contains(view, "the kennel's empty — nothing cached on this repo.") {
		t.Fatalf("empty state missing:\n%s", view)
	}
}

func TestViewTruncatesLongKeysWithinWidth(t *testing.T) {
	data := testData()
	data.Caches[0].Key = strings.Repeat("setup-go-Linux-x64-ubuntu24-", 8)
	m := NewModel("indrasvat/gh-hound", data)
	for _, width := range []int{78, 118, 198} {
		view := View(m, width, time.Now())
		for line := range strings.SplitSeq(view, "\n") {
			if got := visibleWidth(line); got > width {
				t.Fatalf("line overflows %d cols (%d): %q", width, got, line)
			}
		}
		if !strings.Contains(view, "…") {
			t.Fatalf("long key must truncate with ellipsis at %d cols:\n%s", width, view)
		}
	}
}

func visibleWidth(line string) int {
	return len([]rune(stripANSI(line)))
}

func stripANSI(value string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range value {
		switch {
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		case r == '\x1b':
			inEscape = true
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
