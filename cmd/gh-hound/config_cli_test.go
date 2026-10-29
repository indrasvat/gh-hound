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
	out, err := launchWithConfig(t, "theme = \"bone\"\nwelcome = false\n", "q")
	if err != nil {
		t.Fatal(err)
	}
	// Bone's OK green; Bramble draws the frame in 79;211;122.
	if bone := "38;2;31;158;85"; !strings.Contains(out, bone) {
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
		set   func(*cliOptions) // what parsing that flag stored; "" for strings, which only the flag check can keep
		check func(cliOptions) bool
	}{
		{"HOUND_FORMAT", "md", "format", func(o *cliOptions) { o.Format = render.FormatXML }, func(o cliOptions) bool { return o.Format == render.FormatXML }},
		{"HOUND_NO_TUI", "true", "no-tui", func(o *cliOptions) { o.NoTUI = false }, func(o cliOptions) bool { return !o.NoTUI }},
		{"HOUND_JSON", "true", "json", func(o *cliOptions) { o.JSON = false }, func(o cliOptions) bool { return !o.JSON && !o.NoTUI }},
		{"HOUND_JSON", "true", "format", func(o *cliOptions) { o.Format = render.FormatMarkdown }, func(o cliOptions) bool { return o.Format == render.FormatMarkdown && !o.JSON }},
		{"HOUND_LOG_LEVEL", "debug", "log-level", func(o *cliOptions) { o.LogLevel = "warn" }, func(o cliOptions) bool { return o.LogLevel == "warn" }},
		{"HOUND_TRACE_HTTP", "true", "trace-http", func(o *cliOptions) { o.TraceHTTP = false }, func(o cliOptions) bool { return !o.TraceHTTP }},
		{"HOUND_ALL", "true", "all", func(o *cliOptions) { o.All = false }, func(o cliOptions) bool { return !o.All }},
		{"HOUND_REPO", "a/env", "repo", func(o *cliOptions) { o.Repo = "" }, func(o cliOptions) bool { return o.Repo == "" }},
		{"GH_REPO", "a/env", "repo", func(o *cliOptions) { o.Repo = "" }, func(o cliOptions) bool { return o.Repo == "" }},
		{"HOUND_BRANCH", "env", "branch", func(o *cliOptions) { o.Branch = "" }, func(o cliOptions) bool { return o.Branch == "" }},
		{"HOUND_STATUS", "failure", "status", func(o *cliOptions) { o.Status = "" }, func(o cliOptions) bool { return o.Status == "" }},
		{"HOUND_ACTOR", "env", "actor", func(o *cliOptions) { o.Actor = "" }, func(o cliOptions) bool { return o.Actor == "" && !o.ActorFromEnv }},
		{"HOUND_FAKE_SCENARIO", "failure", "fake-scenario", func(o *cliOptions) { o.Fake = "" }, func(o cliOptions) bool { return o.Fake == "" }},
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

func launchWithConfig(t *testing.T, body, stdin string) (string, error) {
	t.Helper()
	xdg := t.TempDir()
	writeConfig(t, xdg, body)
	var out bytes.Buffer
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &out,
		Stderr: &bytes.Buffer{},
		Stdin:  strings.NewReader(stdin),
		Env:    mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg}),
		IsTTY:  true,
		GitHub: &cliGitHub{runs: []model.Run{cliRun(901, "Release", model.StatusCompleted, model.ConclusionSuccess)}},
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{})
	_, err := executeCommand(cmd)
	return out.String(), err
}

// [keybindings] reaches the running TUI through the real key decoder.
func TestConfigKeybindingsMoveKeysInTheTUI(t *testing.T) {
	const cfg = "welcome = false\n[keybindings]\nhelp = \"h\"\n"
	out, err := launchWithConfig(t, cfg, "h")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "help · gh hound") || !strings.Contains(out, "h close") {
		t.Fatal("h did not open help with help = \"h\"")
	}
	out, err = launchWithConfig(t, cfg, "?")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "help · gh hound") {
		t.Fatal("? still opens help with help = \"h\"")
	}
}

func TestConfigKeybindingConflictStopsTheLaunch(t *testing.T) {
	_, err := launchWithConfig(t, "[keybindings]\ncancel = \"j\"\n", "q")
	if err == nil || !strings.Contains(err.Error(), "config.toml") || !strings.Contains(err.Error(), `"j" is both a fixed key and cancel`) {
		t.Fatalf("conflicting keybinding error = %v", err)
	}
}

