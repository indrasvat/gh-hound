package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaceDefaults(t *testing.T) {
	cfg := Default()
	if !cfg.Pace || cfg.PaceWindow != 20 {
		t.Fatalf("Pace %v PaceWindow %d, want on and 20", cfg.Pace, cfg.PaceWindow)
	}
}

// Overrides beat HOUND_* variables, which beat the file.
func TestPacePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("pace = false\npace_window = 30\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noEnv := func(string) (string, bool) { return "", false }
	cfg, err := Load(LoadOptions{Path: path, LookupEnv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pace || cfg.PaceWindow != 30 {
		t.Fatalf("file: Pace %v window %d, want off and 30", cfg.Pace, cfg.PaceWindow)
	}
	env := func(key string) (string, bool) {
		switch key {
		case "HOUND_PACE":
			return "true", true
		case "HOUND_PACE_WINDOW":
			return "40", true
		}
		return "", false
	}
	cfg, err = Load(LoadOptions{Path: path, LookupEnv: env})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Pace || cfg.PaceWindow != 40 {
		t.Fatalf("env over file: Pace %v window %d, want on and 40", cfg.Pace, cfg.PaceWindow)
	}
	window, pace := 50, false
	cfg, err = Load(LoadOptions{Path: path, LookupEnv: env, Overrides: Overrides{PaceWindow: &window, Pace: &pace}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Pace || cfg.PaceWindow != 50 {
		t.Fatalf("override over env: Pace %v window %d, want off and 50", cfg.Pace, cfg.PaceWindow)
	}
}

func TestPaceWindowValidation(t *testing.T) {
	for _, bad := range []int{0, 4, 101} {
		cfg := Default()
		cfg.PaceWindow = bad
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "pace_window must be between 5 and 100") {
			t.Fatalf("Validate(%d) = %v, want a pace_window range error", bad, err)
		}
	}
	for _, good := range []int{5, 100} {
		cfg := Default()
		cfg.PaceWindow = good
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%d) = %v", good, err)
		}
	}
	for key, value := range map[string]string{"HOUND_PACE": "maybe", "HOUND_PACE_WINDOW": "lots"} {
		_, err := Load(LoadOptions{LookupEnv: func(k string) (string, bool) {
			if k == key {
				return value, true
			}
			return "", false
		}})
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("%s=%s: err %v, want a parse error naming the variable", key, value, err)
		}
	}
}
