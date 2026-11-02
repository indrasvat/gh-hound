package watch_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/tui/screens/watch"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

var watchNow = time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

// fourMinutes is a baseline with p50 4m00s and p90 4m30s.
func fourMinutes() usecase.PaceBaseline {
	b := usecase.PaceBaseline{}
	for i, s := range []int{200, 220, 230, 235, 240, 245, 250, 260, 270, 280} {
		b.Samples = append(b.Samples, usecase.PaceSample{RunID: int64(9000 + i), Duration: time.Duration(s) * time.Second})
	}
	return b
}

func live(id int64, name string, elapsed time.Duration) model.Run {
	run := boardRun(id, name, model.StatusInProgress, model.ConclusionNone)
	run.RunAttempt = 1
	run.RunStartedAt = watchNow.Add(-elapsed)
	return run
}

func boardLine(t *testing.T, view, name string) string {
	t.Helper()
	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		if strings.Contains(line, " "+name+" ") {
			return line
		}
	}
	t.Fatalf("no %s row:\n%s", name, ansi.Strip(view))
	return ""
}

func TestBoardStateCellCarriesPaceAndHeadlineSaysWhenAllAreHome(t *testing.T) {
	home := boardRun(101, "CI", model.StatusCompleted, model.ConclusionSuccess)
	runs := []model.Run{home, live(102, "Release", 48*time.Second), live(104, "Lint", 3*time.Minute)}
	board := watch.NewBoard("o/r", "main", runs[1], runs)
	board.Pace = map[int64]usecase.PaceBaseline{102: fourMinutes(), 104: fourMinutes()}
	view := watch.BoardViewSize(board, 120, 0, watchNow)

	if row := boardLine(t, view, "Release"); !strings.Contains(row, " 4m left ") || !strings.Contains(row, "0m48s") {
		t.Fatalf("Release row: %q, want 4m left beside elapsed 0m48s", row)
	}
	if row := boardLine(t, view, "Lint"); !strings.Contains(row, " 1m left ") {
		t.Fatalf("Lint row: %q", row)
	}
	if row := boardLine(t, view, "CI"); !strings.Contains(row, " success ") {
		t.Fatalf("a finished row keeps its state: %q", row)
	}
	headline := strings.SplitN(ansi.Strip(view), "\n", 2)[0]
	if !strings.Contains(headline, "· all home in ~4m") {
		t.Fatalf("headline = %q, want the slowest running ETA", headline)
	}
	// Header and rows share one column math: State starts in one cell.
	header := strings.Split(ansi.Strip(view), "\n")[2]
	col := len([]rune(header[:strings.Index(header, "State")]))
	for name, state := range map[string]string{"Release": "4m left", "Lint": "1m left", "CI": "success"} {
		if row := []rune(boardLine(t, view, name)); !strings.HasPrefix(string(row[col:]), state) {
			t.Fatalf("%s State cell misaligned: %q", name, string(row))
		}
	}
}

// One running run without an estimate means nobody can say when the
// hunt ends: no headline suffix.
func TestBoardHeadlineNeedsEveryRunningRunEstimated(t *testing.T) {
	runs := []model.Run{live(102, "Release", 48*time.Second), live(104, "Lint", 9*time.Minute)}
	board := watch.NewBoard("o/r", "main", runs[0], runs)
	board.Pace = map[int64]usecase.PaceBaseline{102: fourMinutes(), 104: fourMinutes()}
	view := watch.BoardViewSize(board, 120, 0, watchNow)
	if strings.Contains(ansi.Strip(view), "all home") {
		t.Fatalf("an overrun run has no ETA; the headline must not guess:\n%s", ansi.Strip(view))
	}
	if row := boardLine(t, view, "Lint"); !strings.Contains(row, "↑ 9m00s") {
		t.Fatalf("overrun row: %q", row)
	}
	board.Pace = nil
	if view := ansi.Strip(watch.BoardViewSize(board, 120, 0, watchNow)); strings.Contains(view, "left") || strings.Contains(view, "all home") || !strings.Contains(view, "in_progress") {
		t.Fatalf("no baselines, no pace:\n%s", view)
	}
}