func TestKeybindingsThatIsNotATableIsRejected(t *testing.T) {
	for _, body := range []string{`keybindings = "x"`, `keybindings = 5`, `keybindings = ["a"]`} {
		xdg := t.TempDir()
		path := writeConfig(t, xdg, body+"\n")
		if _, _, err := tuiConfig(mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg})); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: err = %v, want an error naming %s", body, err, path)
		}
	}
}

func TestHoundJSONStillDisablesTheTUIUnderAnExplicitFormat(t *testing.T) {
	opts := defaultCLIOptions()
	opts.Format = render.FormatMarkdown
	applyEnv(&opts, mapEnv(map[string]string{"HOUND_JSON": "1"}), func(name string) bool { return name == "format" })
	if !opts.NoTUI || opts.Format != render.FormatMarkdown {
		t.Fatalf("HOUND_JSON=1 --format md: NoTUI=%v Format=%s, want no TUI and md", opts.NoTUI, opts.Format)
	}
}

func TestRunsPipeReadsTheConfigFile(t *testing.T) {
	xdg := t.TempDir()
	writeConfig(t, xdg, "per_page = 0\n")
	cmd := newRootCommandWithRuntime(commandRuntime{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Env: mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg})}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--fake-scenario", "green", "--no-tui"})
	if _, err := executeCommand(cmd); err == nil || !strings.Contains(err.Error(), "per_page") {
		t.Fatalf("runs --no-tui with per_page = 0: err = %v", err)
	}
}

func TestRunsPipeListsConfigPerPage(t *testing.T) {
	xdg := t.TempDir()
	writeConfig(t, xdg, "per_page = 7\n")
	gh := &cliGitHub{runs: []model.Run{cliRun(901, "CI", model.StatusCompleted, model.ConclusionSuccess)}}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Env:    mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg}),
		GitHub: gh,
		Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
	}, testBuildInfo())
	cmd.SetArgs([]string{"runs", "--no-tui"})
	if _, err := executeCommand(cmd); err != nil {
		t.Fatal(err)
	}
	if len(gh.filters) == 0 || gh.filters[0].PerPage != 7 {
		t.Fatalf("runs filters %+v, want config per_page 7", gh.filters)
	}
}

func TestRelativeXDGConfigHomeIsIgnored(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".config", "gh-hound", "config.toml")
	if got := configPath(mapEnv(map[string]string{"XDG_CONFIG_HOME": "cfgrel", "HOME": home})); got != want {
		t.Fatalf("configPath = %q, want %q", got, want)
	}
}

// Commands that use no config value still refuse a broken file, in their
// own JSON envelope so agents branch on error.kind as for any refusal.
func TestEveryCommandRefusesABadConfigFileInItsEnvelope(t *testing.T) {
	xdg := t.TempDir()
	path := writeConfig(t, xdg, "per_page = = 1\n")
	for _, args := range [][]string{
		{"cancel", "--run", "123", "--fake-scenario", "green"},
		{"rerun", "--run", "123", "--fake-scenario", "green"},
		{"approvals", "--run", "123", "--no-tui", "--fake-scenario", "waiting"},
		{"workflows", "--no-tui", "--fake-scenario", "green"},
		{"caches", "--no-tui", "--fake-scenario", "green"},
		{"queue", "--run", "123", "--no-tui", "--fake-scenario", "queue"},
		{"diff", "--workflow", "ci.yml", "--no-tui", "--fake-scenario", "regression"},
		{"flakes", "--workflow", "ci.yml", "--no-tui", "--fake-scenario", "flaky"},
	} {
		var out bytes.Buffer
		cmd := newRootCommandWithRuntime(commandRuntime{Stdout: &out, Stderr: &bytes.Buffer{}, Env: mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg})}, testBuildInfo())
		cmd.SetArgs(args)
		if code, _ := executeCommand(cmd); code != render.ExitError {
			t.Errorf("%v: exit %d, want %d", args, code, render.ExitError)
		}
		var envelope struct {
			Error struct{ Kind, Message string } `json:"error"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Error.Kind != "validation" || !strings.Contains(envelope.Error.Message, path) {
			t.Errorf("%v: stdout %q, want a validation envelope naming the file", args, out.String())
		}
	}
	for _, args := range [][]string{{"version"}, {"-v"}, {"__screen", "--screen", "runs"}} {
		cmd := newRootCommandWithRuntime(commandRuntime{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Env: mapEnv(map[string]string{"XDG_CONFIG_HOME": xdg})}, testBuildInfo())
		cmd.SetArgs(args)
		if _, err := executeCommand(cmd); err != nil {
			t.Errorf("%v read the config: %v", args, err)
		}
	}
}
