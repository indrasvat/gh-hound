package watch

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
)

func TestModelFollowCancelDebugAndDetach(t *testing.T) {
	m := NewModel(State{
		Repo:   "indrasvat/gh-hound",
		Branch: "main",
		Run:    model.Run{ID: 570, Name: "CI", RunNumber: 570, Status: model.StatusInProgress},
		Lines:  logs.Parse("041 17:43:02Z go test ./...\n042 ok internal/api 0.214s\n").Lines,
	})
	if !m.Follow {
		t.Fatalf("follow should default on")
	}
	m = m.Update(KeyMsg{Key: "f"})
	if m.Follow {
		t.Fatalf("follow should toggle off")
	}
	m = m.Update(KeyMsg{Key: "d"})
	if !m.Debug {
		t.Fatalf("debug should toggle on")
	}
	m = m.Update(KeyMsg{Key: "x"})
	if m.Intent.Kind != IntentCancel {
		t.Fatalf("cancel intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.Intent.Kind != IntentDetach {
		t.Fatalf("detach intent = %#v", m.Intent)
	}
}

func TestViewMatchesStreamingMock(t *testing.T) {
	m := NewModel(State{
		Repo:    "indrasvat/gh-hound",
		Branch:  "main",
		Elapsed: "0m48s",
		Run:     model.Run{ID: 570, Name: "CI", RunNumber: 570, Status: model.StatusInProgress},
		Lines: logs.Parse(strings.Join([]string{
			"17:43:02.781Z go test ./... -race -count=1",
			"ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
			"ok    github.com/indrasvat/gh-hound/internal/render 0.331s",
			"=== RUN   TestLexIdent",
			"=== RUN   TestLexIdent/basic",
			"--- PASS: TestLexIdent/basic (0.00s)",
		}, "\n")).Lines,
	})
	view := View(m, 80)
	plain := ansi.Strip(view)
	for _, want := range []string{
		"watch · CI #570 · main",
		"streaming ⠹ 0m48s follow ●",
		"▾ completed job logs",
		"001 17:43:02.781Z go test ./... -race -count=1",
		"006 --- PASS: TestLexIdent/basic (0.00s)█",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("watch view missing %q\n%s", want, view)
		}
	}
	for _, banned := range []string{"041 041", "incoming ▾ active step tail"} {
		if strings.Contains(plain, banned) {
			t.Fatalf("watch view should not contain placeholder/duplicate text %q\n%s", banned, view)
		}
	}
	assertWidth(t, view, 80)
}

func TestViewMatchesMockStreamingColors(t *testing.T) {
	m := NewModel(State{
		Repo:    "indrasvat/gh-hound",
		Branch:  "main",
		Elapsed: "0m48s",
		Run:     model.Run{ID: 570, Name: "CI", RunNumber: 570, Status: model.StatusInProgress},
		Lines: logs.Parse(strings.Join([]string{
			"17:43:02.781Z go test ./... -race -count=1",
			"ok    github.com/indrasvat/gh-hound/internal/api 0.214s",
			"--- PASS: TestLexIdent/basic (0.00s)",
		}, "\n")).Lines,
	})
	view := View(m, 80)
	for _, want := range []string{
		"\x1b[38;2;107;112;96m17:43:02.781Z\x1b[0m",
		"\x1b[38;2;110;156;181mgo test ./... -race -count=1\x1b[0m",
		"\x1b[38;2;79;211;122mok\x1b[0m",
		"\x1b[38;2;224;163;62m--- PASS: TestLexIdent/basic (0.00s)\x1b[0m\x1b[38;2;79;211;122m█\x1b[0m",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("watch view missing styled token %q\n%s", want, view)
		}
	}
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line too wide (%d): %q\n%s", got, line, view)
		}
	}
}

