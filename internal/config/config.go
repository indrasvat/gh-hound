package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

type Scope string

const (
	ScopeBranch Scope = "branch"
	ScopeRepo   Scope = "repo"
)

type Theme string

const (
	ThemeBramble Theme = "bramble"
	ThemeBone    Theme = "bone"
)

type Config struct {
	DefaultScope Scope
	AutoWatch    bool
	Welcome      bool
	PerPage      int
	// DiffMaxPages bounds the regression scan's history walk (pages of
	// 100 runs). Hitting the cap yields an inconclusive verdict.
	DiffMaxPages int
	// WatchGroupMax caps how many runs one pack board watches; the
	// group poll budget stays one list call per tick regardless.
	WatchGroupMax int
	Theme         Theme
	PollMin       time.Duration
	PollMax       time.Duration
	Keybindings   map[string]string
	// FlakeWindow is how many recent runs the flake scan reads per
	// workflow+branch before issuing a verdict.
	FlakeWindow int
	// FlakeBadges marks runs-list rows whose workflow has a known
	// flaker (from verdicts already computed this session — the badge
	// never spends API calls of its own).
	FlakeBadges bool
	// LogCompact opens full logs with repeating lines folded.
	LogCompact bool
}

type LoadOptions struct {
	Path      string
	LookupEnv func(string) (string, bool)
	Overrides Overrides
}

type Overrides struct {
	DefaultScope  *Scope
	AutoWatch     *bool
	Welcome       *bool
	PerPage       *int
	DiffMaxPages  *int
	WatchGroupMax *int
	Theme         *Theme
	PollMin       *time.Duration
	PollMax       *time.Duration
	Keybindings   map[string]string
	FlakeWindow   *int
	FlakeBadges   *bool
	LogCompact    *bool
}

type fileConfig struct {
	DefaultScope  *string           `toml:"default_scope"`
	AutoWatch     *bool             `toml:"auto_watch"`
	Welcome       *bool             `toml:"welcome"`
	PerPage       *int              `toml:"per_page"`
	DiffMaxPages  *int              `toml:"diff_max_pages"`
	WatchGroupMax *int              `toml:"watch_group_max"`
	Theme         *string           `toml:"theme"`
	PollMinMS     *int              `toml:"poll_min_ms"`
	PollMaxMS     *int              `toml:"poll_max_ms"`
	Keybindings   map[string]string `toml:"keybindings"`
	FlakeWindow   *int              `toml:"flake_window"`
	FlakeBadges   *bool             `toml:"flake_badges"`
	LogCompact    *bool             `toml:"log_compact"`
}

//go:fix inline
func Ptr[T any](value T) *T {
	return new(value)
}

func Default() Config {
	return Config{
		DefaultScope:  ScopeBranch,
		AutoWatch:     false,
		Welcome:       true,
		PerPage:       30,
		DiffMaxPages:  10,
		WatchGroupMax: 10,
		Theme:         ThemeBramble,
		PollMin:       2 * time.Second,
		PollMax:       30 * time.Second,
		Keybindings:   map[string]string{},
		FlakeWindow:   50,
		FlakeBadges:   true,
		LogCompact:    true,
	}
}

