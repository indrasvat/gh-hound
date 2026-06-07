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
	PerPage      int
	Theme        Theme
	PollMin      time.Duration
	PollMax      time.Duration
	Keybindings  map[string]string
}

type LoadOptions struct {
	Path      string
	LookupEnv func(string) (string, bool)
	Overrides Overrides
}

type Overrides struct {
	DefaultScope *Scope
	AutoWatch    *bool
	PerPage      *int
	Theme        *Theme
	PollMin      *time.Duration
	PollMax      *time.Duration
	Keybindings  map[string]string
}

type fileConfig struct {
	DefaultScope string            `toml:"default_scope"`
	AutoWatch    *bool             `toml:"auto_watch"`
	PerPage      *int              `toml:"per_page"`
	Theme        string            `toml:"theme"`
	PollMinMS    *int              `toml:"poll_min_ms"`
	PollMaxMS    *int              `toml:"poll_max_ms"`
	Keybindings  map[string]string `toml:"keybindings"`
}

//go:fix inline
func Ptr[T any](value T) *T {
	return new(value)
}

func Default() Config {
	return Config{
		DefaultScope: ScopeBranch,
		AutoWatch:    false,
		PerPage:      30,
		Theme:        ThemeBramble,
		PollMin:      2 * time.Second,
		PollMax:      30 * time.Second,
		Keybindings:  map[string]string{},
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
	if fc.PerPage != nil {
		cfg.PerPage = *fc.PerPage
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
	if value, ok := lookup("HOUND_PER_PAGE"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse HOUND_PER_PAGE: %w", err)
		}
		cfg.PerPage = parsed
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
	if overrides.PerPage != nil {
		cfg.PerPage = *overrides.PerPage
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
