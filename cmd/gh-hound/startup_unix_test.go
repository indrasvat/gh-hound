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

	"github.com/indrasvat/gh-hound/internal/model"
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

// stalledRerunGitHub never answers a rerun until its context ends, like
// GitHub hanging on a confirmed mutation.
type stalledRerunGitHub struct {
	*cliGitHub
	entered chan struct{}
}

func (g stalledRerunGitHub) RerunRun(ctx context.Context, _ string, _ int64, _ bool) (usecase.ActionResult, error) {
	close(g.entered)
	<-ctx.Done()
	return usecase.ActionResult{}, ctx.Err()
}

// A signal must end the program even while a confirmed action is stuck
// on the network inside the event loop.
func TestTerminationSignalDuringAStalledActionRestoresTheTerminal(t *testing.T) {
	out := &syncBuffer{}
	stdin, keys := io.Pipe()
	t.Cleanup(func() { _ = keys.Close() })
	github := stalledRerunGitHub{
		cliGitHub: &cliGitHub{runs: []model.Run{cliRun(910, "CI", model.StatusCompleted, model.ConclusionFailure)}},
		entered:   make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		done <- runTUI(context.Background(), commandRuntime{
			Stdout: out, Stderr: io.Discard, Stdin: stdin, IsTTY: true,
			Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
			GitHub: github,
			Repo:   &cliRepo{context: usecase.RepositoryContext{Repo: "indrasvat/gh-hound", Branch: "main"}},
		}, buildInfo{Version: "test"}, cliOptions{})
	}()
	waitForOutput(t, out, "rows 1-1/1")
	go func() { _, _ = io.WriteString(keys, "ry") }()
	select {
	case <-github.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the rerun never reached GitHub")
	}
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
		t.Fatal("SIGTERM did not end a program stuck on a stalled action")
	}
	if !strings.HasSuffix(out.String(), showCursor+leaveAlt) {
		t.Fatal("terminal not restored")
	}
}
