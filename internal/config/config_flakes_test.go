package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlakeDefaults(t *testing.T) {
	cfg := Default()
	if cfg.FlakeWindow != 50 {
		t.Fatalf("FlakeWindow default = %d, want 50", cfg.FlakeWindow)
	}
	if !cfg.FlakeBadges {
		t.Fatal("FlakeBadges default = false, want true")
	}
}

func TestFlakeWindowFromFileEnvAndOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("flake_window = 25\nflake_badges = false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(LoadOptions{Path: path, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.FlakeWindow != 25 {
		t.Fatalf("file FlakeWindow = %d, want 25", cfg.FlakeWindow)
	}
	if cfg.FlakeBadges {
		t.Fatal("file FlakeBadges = true, want false")
	}

	cfg, err = Load(LoadOptions{LookupEnv: func(key string) (string, bool) {
		switch key {
		case "HOUND_FLAKE_WINDOW":
			return "80", true
		case "HOUND_FLAKE_BADGES":
			return "false", true
		}
		return "", false
	}})
	if err != nil {
		t.Fatalf("Load env returned error: %v", err)
	}
	if cfg.FlakeWindow != 80 {
		t.Fatalf("env FlakeWindow = %d, want 80", cfg.FlakeWindow)
	}
	if cfg.FlakeBadges {
		t.Fatal("env FlakeBadges = true, want false")
	}

	window := 10
	badges := false
	cfg, err = Load(LoadOptions{
		LookupEnv: func(string) (string, bool) { return "", false },
		Overrides: Overrides{FlakeWindow: &window, FlakeBadges: &badges},
	})
	if err != nil {
		t.Fatalf("Load override returned error: %v", err)
	}
	if cfg.FlakeWindow != 10 {
		t.Fatalf("override FlakeWindow = %d, want 10", cfg.FlakeWindow)
	}
	if cfg.FlakeBadges {
		t.Fatal("override FlakeBadges = true, want false")
	}
}

func TestFlakeWindowValidation(t *testing.T) {
	cfg := Default()
	cfg.FlakeWindow = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "flake_window") {
		t.Fatalf("Validate(0) = %v, want flake_window error", err)
	}
	cfg.FlakeWindow = 501
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "flake_window") {
		t.Fatalf("Validate(501) = %v, want flake_window error", err)
	}
	cfg.FlakeWindow = 500
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate(500) = %v, want nil", err)
	}
}

func TestFlakeEnvParseErrors(t *testing.T) {
	_, err := Load(LoadOptions{LookupEnv: func(key string) (string, bool) {
		if key == "HOUND_FLAKE_WINDOW" {
			return "lots", true
		}
		return "", false
	}})
	if err == nil || !strings.Contains(err.Error(), "HOUND_FLAKE_WINDOW") {
		t.Fatalf("bad HOUND_FLAKE_WINDOW = %v, want parse error", err)
	}
	_, err = Load(LoadOptions{LookupEnv: func(key string) (string, bool) {
		if key == "HOUND_FLAKE_BADGES" {
			return "sometimes", true
		}
		return "", false
	}})
	if err == nil || !strings.Contains(err.Error(), "HOUND_FLAKE_BADGES") {
		t.Fatalf("bad HOUND_FLAKE_BADGES = %v, want parse error", err)
	}
}
