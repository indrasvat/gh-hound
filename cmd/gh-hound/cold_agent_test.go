package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestColdContextAgentsFollowTargetMismatchHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-based cold agent harness is Unix-only")
	}
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "gh"), `#!/bin/sh
set -eu
if [ "${1:-}" != "hound" ]; then
  echo "expected gh hound invocation" >&2
  exit 127
fi
shift
exec "$GH_HOUND_TEST_EXE" -test.run '^TestColdAgentGhHelper$' -- "$@"
`)
	for _, agent := range []string{"codex", "claude", "agy"} {
		writeExecutable(t, filepath.Join(binDir, agent), coldAgentScript(agent))
	}

	for _, agent := range []string{"codex", "claude", "agy"} {
		t.Run(agent, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(binDir, agent), "Inspect the latest release/tag workflow with gh-hound.")
			cmd.Dir = t.TempDir()
			cmd.Env = []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"HOME=" + t.TempDir(),
				"GH_HOUND_TEST_EXE=" + os.Args[0],
				"GH_HOUND_TEST_HELPER=1",
			}
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s cold agent failed: %v\nstdout:\n%s\nstderr:\n%s", agent, err, out.String(), stderr.String())
			}
			if !strings.Contains(out.String(), agent+" OK") {
				t.Fatalf("%s cold agent did not finish the gh-hound flow:\n%s", agent, out.String())
			}
		})
	}
}

func TestColdAgentGhHelper(t *testing.T) {
	if os.Getenv("GH_HOUND_TEST_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	cmd := newRootCommandWithRuntime(commandRuntime{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    emptyEnv,
		IsTTY:  false,
	}, testBuildInfo())
	cmd.SetArgs(args)
	code, err := executeCommand(cmd)
	if err != nil && !isOutcome(err) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

func coldAgentScript(agent string) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
initial="$(gh hound runs -R indrasvat/gh-hound --branch release/v1 --no-tui --json --fake-scenario empty)"
printf '%%s\n' "$initial" | jq -e '.diagnostic.kind == "target_mismatch"' >/dev/null || { echo "%[1]s missing target_mismatch diagnostic" >&2; exit 20; }
printf '%%s\n' "$initial" | jq -e '
  .diagnostic.next_commands as $cmds |
  ($cmds | index("gh hound runs -R indrasvat/gh-hound --all --no-tui --json") != null) and
  ($cmds | index("gh hound runs -R indrasvat/gh-hound --run <id> --no-tui --json") != null) and
  ($cmds | index("gh hound watch -R indrasvat/gh-hound --group --run <run-id> --no-tui --timeout 10m") != null)
' >/dev/null || { echo "%[1]s missing concrete fallback commands" >&2; printf '%%s\n' "$initial" >&2; exit 21; }
repo_wide="$(gh hound runs -R indrasvat/gh-hound --all --no-tui --json --fake-scenario green)"
printf '%%s\n' "$repo_wide" | jq -e 'any(.runs[]?; .id == 30433642)' >/dev/null || { echo "%[1]s could not discover repo-wide run" >&2; printf '%%s\n' "$repo_wide" >&2; exit 22; }
explicit="$(gh hound runs -R indrasvat/gh-hound --run 30433642 --no-tui --json --fake-scenario green)"
printf '%%s\n' "$explicit" | jq -e 'has("diagnostic") | not' >/dev/null || { echo "%[1]s saw diagnostic on explicit run" >&2; printf '%%s\n' "$explicit" >&2; exit 23; }
printf '%[1]s OK\n'
`, agent)
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}
