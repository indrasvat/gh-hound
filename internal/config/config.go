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
}

type fileConfig struct {
	DefaultScope  string            `toml:"default_scope"`
	AutoWatch     *bool             `toml:"auto_watch"`
	Welcome       *bool             `toml:"welcome"`
	PerPage       *int              `toml:"per_page"`
	DiffMaxPages  *int              `toml:"diff_max_pages"`
	WatchGroupMax *int              `toml:"watch_group_max"`
	Theme         string            `toml:"theme"`
	PollMinMS     *int              `toml:"poll_min_ms"`
	PollMaxMS     *int              `toml:"poll_max_ms"`
	Keybindings   map[string]string `toml:"keybindings"`
	FlakeWindow   *int              `toml:"flake_window"`
	FlakeBadges   *bool             `toml:"flake_badges"`
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
	}
}

func Load(opts LoadOptions) (Config, error) {
	cfg := Default()
	if opts.LookupEnv == nil {
		opts.LookupEnv = os.LookupEnv
	}

	if opts.Path != "" {
		if err := loadFile(opts.Path, &cfg); err != nil {
			return Config{}, err
		}
	}
	if err := applyEnv(opts.LookupEnv, &cfg); err != nil {
		return Config{}, err
	}
	applyOverrides(opts.Overrides, &cfg)
	if err := cfg.Validate(); err != nil {
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
	seen := map[string]string{}
	for action, key := range c.Keybindings {
		if key == "" {
			return fmt.Errorf("keybinding %q cannot be empty", action)
		}
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("keybinding conflict: %q and %q both use %q", previous, action, key)
		}
		seen[key] = action
	}
	return nil
}

func loadFile(path string, cfg *Config) error {
	var fc fileConfig
	if _, err := toml.DecodeFile(path, &fc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load config %s: %w", path, err)
	}
	if fc.DefaultScope != "" {
		cfg.DefaultScope = Scope(fc.DefaultScope)
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
	if fc.Theme != "" {
		cfg.Theme = Theme(fc.Theme)
	}
	if fc.PollMinMS != nil {
		cfg.PollMin = time.Duration(*fc.PollMinMS) * time.Millisecond
	}
	if fc.PollMaxMS != nil {
		cfg.PollMax = time.Duration(*fc.PollMaxMS) * time.Millisecond
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
	if value, ok := lookup("HOUND_THEME"); ok {
		cfg.Theme = Theme(value)
	}
	if value, ok := lookup("HOUND_POLL_MIN_MS"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_POLL_MIN_MS: %w", err)
		}
		cfg.PollMin = time.Duration(parsed) * time.Millisecond
	}
	if value, ok := lookup("HOUND_POLL_MAX_MS"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_POLL_MAX_MS: %w", err)
		}
		cfg.PollMax = time.Duration(parsed) * time.Millisecond
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