func Load(opts LoadOptions) (Config, error) {
	cfg := Default()
	if opts.LookupEnv == nil {
		opts.LookupEnv = os.LookupEnv
	}

	fileOnly := cfg
	if opts.Path != "" {
		if err := loadFile(opts.Path, &cfg); err != nil {
			return Config{}, err
		}
		fileOnly = cfg
	}
	if err := applyEnv(opts.LookupEnv, &cfg); err != nil {
		return Config{}, err
	}
	applyOverrides(opts.Overrides, &cfg)
	if err := cfg.Validate(); err != nil {
		// Later layers may repair the file (its poll min against an env
		// max), so the file is blamed only when it is wrong by itself.
		if opts.Path != "" && fileOnly.Validate() != nil {
			return Config{}, fmt.Errorf("config %s: %w", opts.Path, err)
		}
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.DefaultScope != ScopeBranch && c.DefaultScope != ScopeRepo {
		return fmt.Errorf("default_scope must be %q or %q, got %q", ScopeBranch, ScopeRepo, c.DefaultScope)
	}
	if c.Theme != ThemeBramble && c.Theme != ThemeBone {
		return fmt.Errorf("theme must be %q or %q, got %q", ThemeBramble, ThemeBone, c.Theme)
	}
	if c.PerPage < 1 || c.PerPage > 100 {
		return fmt.Errorf("per_page must be between 1 and 100, got %d", c.PerPage)
	}
	if c.DiffMaxPages < 1 || c.DiffMaxPages > 100 {
		return fmt.Errorf("diff_max_pages must be between 1 and 100, got %d", c.DiffMaxPages)
	}
	if c.WatchGroupMax < 1 || c.WatchGroupMax > 50 {
		return fmt.Errorf("watch_group_max must be between 1 and 50, got %d", c.WatchGroupMax)
	}
	if c.FlakeWindow < 1 || c.FlakeWindow > 500 {
		return fmt.Errorf("flake_window must be between 1 and 500, got %d", c.FlakeWindow)
	}
	if c.PollMin <= 0 || c.PollMax <= 0 || c.PollMin > c.PollMax {
		return fmt.Errorf("poll interval must be positive and min <= max, got min=%s max=%s", c.PollMin, c.PollMax)
	}
	// Keybindings are checked per screen by the TUI's keymap
	// (keys.NewKeymap), the only reader.
	return nil
}

// maxFileSize bounds the read: a config is a few hundred bytes, and a
// path pointing at /dev/zero must not eat memory.
const maxFileSize = 1 << 20

func readFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, lerr := os.Lstat(path); lerr == nil {
				return nil, fmt.Errorf("load config %s: broken symlink", path)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	// A FIFO or device would block or never end.
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("load config %s: not a regular file", path)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("load config %s: larger than %d bytes", path, maxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	return data, nil
}

// pollMS bounds a millisecond value before it becomes a Duration, so a
// huge one cannot wrap around into a fast poll.
func pollMS(name string, ms int) (time.Duration, error) {
	if ms < 1 || ms > 3_600_000 {
		return 0, fmt.Errorf("%s must be between 1 and 3600000, got %d", name, ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func loadFile(path string, cfg *Config) error {
	data, err := readFile(path)
	if err != nil || data == nil {
		return err
	}
	var fc fileConfig
	meta, err := toml.Decode(string(data), &fc)
	if err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return fmt.Errorf("load config %s: unknown key %q", path, undecoded[0].String())
	}
	// The decoder drops a non-table [keybindings] without a word.
	if meta.IsDefined("keybindings") && meta.Type("keybindings") != "Hash" {
		return fmt.Errorf("load config %s: keybindings must be a table", path)
	}
	if fc.DefaultScope != nil {
		cfg.DefaultScope = Scope(*fc.DefaultScope)
	}
	if fc.AutoWatch != nil {
		cfg.AutoWatch = *fc.AutoWatch
	}
	if fc.Welcome != nil {
		cfg.Welcome = *fc.Welcome
	}
	if fc.PerPage != nil {
		cfg.PerPage = *fc.PerPage
	}
	if fc.DiffMaxPages != nil {
		cfg.DiffMaxPages = *fc.DiffMaxPages
	}
	if fc.WatchGroupMax != nil {
		cfg.WatchGroupMax = *fc.WatchGroupMax
	}
	if fc.FlakeWindow != nil {
		cfg.FlakeWindow = *fc.FlakeWindow
	}
	if fc.FlakeBadges != nil {
		cfg.FlakeBadges = *fc.FlakeBadges
	}
	if fc.LogCompact != nil {
		cfg.LogCompact = *fc.LogCompact
	}
	if fc.Theme != nil {
		cfg.Theme = Theme(*fc.Theme)
	}
	if fc.PollMinMS != nil {
		if cfg.PollMin, err = pollMS("poll_min_ms", *fc.PollMinMS); err != nil {
			return fmt.Errorf("config %s: %w", path, err)
		}
	}
	if fc.PollMaxMS != nil {
		if cfg.PollMax, err = pollMS("poll_max_ms", *fc.PollMaxMS); err != nil {
			return fmt.Errorf("config %s: %w", path, err)
		}
	}
	if fc.Keybindings != nil {
		cfg.Keybindings = cloneMap(fc.Keybindings)
	}
	return nil
}

func applyEnv(lookup func(string) (string, bool), cfg *Config) error {
	if value, ok := lookup("HOUND_DEFAULT_SCOPE"); ok {
		cfg.DefaultScope = Scope(value)
	}
	if value, ok := lookup("HOUND_AUTO_WATCH"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_AUTO_WATCH: %w", err)
		}
		cfg.AutoWatch = parsed
	}
	if value, ok := lookup("HOUND_WELCOME"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_WELCOME: %w", err)
		}
		cfg.Welcome = parsed
	}
	if value, ok := lookup("HOUND_PER_PAGE"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_PER_PAGE: %w", err)
		}
		cfg.PerPage = parsed
	}
	if value, ok := lookup("HOUND_DIFF_MAX_PAGES"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_DIFF_MAX_PAGES: %w", err)
		}
		cfg.DiffMaxPages = parsed
	}
	if value, ok := lookup("HOUND_WATCH_GROUP_MAX"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_WATCH_GROUP_MAX: %w", err)
		}
		cfg.WatchGroupMax = parsed
	}
	if value, ok := lookup("HOUND_FLAKE_WINDOW"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_FLAKE_WINDOW: %w", err)
		}
		cfg.FlakeWindow = parsed
	}
	if value, ok := lookup("HOUND_FLAKE_BADGES"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_FLAKE_BADGES: %w", err)
		}
		cfg.FlakeBadges = parsed
	}
	if value, ok := lookup("HOUND_LOG_COMPACT"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_LOG_COMPACT: %w", err)
		}
		cfg.LogCompact = parsed
	}
	if value, ok := lookup("HOUND_THEME"); ok {
		cfg.Theme = Theme(value)
	}
	if value, ok := lookup("HOUND_POLL_MIN_MS"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_POLL_MIN_MS: %w", err)
		}
		if cfg.PollMin, err = pollMS("HOUND_POLL_MIN_MS", parsed); err != nil {
			return err
		}
	}
	if value, ok := lookup("HOUND_POLL_MAX_MS"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_POLL_MAX_MS: %w", err)
		}
		if cfg.PollMax, err = pollMS("HOUND_POLL_MAX_MS", parsed); err != nil {
			return err
		}
	}
	return nil
}

