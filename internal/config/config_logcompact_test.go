package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogCompactLayers(t *testing.T) {
	if !Default().LogCompact {
		t.Fatal("LogCompact default = false, want true")
	}
	noEnv := func(string) (string, bool) { return "", false }
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("log_compact = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{Path: path, LookupEnv: noEnv})
	if err != nil || cfg.LogCompact {
		t.Fatalf("file: LogCompact=%v err=%v, want false", cfg.LogCompact, err)
	}
	env := func(key string) (string, bool) {
		if key == "HOUND_LOG_COMPACT" {
			return "true", true
		}
		return "", false
	}
	cfg, err = Load(LoadOptions{Path: path, LookupEnv: env})
	if err != nil || !cfg.LogCompact {
		t.Fatalf("env: LogCompact=%v err=%v, want true", cfg.LogCompact, err)
	}
	cfg, err = Load(LoadOptions{Path: path, LookupEnv: env, Overrides: Overrides{LogCompact: Ptr(false)}})
	if err != nil || cfg.LogCompact {
		t.Fatalf("override: LogCompact=%v err=%v, want false", cfg.LogCompact, err)
	}
	bad := func(key string) (string, bool) { return "maybe", key == "HOUND_LOG_COMPACT" }
	if _, err := Load(LoadOptions{Path: path, LookupEnv: bad}); err == nil || !strings.Contains(err.Error(), "HOUND_LOG_COMPACT") {
		t.Fatalf("bad env err = %v", err)
	}
}