// A finished run is no longer streaming: the header names how it ended
// and x has nothing to call off.
func TestFinishedRunShowsItsConclusion(t *testing.T) {
	for _, tc := range []struct {
		conclusion model.Conclusion
		want       string
	}{
		{model.ConclusionSuccess, "✔ success 4m45s follow ●"},
		{model.ConclusionFailure, "✗ failure 4m45s follow ●"},
		{model.ConclusionCancelled, "⊘ cancelled 4m45s follow ●"},
		{model.ConclusionNone, "◇ completed 4m45s follow ●"},
	} {
		m := NewModel(State{
			Branch: "main", Elapsed: "4m45s",
			Run: model.Run{ID: 164, Name: "CI", RunNumber: 164, Status: model.StatusCompleted, Conclusion: tc.conclusion},
		})
		plain := ansi.Strip(View(m, 80))
		if !strings.Contains(plain, "watch · CI #164 · main "+tc.want) || strings.Contains(plain, "streaming") {
			t.Fatalf("%s header:\n%s", tc.conclusion, plain)
		}
		if strings.Contains(plain, "waiting for completed job logs") || !strings.Contains(plain, "no job logs to show") {
			t.Fatalf("%s run with no lines waits for logs:\n%s", tc.conclusion, plain)
		}
		if m.Update(KeyMsg{Key: "x"}).Intent.Kind != IntentNone {
			t.Fatalf("x on a %s run asks to cancel it", tc.conclusion)
		}
	}
}

// Jobs shown without a log are named under what happened to them, the
// status first so a cut line never loses it.
func TestViewNamesJobsWithoutALog(t *testing.T) {
	long := "integration-tests (ubuntu-latest, go 1.24, postgres 16, redis 7, kafka 3.7)"
	m := NewModel(State{
		Branch: "main",
		Run:    model.Run{ID: 148, Name: "Release", RunNumber: 148, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
		NoLog:  []NoLog{{Job: long, Expired: true}, {Job: "publish\nstep", Expired: true}, {Job: "notify"}, {Job: " "}},
	})
	for _, width := range []int{80, 120} {
		view := View(m, width)
		plain := ansi.Strip(view)
		for _, want := range []string{"log expired · integration-tests", "log not available yet · notify, job"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("view at %d missing %q:\n%s", width, want, plain)
			}
		}
		if width == 120 && !strings.Contains(plain, "kafka 3.7), publish step") {
			t.Fatalf("names are not joined on one line at 120:\n%s", plain)
		}
		assertWidth(t, view, width)
	}
}

// A finished run has no active step: no running cursor on its last
// line, and the no-log note stands alone when nothing else shows.
func TestFinishedRunHasNoActiveCursor(t *testing.T) {
	done := model.Run{ID: 164, Name: "CI", RunNumber: 164, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}
	m := NewModel(State{Branch: "main", Run: done, Lines: logs.Parse("17:43:02Z go test ./...\nok internal/api 0.214s\n").Lines})
	if plain := ansi.Strip(View(m, 80)); strings.Contains(plain, "█") {
		t.Fatalf("finished run draws the running cursor:\n%s", plain)
	}
	m = NewModel(State{Branch: "main", Run: done, NoLog: []NoLog{{Job: "build", Expired: true}}})
	if plain := ansi.Strip(View(m, 80)); strings.Contains(plain, "no job logs to show") {
		t.Fatalf("the note is repeated:\n%s", plain)
	}
}

