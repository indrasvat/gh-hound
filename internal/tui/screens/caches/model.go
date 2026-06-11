package caches

import (
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone IntentKind = ""
	// IntentDelete digs up the selected cache by ID (always exactly
	// one match).
	IntentDelete IntentKind = "delete"
	// IntentDeleteKey digs up every loaded cache sharing the selected
	// key — the confirm shows the match count first.
	IntentDeleteKey IntentKind = "delete_key"
	IntentBack      IntentKind = "back"
)

type Intent struct {
	Kind    IntentKind
	CacheID int64
	Key     string
}

// Data is the resolver payload for the caches screen: one usage call
// plus the paginated listing.
type Data struct {
	Caches []model.Cache
	Usage  model.CacheUsage
}

type Model struct {
	Repo      string
	Usage     model.CacheUsage
	Cap       int64
	Caches    []model.Cache
	Selected  int
	Filter    string
	InputMode bool
	SortBy    usecase.CacheSort
	Intent    Intent

	// Loading state is transient render input set by the app from its
	// pending load each frame — never persisted on the model.
	Loading     bool
	LoadingLine string
}

func NewModel(repo string, data Data) Model {
	return Model{
		Repo:   repo,
		Usage:  data.Usage,
		Cap:    usecase.CacheCapFallbackBytes,
		Caches: data.Caches,
		SortBy: usecase.CacheSortSize,
	}
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	if m.InputMode {
		return m.updateInput(msg)
	}
	total := len(m.VisibleCaches())
	switch msg.Key {
	case "j", "down":
		if m.Selected < total-1 {
			m.Selected++
		}
	case "k", "up":
		if m.Selected > 0 {
			m.Selected--
		}
	case "g":
		m.Selected = 0
	case "G":
		if total > 0 {
			m.Selected = total - 1
		}
	case "s":
		m = m.toggleSort()
	case "/":
		m.InputMode = true
		m.Filter = ""
		m.Selected = 0
	case "esc":
		if m.Filter != "" {
			m.Filter = ""
			m.Selected = 0
			return m
		}
		m.Intent = Intent{Kind: IntentBack}
	case "d":
		if cache, ok := m.SelectedCache(); ok {
			m.Intent = Intent{Kind: IntentDelete, CacheID: cache.ID, Key: cache.Key}
		}
	case "D":
		if cache, ok := m.SelectedCache(); ok {
			m.Intent = Intent{Kind: IntentDeleteKey, Key: cache.Key}
		}
	}
	return m
}

func (m Model) updateInput(msg KeyMsg) Model {
	switch msg.Key {
	case "esc":
		m.InputMode = false
		m.Filter = ""
	case "enter":
		m.InputMode = false
		m.Selected = 0
	case "backspace":
		if len(m.Filter) > 0 {
			m.Filter = m.Filter[:len(m.Filter)-1]
		}
	case "space":
		m.Filter += " "
	default:
		if len([]rune(msg.Key)) == 1 {
			m.Filter += msg.Key
		}
	}
	return m
}

func (m Model) toggleSort() Model {
	if m.SortBy == usecase.CacheSortSize {
		m.SortBy = usecase.CacheSortLastUsed
	} else {
		m.SortBy = usecase.CacheSortSize
	}
	m.Selected = 0
	return m
}

// VisibleCaches is the rendered row set: client-side key filter, then
// the active sort (size: biggest first, last used: stalest first).
func (m Model) VisibleCaches() []model.Cache {
	return usecase.SortCaches(usecase.FilterCachesByKey(m.Caches, m.Filter), m.SortBy)
}

func (m Model) SelectedCache() (model.Cache, bool) {
	visible := m.VisibleCaches()
	if len(visible) == 0 {
		return model.Cache{}, false
	}
	selected := max(m.Selected, 0)
	if selected >= len(visible) {
		selected = len(visible) - 1
	}
	return visible[selected], true
}

// MatchCount reports how many loaded caches share a key — the number
// the delete confirm must show before anything is dug up.
func (m Model) MatchCount(key string) int {
	count := 0
	for _, cache := range m.Caches {
		if cache.Key == key {
			count++
		}
	}
	return count
}

// KeyBytes sums the size of every loaded cache sharing a key.
func (m Model) KeyBytes(key string) int64 {
	var total int64
	for _, cache := range m.Caches {
		if cache.Key == key {
			total += cache.SizeInBytes
		}
	}
	return total
}

// WithoutCache folds a confirmed single-cache delete into the model:
// the row disappears and the gauge drops by its size.
func (m Model) WithoutCache(id int64) Model {
	kept := make([]model.Cache, 0, len(m.Caches))
	for _, cache := range m.Caches {
		if cache.ID == id {
			m.Usage.ActiveSizeInBytes -= cache.SizeInBytes
			m.Usage.ActiveCount--
			continue
		}
		kept = append(kept, cache)
	}
	m.Caches = kept
	m.Selected = clampSelection(m.Selected, len(usecase.FilterCachesByKey(kept, m.Filter)))
	return m
}

// WithoutKey folds a confirmed key delete into the model.
func (m Model) WithoutKey(key string) Model {
	kept := make([]model.Cache, 0, len(m.Caches))
	for _, cache := range m.Caches {
		if cache.Key == key {
			m.Usage.ActiveSizeInBytes -= cache.SizeInBytes
			m.Usage.ActiveCount--
			continue
		}
		kept = append(kept, cache)
	}
	m.Caches = kept
	m.Selected = clampSelection(m.Selected, len(usecase.FilterCachesByKey(kept, m.Filter)))
	return m
}

func clampSelection(selected, total int) int {
	if total <= 0 || selected < 0 {
		return 0
	}
	if selected >= total {
		return total - 1
	}
	return selected
}
