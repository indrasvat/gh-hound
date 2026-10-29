package config

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func loadBody(t *testing.T, body string, env map[string]string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(LoadOptions{Path: path, LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }})
	return path, err
}

func TestLoadRejectsValuesTheFileCannotMean(t *testing.T) {
	tests := []struct{ body, want string }{
		{"poll_min_ms = 18446744073710\npoll_max_ms = 18446744073710\n", "poll_min_ms must be between 1 and 3600000"},
		{"poll_max_ms = 0\n", "poll_max_ms must be between 1 and 3600000"},
		{"poll_min_ms = -5\n", "poll_min_ms must be between 1 and 3600000"},
		{"theme = \"\"\n", `theme must be "bramble" or "bone", got ""`},
		{"default_scope = \"\"\n", `default_scope must be "branch" or "repo", got ""`},
		{"keybindings = \"x\"\n", "keybindings must be a table"},
	}
	for _, tt := range tests {
		path, err := loadBody(t, tt.body, nil)
		if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), path) {
			t.Errorf("%q: err = %v, want %q naming the file", tt.body, err, tt.want)
		}
	}
	if _, err := Load(LoadOptions{LookupEnv: func(k string) (string, bool) { return "18446744073710", k == "HOUND_POLL_MIN_MS" }}); err == nil {
		t.Error("HOUND_POLL_MIN_MS overflow was accepted")
	}
}

// Later layers win, so the file may be repaired by the environment.
func TestLoadValidatesTheMergedLayers(t *testing.T) {
	if _, err := loadBody(t, "poll_min_ms = 40000\n", map[string]string{"HOUND_POLL_MAX_MS": "60000"}); err != nil {
		t.Fatalf("file min 40s + env max 60s rejected: %v", err)
	}
	if _, err := loadBody(t, "poll_min_ms = 0\n", map[string]string{"HOUND_POLL_MIN_MS": "2000"}); err != nil {
		t.Fatalf("file poll_min_ms = 0 repaired by the env rejected: %v", err)
	}
	for _, tt := range []struct {
		body string
		env  map[string]string
	}{
		{"", map[string]string{"HOUND_PER_PAGE": "500"}},
		// The file's own error is repaired; the theme error is the env's.
		{"per_page = 500\n", map[string]string{"HOUND_PER_PAGE": "30", "HOUND_THEME": "plaid"}},
	} {
		path, err := loadBody(t, tt.body, tt.env)
		if err == nil || strings.Contains(err.Error(), path) {
			t.Errorf("%q + %v: an env error is blamed on the file: %v", tt.body, tt.env, err)
		}
	}
}

func TestLoadRefusesPathsThatAreNotAConfigFile(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo.toml")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(dir, "dangling.toml")
	if err := os.Symlink(filepath.Join(dir, "gone.toml"), dangling); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big.toml")
	if err := os.WriteFile(big, []byte("# "+strings.Repeat("x", 2<<20)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{fifo: "not a regular file", dangling: "broken symlink", big: "larger than"} {
		done := make(chan error, 1)
		go func() { _, err := Load(LoadOptions{Path: path, LookupEnv: emptyEnv}); done <- err }()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s: err = %v, want %q", filepath.Base(path), err, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: Load hung", filepath.Base(path))
		}
	}
}
