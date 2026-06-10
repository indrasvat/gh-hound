package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfigMatchesPRD(t *testing.T) {
	got := Default()

	if got.DefaultScope != ScopeBranch {
		t.Fatalf("DefaultScope = %q, want %q", got.DefaultScope, ScopeBranch)
	}
	if got.AutoWatch {
		t.Fatal("AutoWatch should default false")
	}
	if got.PerPage != 30 {
		t.Fatalf("PerPage = %d, want 30", got.PerPage)
	}
	if got.Theme != ThemeBramble {
		t.Fatalf("Theme = %q, want %q", got.Theme, ThemeBramble)
	}
	if got.PollMin != 2*time.Second {
		t.Fatalf("PollMin = %s, want 2s", got.PollMin)
	}
	if got.PollMax != 30*time.Second {
		t.Fatalf("PollMax = %s, want 30s", got.PollMax)
	}
}

func TestLoadMergesFileEnvAndFlagsInPrecedenceOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
default_scope = "repo"
auto_watch = true
per_page = 10
theme = "bone"
poll_min_ms = 3000
poll_max_ms = 45000

[keybindings]
rerun_failed = "f"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := map[string]string{
		"HOUND_PER_PAGE": "42",
		"HOUND_THEME":    "bramble",
	}
	got, err := Load(LoadOptions{
		Path: path,
		LookupEnv: func(key string) (string, bool) {
			value, ok := env[key]
			return value, ok
		},
		Overrides: Overrides{
			DefaultScope: new(ScopeBranch),
			AutoWatch:    new(false),
		},
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got.DefaultScope != ScopeBranch {
		t.Fatalf("flag DefaultScope override = %q, want branch", got.DefaultScope)
	}
	if got.AutoWatch {
		t.Fatal("flag AutoWatch override should win over file")
	}
	if got.PerPage != 42 {
		t.Fatalf("env PerPage override = %d, want 42", got.PerPage)
	}
	if got.Theme != ThemeBramble {
		t.Fatalf("env Theme override = %q, want bramble", got.Theme)
	}
	if got.Keybindings["rerun_failed"] != "f" {
		t.Fatalf("file keybinding was not loaded: %#v", got.Keybindings)
	}
}

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	got, err := Load(LoadOptions{
		Path:      filepath.Join(t.TempDir(), "missing.toml"),
		LookupEnv: emptyEnv,
	})
	if err != nil {
		t.Fatalf("Load missing config returned error: %v", err)
	}
	assertCoreDefaults(t, got)
}

func TestLoadInvalidTOMLReturnsClearError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`default_scope = [`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(LoadOptions{Path: path, LookupEnv: emptyEnv})
	if err == nil {
		t.Fatal("Load accepted invalid TOML")
	}
}

func TestValidateRejectsKeybindingConflicts(t *testing.T) {
	cfg := Default()
	cfg.Keybindings = map[string]string{
		"rerun_failed": "r",
		"rerun_run":    "r",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted duplicate keybindings")
	}
}

func emptyEnv(string) (string, bool) {
	return "", false
}

func assertCoreDefaults(t *testing.T, got Config) {
	t.Helper()

	want := Default()
	if got.DefaultScope != want.DefaultScope ||
		got.AutoWatch != want.AutoWatch ||
		got.PerPage != want.PerPage ||
		got.Theme != want.Theme ||
		got.PollMin != want.PollMin ||
		got.PollMax != want.PollMax {
		t.Fatalf("config = %#v, want core defaults %#v", got, want)
	}
}