func TestWatchHeaderCarriesTimeLeft(t *testing.T) {
	run := live(570, "CI", 48*time.Second)
	m := watch.NewModel(watch.State{Repo: "o/r", Branch: "main", Run: run, Elapsed: "0m48s", Now: watchNow})
	m.Pace = fourMinutes()
	head := strings.SplitN(ansi.Strip(watch.View(m, 120)), "\n", 2)[0]
	if !strings.Contains(head, "0m48s · 4m left follow ●") {
		t.Fatalf("header = %q, want time left after elapsed", head)
	}
	m.State.Run = live(570, "CI", 9*time.Minute)
	if head := strings.SplitN(ansi.Strip(watch.View(m, 120)), "\n", 2)[0]; !strings.Contains(head, "· ↑ overrun") {
		t.Fatalf("overrun header = %q", head)
	}
	m.Pace = usecase.PaceBaseline{}
	if head := strings.SplitN(ansi.Strip(watch.View(m, 120)), "\n", 2)[0]; strings.Contains(head, "left") || strings.Contains(head, "↑") {
		t.Fatalf("no baseline header = %q", head)
	}
}

// A long estimate keeps its own prefix: never "~>9h".
func TestBoardHeadlineLongEstimate(t *testing.T) {
	b := usecase.PaceBaseline{}
	for i := range 10 {
		b.Samples = append(b.Samples, usecase.PaceSample{RunID: int64(9000 + i), Duration: time.Duration(11*3600+i) * time.Second})
	}
	runs := []model.Run{live(102, "Nightly", time.Minute)}
	board := watch.NewBoard("o/r", "main", runs[0], runs)
	board.Pace = map[int64]usecase.PaceBaseline{102: b}
	headline := strings.SplitN(ansi.Strip(watch.BoardViewSize(board, 120, 0, watchNow)), "\n", 2)[0]
	if !strings.Contains(headline, "all home in >9h") || strings.Contains(headline, "~>") {
		t.Fatalf("headline = %q", headline)
	}
}

// At 80 columns the follow state outranks the all-home estimate.
func TestBoardHeadlineKeepsFollowAtEighty(t *testing.T) {
	home := boardRun(101, "CI", model.StatusCompleted, model.ConclusionSuccess)
	runs := []model.Run{home, live(102, "Release", 48*time.Second)}
	board := watch.NewBoard("o/r", "main", runs[1], runs)
	board.Pace = map[int64]usecase.PaceBaseline{102: fourMinutes()}
	for _, width := range []int{78, 118} {
		headline := strings.SplitN(ansi.Strip(watch.BoardViewSize(board, width, 0, watchNow)), "\n", 2)[0]
		if !strings.Contains(headline, "follow ○") {
			t.Fatalf("width %d: follow state lost: %q", width, headline)
		}
		if width == 118 && !strings.Contains(headline, "all home in ~4m") {
			t.Fatalf("width %d: room for both, estimate missing: %q", width, headline)
		}
	}
}

// The frame colours the header line by state only when the line has no
// colour of its own: the pace suffix must stay plain.
func TestWatchHeaderPaceSuffixIsPlain(t *testing.T) {
	m := watch.NewModel(watch.State{Repo: "o/r", Branch: "main", Run: live(570, "CI", 9*time.Minute), Elapsed: "9m00s", Now: watchNow})
	m.Pace = fourMinutes()
	head := strings.SplitN(watch.View(m, 120), "\n", 2)[0]
	if head != ansi.Strip(head) {
		t.Fatalf("header carries its own colour: %q", head)
	}
}

// A queued or approval-waiting member has no estimate and may never
// start, so the board must not promise when everyone is home.
func TestBoardHeadlineMakesNoPromiseWhileAMemberWaits(t *testing.T) {
	for _, status := range []model.Status{model.StatusQueued, model.StatusWaiting} {
		waiting := boardRun(105, "Deploy", status, model.ConclusionNone)
		runs := []model.Run{live(102, "Release", 48*time.Second), waiting}
		board := watch.NewBoard("o/r", "main", runs[0], runs)
		board.Pace = map[int64]usecase.PaceBaseline{102: fourMinutes(), 105: fourMinutes()}
		headline := strings.SplitN(ansi.Strip(watch.BoardViewSize(board, 120, 0, watchNow)), "\n", 2)[0]
		if strings.Contains(headline, "all home") {
			t.Fatalf("%s member: headline = %q, want no all-home estimate", status, headline)
		}
	}
}
