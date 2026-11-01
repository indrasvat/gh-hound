package runs

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/theme"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

var paceNow = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

// fourMinutes is a baseline with p50 4m00s and p90 4m30s.
func fourMinutes() usecase.PaceBaseline {
	b := usecase.PaceBaseline{Event: "push", BaselineBranch: "main"}
	for i, s := range []int{200, 220, 230, 235, 240, 245, 250, 260, 270, 280} {
		b.Samples = append(b.Samples, usecase.PaceSample{RunID: int64(9000 + i), Duration: time.Duration(s) * time.Second})
	}
	return b
}

func pacedRow(id int64, number int, name string, took time.Duration) model.Run {
	run := successRun(id, number, name)
	run.RunStartedAt = paceNow.Add(-time.Hour)
	run.UpdatedAt = run.RunStartedAt.Add(took)
	run.RunAttempt = 1
	return run
}

func inProgress(id int64, number int, name string, elapsed time.Duration) model.Run {
	run := pacedRow(id, number, name, 0)
	run.Status, run.Conclusion = model.StatusInProgress, model.ConclusionNone
	run.RunStartedAt = paceNow.Add(-elapsed)
	run.UpdatedAt = paceNow.Add(-time.Second)
	return run
}

func pacedModel(runs ...model.Run) Model {
	m := NewModel(usecase.LaunchContext{Repo: "o/r", State: usecase.LaunchStateRuns, Runs: runs})
	m.Pace = map[int64]usecase.PaceBaseline{}
	for _, run := range runs {
		m.Pace[run.ID] = fourMinutes()
	}
	return m
}

// rowOf returns the stripped row for run #number.
func rowOf(t *testing.T, view string, number int) string {
	t.Helper()
	needle := "#" + strconv.Itoa(number) + " "
	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no row for #%d in:\n%s", number, ansi.Strip(view))
	return ""
}

// durationCell is the 8-cell Duration column of a 120-column row.
func durationCell(t *testing.T, view string, number int) string {
	t.Helper()
	header := ""
	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		if strings.Contains(line, "Duration") {
			header = line
			break
		}
	}
	before, _, ok := strings.Cut(header, "Duration")
	if !ok {
		t.Fatalf("no Duration header:\n%s", ansi.Strip(view))
	}
	row := []rune(rowOf(t, view, number))
	start := len([]rune(before))
	return strings.TrimSpace(string(row[start : start+8]))
}

// The placeholder sparkline encoded nothing; the cell is the run's
// real duration, with or without a baseline.
func TestDurationCellShowsRealDurations(t *testing.T) {
	m := NewModel(usecase.LaunchContext{Repo: "o/r", State: usecase.LaunchStateRuns, Runs: []model.Run{
		pacedRow(1, 101, "CI", 110*time.Second),
		pacedRow(2, 102, "Docs", 38*time.Second),
		pacedRow(3, 103, "Nightly", 65*time.Minute),
		inProgress(4, 104, "Smoke", 48*time.Second),
	}})
	m.Context.Runs[0].Conclusion = model.ConclusionFailure // keep the standard table
	view := ViewSize(m, 120, 30, paceNow)
	for number, want := range map[int]string{101: "1m50s", 102: "38s", 103: "1h05m", 104: "48s"} {
		if got := durationCell(t, view, number); got != want {
			t.Errorf("#%d duration = %q, want %q", number, got, want)
		}
	}
	for _, spark := range []string{"▁", "▂", "▃", "▄", "█"} {
		if strings.Contains(ansi.Strip(view), spark) {
			t.Fatalf("placeholder sparkline still drawn:\n%s", ansi.Strip(view))
		}
	}
}

func TestDurationCellCarriesPace(t *testing.T) {
	failing := pacedRow(1, 101, "CI", 140*time.Second)
	failing.Conclusion = model.ConclusionFailure
	m := pacedModel(
		failing,
		inProgress(2, 102, "CI", 60*time.Second),  // 3m left
		inProgress(3, 103, "CI", 250*time.Second), // past p50, not slow
		inProgress(4, 104, "CI", 400*time.Second), // overrun
		pacedRow(5, 105, "CI", 571*time.Second),   // slow
		pacedRow(6, 106, "CI", 245*time.Second),   // on pace
	)
	view := ViewSize(m, 120, 30, paceNow)
	want := map[int]string{101: "2m20s", 102: "3m left", 103: "due", 104: "↑ 6m40s", 105: "↑ 9m31s", 106: "4m05s"}
	for number, cell := range want {
		if got := durationCell(t, view, number); got != cell {
			t.Errorf("#%d cell = %q, want %q", number, got, cell)
		}
	}
	// The ETA counts down with the clock, not with fetches.
	later := ViewSize(m, 120, 30, paceNow.Add(120*time.Second))
	if got := durationCell(t, later, 102); got != "1m left" {
		t.Fatalf("two minutes on, #102 = %q, want 1m left", got)
	}
	if got := durationCell(t, ViewSize(m, 120, 30, paceNow.Add(150*time.Second)), 102); got != "<1m left" {
		t.Fatalf("30s from its median, #102 = %q, want <1m left", got)
	}
}

