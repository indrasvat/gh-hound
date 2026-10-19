//go:build unix

package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// A kill during a hung launch is exactly when users reach for it: the
// terminal must come back (main screen, cursor) and the exit code must
// say which signal ended it.
func TestTerminationSignalDuringStartupRestoresTheTerminal(t *testing.T) {
	out := &syncBuffer{}
	stdin, keys := io.Pipe()
	t.Cleanup(func() { _ = keys.Close() })
	github := blockedGitHub{cliGitHub: &cliGitHub{}, release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- runTUI(context.Background(), commandRuntime{
			Stdout: out,
			Stderr: io.Discard,
			Stdin:  stdin,
			Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
			IsTTY:  true,
			GitHub: github,
			Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
		}, buildInfo{Version: "test"}, cliOptions{})
	}()
	waitForOutput(t, out, "hound")
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		var outcome outcomeError
		if !errors.As(err, &outcome) || outcome.code != 143 {
			t.Fatalf("runTUI err = %v, want exit 143", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SIGTERM did not end the launch")
	}
	if !strings.HasSuffix(out.String(), showCursor+leaveAlt) {
		t.Fatalf("terminal not restored: %q", out.String()[max(len(out.String())-40, 0):])
	}
}