// GitHub serves a job's log only once it finishes: a run whose one job
// is still running shows that job's current step and how long it has
// run, not a blank "waiting" screen (the live-run report).
func TestViewShowsRunningJobProgress(t *testing.T) {
	now := time.Date(2026, 10, 20, 11, 38, 0, 0, time.UTC)
	running := model.Job{ID: 1, Name: "bdd", Status: model.StatusInProgress, StartedAt: now.Add(-15 * time.Minute), Steps: []model.Step{
		{Number: 28, Name: "Prune Docker images", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess},
		{Number: 29, Name: "Run tests", Status: model.StatusInProgress, StartedAt: now.Add(-12*time.Minute - 40*time.Second)},
		{Number: 30, Name: "Upload UI artifacts", Status: model.StatusQueued},
	}}
	queued := model.Job{ID: 2, Name: "report", Status: model.StatusQueued}
	done := model.Job{ID: 3, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}
	m := NewModel(State{
		Branch: "pr-3899",
		Run:    model.Run{ID: 3902, Name: "Edge BDD", RunNumber: 3902, Status: model.StatusInProgress},
		Jobs:   []model.Job{done, running, queued},
		Now:    now,
	})
	for _, width := range []int{80, 120} {
		view := View(m, width)
		plain := ansi.Strip(view)
		for _, want := range []string{"bdd › 29 Run tests 12m40s", "report queued", "GitHub serves a job's log once it finishes"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("view at %d missing %q:\n%s", width, want, plain)
			}
		}
		if strings.Contains(plain, "lint") || strings.Contains(plain, "waiting for completed job logs") {
			t.Fatalf("view at %d shows a finished job or the blank wait:\n%s", width, plain)
		}
		assertWidth(t, view, width)
	}

	// A finished run shows no progress block.
	m.State.Run.Status, m.State.Run.Conclusion = model.StatusCompleted, model.ConclusionSuccess
	if plain := ansi.Strip(View(m, 80)); strings.Contains(plain, "Run tests") {
		t.Fatalf("finished run shows step progress:\n%s", plain)
	}
}

// The progress block is bounded so the log tail keeps its rows: a wide
// matrix reads as its running jobs first and a count, and a name with a
// newline or tab stays on one measured line.
func TestProgressBlockIsBounded(t *testing.T) {
	now := time.Date(2026, 10, 20, 11, 38, 0, 0, time.UTC)
	var jobs []model.Job
	jobs = append(jobs, model.Job{ID: 1, Name: "report", Status: model.StatusQueued})
	for i := range 50 {
		jobs = append(jobs, model.Job{ID: int64(10 + i), Name: fmt.Sprintf("shard %d", i), Status: model.StatusInProgress, Steps: []model.Step{
			{Number: 3, Name: "Run\ttests\nnow", Status: model.StatusInProgress, StartedAt: now.Add(-time.Minute)},
		}})
	}
	var tail []string
	for i := range 6 {
		tail = append(tail, fmt.Sprintf("TAILMARK %d", i))
	}
	m := NewModel(State{
		Run:   model.Run{ID: 1, Name: "E2E", RunNumber: 1, Status: model.StatusInProgress},
		Jobs:  jobs,
		Now:   now,
		Lines: logs.Parse(strings.Join(tail, "\n")).Lines,
	})
	view := View(m, 80)
	plain := ansi.Strip(view)
	for _, want := range []string{"shard 0 › 3 Run tests now 1m00s", "shard 1 ›", "+49 more jobs not finished", "TAILMARK 5"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "report") || strings.Contains(plain, "shard 2 ›") {
		t.Fatalf("block not bounded, or a queued job before a running one:\n%s", plain)
	}
	if lines := strings.Count(view, "\n") + 1; lines > 13 {
		t.Fatalf("view is %d lines, want at most 13:\n%s", lines, plain)
	}
	assertWidth(t, view, 80)
}

// A name that reached the screen unmapped (another adapter) still
// carries no control rune to the terminal.
func TestProgressDropsControlRunes(t *testing.T) {
	m := NewModel(State{
		Run:  model.Run{ID: 1, Name: "E2E", RunNumber: 1, Status: model.StatusInProgress},
		Jobs: []model.Job{{ID: 1, Name: "bdd\x1b]52;c;eA==\x07", Status: model.StatusInProgress, Steps: []model.Step{{Number: 1, Name: "go\x1b[2J", Status: model.StatusInProgress}}}},
	})
	if view := View(m, 80); strings.ContainsAny(ansi.Strip(view), "\x1b\x07") {
		t.Fatalf("control rune reached the view: %q", ansi.Strip(view))
	}
}
