package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// blockedGitHub holds every runs listing until release closes, like a
// slow network under a live launch.
type blockedGitHub struct {
	*cliGitHub
	release chan struct{}
}

func (g blockedGitHub) ListRuns(ctx context.Context, filter usecase.RunFilter) ([]model.Run, error) {
	select {
	case <-g.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.cliGitHub.ListRuns(ctx, filter)
}

func startBlockedTUI(t *testing.T) (*syncBuffer, io.Writer, blockedGitHub, <-chan error) {
	t.Helper()
	out := &syncBuffer{}
	stdin, keys := io.Pipe()
	t.Cleanup(func() { _ = keys.Close() })
	github := blockedGitHub{
		cliGitHub: &cliGitHub{runs: []model.Run{cliRun(910, "CI", model.StatusCompleted, model.ConclusionFailure)}},
		release:   make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		done <- runTUI(ctx, commandRuntime{
			Stdout: out,
			Stderr: io.Discard,
			Stdin:  stdin,
			Env:    mapEnv(map[string]string{"HOUND_WELCOME": "false"}),
			GitHub: github,
			Repo: &cliRepo{context: usecase.RepositoryContext{
				Repo: "indrasvat/gh-hound", Branch: "main", Actor: "indrasvat",
			}},
		}, buildInfo{Version: "test"}, cliOptions{})
	}()
	return out, keys, github, done
}

func waitForOutput(t *testing.T, out *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(ansi.Strip(out.String()), want) {
		if time.Now().After(deadline) {
			t.Fatalf("output never showed %q:\n%s", want, ansi.Strip(out.String()))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLiveLaunchPaintsBeforeTheRunsLoadAndKeepsTypedKeys(t *testing.T) {
	out, keys, github, done := startBlockedTUI(t)
	waitForOutput(t, out, "picking up the scent")
	if _, err := io.WriteString(keys, "?"); err != nil {
		t.Fatal(err)
	}
	close(github.release)
	waitForOutput(t, out, "help · gh hound")
	if _, err := io.WriteString(keys, "\x03"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("runTUI err = %v", err)
	}
}

func TestCtrlCQuitsWhileTheLaunchIsStillLoading(t *testing.T) {
	out, keys, _, done := startBlockedTUI(t)
	waitForOutput(t, out, "hound")
	if _, err := io.WriteString(keys, "\x03"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTUI err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ctrl+c did not quit while the launch was loading")
	}
}