// Without a baseline nothing is estimated or flagged.
func TestNoBaselineNoEstimate(t *testing.T) {
	m := pacedModel(inProgress(2, 102, "CI", 60*time.Second), pacedRow(5, 105, "CI", 571*time.Second))
	m.Context.Runs[1].Conclusion = model.ConclusionFailure
	m.Pace = nil
	view := ViewSize(m, 120, 30, paceNow)
	if got := durationCell(t, view, 102); got != "1m00s" {
		t.Fatalf("#102 = %q, want elapsed 1m00s", got)
	}
	if strings.Contains(ansi.Strip(view), "↑") || strings.Contains(ansi.Strip(view), "left") {
		t.Fatalf("no baseline must mean no pace marks:\n%s", ansi.Strip(view))
	}
}

// At 80 columns Duration is dropped; rows with something to say carry
// it inside the label column, and every column stays aligned.
func TestEightyColumnsAnnotateTheLabelColumn(t *testing.T) {
	failing := pacedRow(1, 101, "CI · parser fix validation", 140*time.Second)
	failing.Conclusion = model.ConclusionFailure
	m := pacedModel(
		failing,
		inProgress(2, 102, "CI · push smoke", 60*time.Second),
		pacedRow(5, 105, "Release · a very long snapshot release name that truncates", 571*time.Second),
	)
	view := ViewSize(m, 80, 30, paceNow)
	plain := ansi.Strip(view)
	if strings.Contains(plain, "Duration") {
		t.Fatalf("80 columns must not show the Duration column:\n%s", plain)
	}
	running, slow, quiet := rowOf(t, view, 102), rowOf(t, view, 105), rowOf(t, view, 101)
	if !strings.Contains(running, " 3m left ") || !strings.Contains(slow, " ↑ 9m31s ") {
		t.Fatalf("annotations missing:\n%s\n%s", running, slow)
	}
	if strings.Contains(quiet, "left") || strings.Contains(quiet, "↑") {
		t.Fatalf("a row with nothing to say must not annotate: %q", quiet)
	}
	// The Actor column starts at the same cell on every row.
	header := ""
	for line := range strings.SplitSeq(plain, "\n") {
		if strings.Contains(line, "Actor") {
			header = line
		}
	}
	col := len([]rune(header[:strings.Index(header, "Actor")]))
	for _, line := range []string{running, slow, quiet} {
		if ansi.StringWidth(line) > 80 {
			t.Fatalf("row width %d overflows 80: %q", ansi.StringWidth(line), line)
		}
		if r := []rune(line); !strings.HasPrefix(string(r[col:]), "unavailable") {
			t.Fatalf("Actor column shifted from cell %d: %q", col, line)
		}
	}
	if !strings.Contains(slow, "…") {
		t.Fatalf("the label truncates before the annotation does: %q", slow)
	}
}

// A label never outgrows its column, however narrow: the name keeps
// what fits and the detail goes first.
func TestRunLabelNeverOutgrowsItsWidth(t *testing.T) {
	st := stylesFor(theme.Theme{}.OrBramble())
	run := successRun(1, 1, "Release")
	run.DisplayTitle = "snapshot release"
	for width := 1; width <= 30; width++ {
		if got := ansi.StringWidth(runLabel(st, run, width)); got > width {
			t.Fatalf("runLabel(width %d) is %d cells: %q", width, got, ansi.Strip(runLabel(st, run, width)))
		}
	}
}

// The all-green home is where a slow green run most often sits: its
// rows and the latest-run line carry the slow mark too, beside an
// Actor column that stays put.
func TestAllGreenCarriesSlowMarks(t *testing.T) {
	m := pacedModel(
		pacedRow(5, 105, "Release", 571*time.Second),
		pacedRow(6, 106, "CI", 245*time.Second),
	)
	m.Context.Runs[0].Actor, m.Context.Runs[1].Actor = "indrasvat", "indrasvat"
	for _, width := range []int{80, 120} {
		view := ansi.Strip(ViewSize(m, width, 30, paceNow))
		if !strings.Contains(view, "All checks passing") {
			t.Fatalf("want the all-green home:\n%s", view)
		}
		if !strings.Contains(view, "↑ 9m31s · success") {
			t.Fatalf("width %d: latest-run line lost the slow mark:\n%s", width, view)
		}
		slow, quiet := rowOf(t, view, 105), rowOf(t, view, 106)
		if !strings.Contains(slow, " ↑ 9m31s ") || strings.Contains(quiet, "↑") {
			t.Fatalf("width %d: rows %q / %q", width, slow, quiet)
		}
		if ansi.StringWidth(slow[:strings.Index(slow, "@indrasvat")]) != ansi.StringWidth(quiet[:strings.Index(quiet, "@indrasvat")]) {
			t.Fatalf("width %d: Actor moved:\n%q\n%q", width, slow, quiet)
		}
	}
	m.Pace = nil
	if view := ansi.Strip(ViewSize(m, 120, 30, paceNow)); strings.Contains(view, "↑") {
		t.Fatalf("no baseline, no mark:\n%s", view)
	}
}
