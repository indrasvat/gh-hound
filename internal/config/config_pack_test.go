package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchGroupMaxDefaultsToTen(t *testing.T) {
	if got := Default().WatchGroupMax; got != 10 {
		t.Fatalf("WatchGroupMax default = %d, want 10", got)
	}
}

func TestWatchGroupMaxFromFileEnvAndOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("watch_group_max = 4\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(LoadOptions{Path: path, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.WatchGroupMax != 4 {
		t.Fatalf("file WatchGroupMax = %d, want 4", cfg.WatchGroupMax)
	}

	cfg, err = Load(LoadOptions{LookupEnv: func(key string) (string, bool) {
		if key == "HOUND_WATCH_GROUP_MAX" {
			return "7", true
		}
		return "", false
	}})
	if err != nil {
		t.Fatalf("Load env returned error: %v", err)
	}
	if cfg.WatchGroupMax != 7 {
		t.Fatalf("env WatchGroupMax = %d, want 7", cfg.WatchGroupMax)
	}

	max := 3
	cfg, err = Load(LoadOptions{
		LookupEnv: func(string) (string, bool) { return "", false },
		Overrides: Overrides{WatchGroupMax: &max},
	})
	if err != nil {
		t.Fatalf("Load override returned error: %v", err)
	}
	if cfg.WatchGroupMax != 3 {
		t.Fatalf("override WatchGroupMax = %d, want 3", cfg.WatchGroupMax)
	}
}

func TestWatchGroupMaxValidation(t *testing.T) {
	cfg := Default()
	cfg.WatchGroupMax = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "watch_group_max") {
		t.Fatalf("Validate(0) = %v, want watch_group_max error", err)
	}
	cfg.WatchGroupMax = 51
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate(51) should fail")
	}
}
