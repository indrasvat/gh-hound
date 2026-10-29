package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/render"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "gh-hound", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultConfigReadsTheXDGConfigFile(t *testing.T) {
	xdg := t.TempDir()
	writeConfig(t, xdg, "per_page = 7\ntheme = \"bone\"\n")
	home := t.TempDir()
	writeConfig(t, filepath.Join(home, ".config"), "per_page = 9\n")

	tests := []struct {
		name    string
		env     map[string]string
		perPage int
	}{
		{"XDG_CONFIG_HOME wins over HOME", map[string]string{"XDG_CONFIG_HOME": xdg, "HOME": home}, 7},
		{"HOME/.config without XDG_CONFIG_HOME", map[string]string{"HOME": home}, 9},
		{"env beats the file", map[string]string{"XDG_CONFIG_HOME": xdg, "HOUND_PER_PAGE": "11"}, 11},
		{"missing file is defaults", map[string]string{"XDG_CONFIG_HOME": t.TempDir()}, 30},
		{"no home at all is defaults", map[string]string{}, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := defaultConfig(mapEnv(tt.env))
			if err != nil {
				t.Fatalf("defaultConfig: %v", err)
			}
			if cfg.PerPage != tt.perPage {
				t.Fatalf("per_page = %d, want %d", cfg.PerPage, tt.perPage)
			}
		})
	}
}

func TestDefaultConfigRejectsABadFileByPath(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"invalid TOML", "per_page = = 3\n", "config.toml"},
		{"unknown key", "per_pag = 3\n", `unknown key "per_pag"`},
		{"out of range", "per_page = 500\n", "per_page must be between 1 and 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xdg := t.TempDir()
			path := writeConfig(t, xdg, tt.body)
			_, err := defaultConfig(mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg}))
			if err == nil {
				t.Fatalf("defaultConfig accepted %q", tt.body)
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q should name %q and the file %s", err, tt.want, path)
			}
		})
	}
}

// The launch reads the file: a bone theme in config.toml paints the
// Bone palette, which nothing else in this launch would select.
func TestTUILaunchAppliesTheConfigFileTheme(t *testing.T) {
	xdg := t.TempDir()
	writeConfig(t, xdg, "theme = \"bone\"\nwelcome = false\n")
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Stdin:  strings.NewReader("q"),
		Env:    mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg}),
		IsTTY:  true,
		GitHub: &cliGitHub{runs: []model.Run{cliRun(901, "Release", model.StatusCompleted, model.ConclusionSuccess)}},
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{})
	if code, err := executeCommand(cmd); err != nil || code != 0 {
		t.Fatalf("tty root code=%d err=%v", code, err)
	}
	// Bone's OK green; Bramble draws the frame in 79;211;122.
	if bone := "38;2;31;158;85"; !strings.Contains(out.String(), bone) {
		t.Fatalf("config theme=bone did not paint the Bone frame (%s)", bone)
	}
}

func TestTUILaunchFailsLoudOnABadConfigFile(t *testing.T) {
	xdg := t.TempDir()
	path := writeConfig(t, xdg, "theme = \"plaid\"\n")
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Stdin:  strings.NewReader("q"),
		Env:    mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg}),
		IsTTY:  true,
		GitHub: &cliGitHub{},
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{})
	code, err := executeCommand(cmd)
	if err == nil || code != render.ExitError {
		t.Fatalf("bad config code=%d err=%v, want an error exit", code, err)
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), `"plaid"`) {
		t.Fatalf("error %q should name the file and the bad value", err)
	}
}

func TestExplicitFlagsBeatEveryHoundVariable(t *testing.T) {
	tests := []struct {
		env   string
		value string
		flag  string            // the explicit flag
		set   func(*cliOptions) // what parsing that flag stored
		check func(cliOptions) bool
	}{
		{"HOUND_FORMAT", "md", "format", func(o *cliOptions) { o.Format = render.FormatXML }, func(o cliOptions) bool { return o.Format == render.FormatXML }},
		{"HOUND_NO_TUI", "true", "no-tui", func(o *cliOptions) { o.NoTUI = false }, func(o cliOptions) bool { return !o.NoTUI }},
		{"HOUND_JSON", "true", "json", func(o *cliOptions) { o.JSON = false }, func(o cliOptions) bool { return !o.JSON && !o.NoTUI }},
		{"HOUND_JSON", "true", "format", func(o *cliOptions) { o.Format = render.FormatMarkdown }, func(o cliOptions) bool { return o.Format == render.FormatMarkdown && !o.JSON }},
		{"HOUND_LOG_LEVEL", "debug", "log-level", func(o *cliOptions) { o.LogLevel = "warn" }, func(o cliOptions) bool { return o.LogLevel == "warn" }},
		{"HOUND_TRACE_HTTP", "true", "trace-http", func(o *cliOptions) { o.TraceHTTP = false }, func(o cliOptions) bool { return !o.TraceHTTP }},
		{"HOUND_ALL", "true", "all", func(o *cliOptions) { o.All = false }, func(o cliOptions) bool { return !o.All }},
		{"HOUND_REPO", "a/env", "repo", func(o *cliOptions) { o.Repo = "b/flag" }, func(o cliOptions) bool { return o.Repo == "b/flag" }},
		{"GH_REPO", "a/env", "repo", func(o *cliOptions) { o.Repo = "b/flag" }, func(o cliOptions) bool { return o.Repo == "b/flag" }},
		{"HOUND_BRANCH", "env", "branch", func(o *cliOptions) { o.Branch = "flag" }, func(o cliOptions) bool { return o.Branch == "flag" }},
		{"HOUND_STATUS", "failure", "status", func(o *cliOptions) { o.Status = "success" }, func(o cliOptions) bool { return o.Status == "success" }},
		{"HOUND_ACTOR", "env", "actor", func(o *cliOptions) { o.Actor = "flag" }, func(o cliOptions) bool { return o.Actor == "flag" && !o.ActorFromEnv }},
		{"HOUND_FAKE_SCENARIO", "failure", "fake-scenario", func(o *cliOptions) { o.Fake = "green" }, func(o cliOptions) bool { return o.Fake == "green" }},
	}
	for _, tt := range tests {
		t.Run(tt.env+" vs --"+tt.flag, func(t *testing.T) {
			env := mapEnv(map[string]string{tt.env: tt.value})
			flagged := defaultCLIOptions()
			tt.set(&flagged)
			applyEnv(&flagged, env, func(name string) bool { return name == tt.flag })
			if !tt.check(flagged) {
				t.Fatalf("%s=%s overrode --%s: %+v", tt.env, tt.value, tt.flag, flagged)
			}
			// Without the flag the variable still beats the default.
			unflagged := defaultCLIOptions()
			applyEnv(&unflagged, env, func(string) bool { return false })
			if tt.check(unflagged) {
				t.Fatalf("%s=%s did not apply without --%s: %+v", tt.env, tt.value, tt.flag, unflagged)
			}
		})
	}
}

// The issue's repro, through cobra's real flag parsing.
func TestExplicitFormatBeatsHoundFormatEndToEnd(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Env:    mapEnv(map[string]string{"HOUND_FORMAT": "md", "HOUND_FAKE_SCENARIO": "green"}),
		IsTTY:  true,
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--format", "json", "--no-tui"})
	if code, err := executeCommand(cmd); err != nil || code != 0 {
		t.Fatalf("runs code=%d err=%v", code, err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("--format json printed non-JSON under HOUND_FORMAT=md:\n%s", out.String())
	}
}
