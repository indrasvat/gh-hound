package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzLoadFile: whatever bytes sit in config.toml, Load never panics,
// and an error names the file and comes without a half-applied config.
func FuzzLoadFile(f *testing.F) {
	for _, seed := range []string{
		"",
		"per_page = 7\ntheme = \"bone\"\n",
		"default_scope = \"repo\"\npoll_min_ms = 500\npoll_max_ms = 400\n",
		"[keybindings]\ncancel = \"c\"\nhelp = \"c\"\n",
		"per_page = = 3\n",
		"unknown = 1\n",
		"per_page = 9223372036854775807\n",
		"poll_min_ms = -1\n",
		"[keybindings]\n\"\" = \"x\"\n",
		"theme = \"\\u0000\"\n",
	} {
		f.Add(seed)
	}
	empty := func(string) (string, bool) { return "", false }
	f.Fuzz(func(t *testing.T, body string) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(LoadOptions{Path: path, LookupEnv: empty})
		if err != nil {
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not name the file", err)
			}
			if cfg.Theme != "" {
				t.Fatalf("error came with a config: %+v", cfg)
			}
		}
	})
}