func applyOverrides(overrides Overrides, cfg *Config) {
	if overrides.DefaultScope != nil {
		cfg.DefaultScope = *overrides.DefaultScope
	}
	if overrides.AutoWatch != nil {
		cfg.AutoWatch = *overrides.AutoWatch
	}
	if overrides.Welcome != nil {
		cfg.Welcome = *overrides.Welcome
	}
	if overrides.PerPage != nil {
		cfg.PerPage = *overrides.PerPage
	}
	if overrides.DiffMaxPages != nil {
		cfg.DiffMaxPages = *overrides.DiffMaxPages
	}
	if overrides.WatchGroupMax != nil {
		cfg.WatchGroupMax = *overrides.WatchGroupMax
	}
	if overrides.FlakeWindow != nil {
		cfg.FlakeWindow = *overrides.FlakeWindow
	}
	if overrides.FlakeBadges != nil {
		cfg.FlakeBadges = *overrides.FlakeBadges
	}
	if overrides.LogCompact != nil {
		cfg.LogCompact = *overrides.LogCompact
	}
	if overrides.Theme != nil {
		cfg.Theme = *overrides.Theme
	}
	if overrides.PollMin != nil {
		cfg.PollMin = *overrides.PollMin
	}
	if overrides.PollMax != nil {
		cfg.PollMax = *overrides.PollMax
	}
	for action, key := range overrides.Keybindings {
		if cfg.Keybindings == nil {
			cfg.Keybindings = map[string]string{}
		}
		cfg.Keybindings[action] = key
	}
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
