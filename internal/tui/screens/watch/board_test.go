package watch_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
)

func boardRun(id int64, name string, status model.Status, conclusion model.Conclusion) model.Run {
	return model.Run{
		ID:           id,
		Name:         name,
		HeadSHA:      "9f8e7d6c5b4a3928",
		Event:        "push",
		Status:       status,
		Conclusion:   conclusion,
		RunNumber:    int(id % 1000),
		RunStartedAt: time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 11, 10, 2, 8, 0, time.UTC),
	}
}

func samplePack() []model.Run {
	return []model.Run{
		boardRun(101, "CI", model.StatusCompleted, model.ConclusionSuccess),
		boardRun(102, "Release", model.StatusInProgress, model.ConclusionNone),
		boardRun(103, "Docs", model.StatusCompleted, model.ConclusionFailure),
	}
}

func TestBoardSelectionAndIntents(t *testing.T) {
	board := watch.NewBoard("indrasvat/gh-hound", "main", samplePack()[1], samplePack())
	if board.Selected != 1 {
		t.Fatalf("anchor selection = %d, want 1", board.Selected)
	}

	board = board.Update(watch.KeyMsg{Key: "j"})
	if board.Selected != 2 {
		t.Fatalf("j selection = %d, want 2", board.Selected)
	}
	board = board.Update(watch.KeyMsg{Key: "j"})
	if board.Selected != 2 {
		t.Fatalf("j past end = %d, want clamped 2", board.Selected)
	}
	board = board.Update(watch.KeyMsg{Key: "k"})
	if board.Selected != 1 {
		t.Fatalf("k selection = %d, want 1", board.Selected)
	}

	board = board.Update(watch.KeyMsg{Key: "enter"})
	if board.Intent.Kind != watch.BoardIntentDrill || board.Intent.RunID != 102 {
		t.Fatalf("enter intent = %#v, want drill 102", board.Intent)
	}
	board = board.Update(watch.KeyMsg{Key: "x"})
	if board.Intent.Kind != watch.BoardIntentCancel || board.Intent.RunID != 102 {
		t.Fatalf("x intent = %#v, want cancel 102", board.Intent)
	}
	board = board.Update(watch.KeyMsg{Key: "esc"})
	if board.Intent.Kind != watch.BoardIntentBack {
		t.Fatalf("esc intent = %#v, want back", board.Intent)
	}
}

func TestBoardFollowWorstTracksAcrossPolls(t *testing.T) {
	pack := samplePack()
	board := watch.NewBoard("indrasvat/gh-hound", "main", pack[0], pack)
	board = board.Update(watch.KeyMsg{Key: "f"})
	if !board.Follow {
		t.Fatal("f must enable follow")
	}
	// Worst right now is the lost Docs run.
	if board.Selected != 2 {
		t.Fatalf("follow selection = %d, want the lost run (2)", board.Selected)
	}

	// Docs recovers (rerun to green) and Release fails: follow retargets.
	next := samplePack()
	next[2].Conclusion = model.ConclusionSuccess
	next[1].Status = model.StatusCompleted
	next[1].Conclusion = model.ConclusionFailure
	board = board.WithRuns(next)
	if board.Selected != 1 {
		t.Fatalf("follow retarget = %d, want the newly lost run (1)", board.Selected)
	}

	// Manual movement takes the leash back.
	board = board.Update(watch.KeyMsg{Key: "k"})
	if board.Follow {
		t.Fatal("manual movement must disable follow")
	}
	if board.Selected != 0 {
		t.Fatalf("manual selection = %d, want 0", board.Selected)
	}
}

func TestBoardWithRunsKeepsSelectionByRunID(t *testing.T) {
	pack := samplePack()
	board := watch.NewBoard("indrasvat/gh-hound", "main", pack[2], pack)
	// A joiner lands at the front of the ID-sorted order.
	next := append([]model.Run{boardRun(100, "Lint", model.StatusInProgress, model.ConclusionNone)}, samplePack()...)
	board = board.WithRuns(next)
	run, ok := board.SelectedRun()
	if !ok || run.ID != 103 {
		t.Fatalf("selection after joiner = %#v, want run 103 still selected", run)
	}
}

func TestBoardViewHeaderAndRowsShareColumnMath(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 3, 0, 0, time.UTC)
	board := watch.NewBoard("indrasvat/gh-hound", "main", samplePack()[0], samplePack())
	view := watch.BoardView(board, 100, now)
	plain := ansi.Strip(view)
	lines := strings.Split(plain, "\n")

	if !strings.Contains(lines[0], "the pack: 1 running · 1 home · 1 lost") {
		t.Fatalf("aggregate header missing, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "9f8e7d6 push") || !strings.Contains(lines[0], "follow ○") {
		t.Fatalf("scent/follow state missing from header: %q", lines[0])
	}

	header := lines[2]
	headerElapsed := strings.Index(header, "Elapsed")
	if headerElapsed < 0 {
		t.Fatalf("column header missing: %q", header)
	}
	// Completed rows pin start→finish: 2m08s, right-aligned under the
	// Elapsed header (same width math, no wobble).
	ciRow := lines[3]
	if !strings.Contains(ciRow, "CI") || !strings.Contains(ciRow, "success") || !strings.Contains(ciRow, "2m08s") {
		t.Fatalf("CI row = %q", ciRow)
	}
	rowElapsedEnd := len([]rune(strings.TrimRight(ciRow, " ")))
	headerElapsedEnd := headerElapsed + len("Elapsed")
	if rowElapsedEnd != headerElapsedEnd {
		t.Fatalf("elapsed column misaligned: header ends at %d, row at %d\n%q\n%q", headerElapsedEnd, rowElapsedEnd, header, ciRow)
	}
	// Live rows tick start→now (3m00s at the pinned clock).
	if !strings.Contains(lines[4], "Release") || !strings.Contains(lines[4], "in_progress") || !strings.Contains(lines[4], "3m00s") {
		t.Fatalf("Release row = %q", lines[4])
	}
	if !strings.Contains(lines[5], "Docs") || !strings.Contains(lines[5], "failure") {
		t.Fatalf("Docs row = %q", lines[5])
	}
	// The cursor sits on the anchor row.
	if !strings.HasPrefix(lines[3], "▌") {
		t.Fatalf("anchor row missing cursor: %q", lines[3])
	}
}

func TestBoardViewEmptyPack(t *testing.T) {
	board := watch.Board{Repo: "indrasvat/gh-hound"}
	view := ansi.Strip(watch.BoardView(board, 80, time.Now()))
	if !strings.Contains(view, "the pack is empty") {
		t.Fatalf("empty pack view = %q", view)
	}
}
