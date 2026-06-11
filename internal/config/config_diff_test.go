package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffMaxPagesDefaultsToTen(t *testing.T) {
	if got := Default().DiffMaxPages; got != 10 {
		t.Fatalf("DiffMaxPages default = %d, want 10", got)
	}
}

func TestDiffMaxPagesFromFileEnvAndOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("diff_max_pages = 4\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(LoadOptions{Path: path, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DiffMaxPages != 4 {
		t.Fatalf("file DiffMaxPages = %d, want 4", cfg.DiffMaxPages)
	}

	cfg, err = Load(LoadOptions{LookupEnv: func(key string) (string, bool) {
		if key == "HOUND_DIFF_MAX_PAGES" {
			return "7", true
		}
		return "", false
	}})
	if err != nil {
		t.Fatalf("Load env returned error: %v", err)
	}
	if cfg.DiffMaxPages != 7 {
		t.Fatalf("env DiffMaxPages = %d, want 7", cfg.DiffMaxPages)
	}

	pages := 3
	cfg, err = Load(LoadOptions{
		LookupEnv: func(string) (string, bool) { return "", false },
		Overrides: Overrides{DiffMaxPages: &pages},
	})
	if err != nil {
		t.Fatalf("Load override returned error: %v", err)
	}
	if cfg.DiffMaxPages != 3 {
		t.Fatalf("override DiffMaxPages = %d, want 3", cfg.DiffMaxPages)
	}
}

func TestDiffMaxPagesValidation(t *testing.T) {
	cfg := Default()
	cfg.DiffMaxPages = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "diff_max_pages") {
		t.Fatalf("Validate(0) = %v, want diff_max_pages error", err)
	}
	cfg.DiffMaxPages = 101
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate(101) should fail")
	}
}
