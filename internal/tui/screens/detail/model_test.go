package detail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
)

func TestModelPreselectsFailureFocusAndIntents(t *testing.T) {
	m := NewModel(run(), jobs()).WithRepo("indrasvat/gh-hound")
	if m.SelectedJob != 0 || m.SelectedStep != 5 || m.Focus != FocusSteps {
		t.Fatalf("initial model = %#v", m)
	}

	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusJobs {
		t.Fatalf("focus after tab = %s", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if m.SelectedJob != 1 {
		t.Fatalf("selected job = %d", m.SelectedJob)
	}
	m = m.Update(KeyMsg{Key: "n"})
	if m.SelectedJob != 0 || m.SelectedStep != 5 || m.Focus != FocusSteps {
		t.Fatalf("n did not jump to failed step: %#v", m)
	}
	m = m.Update(KeyMsg{Key: "l"})
	if m.Intent.Kind != IntentLog || m.Intent.JobID != 100 {
		t.Fatalf("log intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "J"})
	if m.Intent.Kind != IntentNextRun {
		t.Fatalf("J intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "K"})
	if m.Intent.Kind != IntentPreviousRun {
		t.Fatalf("K intent = %#v", m.Intent)
	}
}

func TestViewMatchesMasterDetailAndNarrowCollapse(t *testing.T) {
	m := NewModel(run(), jobs()).WithRepo("indrasvat/gh-hound")
	view := View(m, 120)
	plain := ansi.Strip(view)
	for _, want := range []string{
		"indrasvat/gh-hound › CI #571 › fix/parser · @a1b2c3d",
		"Jobs",
		"build [failure] [ubuntu-latest]",
		"2m14s",
		"▌ ✗ 6  go test ./...",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("detail view missing %q\n%s", want, view)
		}
	}
	assertWidth(t, view, 120)

	narrow := View(m, 80)
	plainNarrow := ansi.Strip(narrow)
	for _, want := range []string{"build [failure]", "▌ ✗ 6  go test ./..."} {
		if !strings.Contains(plainNarrow, want) {
			t.Fatalf("narrow view missing %q\n%s", want, narrow)
		}
	}
	if strings.Contains(plainNarrow, "Jobs |") {
		t.Fatalf("narrow view should collapse the jobs pane:\n%s", narrow)
	}
	assertWidth(t, narrow, 80)
}

func TestViewMatchesMockPaneRowsAndFailHighlight(t *testing.T) {
	view := View(NewModel(run(), jobs()).WithRepo("indrasvat/gh-hound"), 120)
	plain := ansi.Strip(view)
	for _, banned := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(plain, banned) {
			t.Fatalf("detail view should use row panes, not ASCII boxes %q\n%s", banned, view)
		}
	}
	if !strings.Contains(view, "\x1b[48;2;36;39;30m") {
		t.Fatalf("selected job should use surface-2 background fill\n%s", view)
	}
	if !strings.Contains(view, "\x1b[48;2;40;19;18m") {
		t.Fatalf("failed step should use fail-tinted background fill\n%s", view)
	}
	if !strings.Contains(view, "\x1b[38;2;226;86;75m▌") {
		t.Fatalf("failed step should have fail-colored left bar\n%s", view)
	}
	if strings.Count(plain, "n jump to failure") != 1 {
		t.Fatalf("detail hint should render once in the steps pane\n%s", view)
	}
}

func TestViewDoesNotInventMissingGitHubMetadata(t *testing.T) {
	m := NewModel(model.Run{
		ID:         9001,
		RunNumber:  44,
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
	}, []model.Job{{
		ID:         7001,
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
	}})
	view := ansi.Strip(View(m, 100))
	for _, banned := range []string{"repository", "branch", "@sha", "unknown", "runner"} {
		if strings.Contains(view, banned) {
			t.Fatalf("detail view invented fallback %q\n%s", banned, view)
		}
	}
	for _, want := range []string{"#44", "job 7001"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail view missing real identifier %q\n%s", want, view)
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

func run() model.Run {
	return model.Run{
		ID:         571,
		Name:       "CI",
		Status:     model.StatusCompleted,
		Conclusion: model.ConclusionFailure,
		HeadBranch: "fix/parser",
		HeadSHA:    "a1b2c3d",
		RunNumber:  571,
	}
}

func jobs() []model.Job {
	start := time.Date(2026, 6, 7, 17, 42, 0, 0, time.UTC)
	return []model.Job{
		{
			ID:          100,
			Name:        "build",
			Status:      model.StatusCompleted,
			Conclusion:  model.ConclusionFailure,
			Labels:      []string{"ubuntu-latest"},
			StartedAt:   start,
			CompletedAt: start.Add(134 * time.Second),
			Steps: []model.Step{
				step(1, "Set up job", model.ConclusionSuccess, 400*time.Millisecond),
				step(2, "Checkout", model.ConclusionSuccess, 1200*time.Millisecond),
				step(3, "Setup Go 1.26", model.ConclusionSuccess, 3100*time.Millisecond),
				step(4, "Cache modules", model.ConclusionSuccess, 2*time.Second),
				step(5, "go build ./...", model.ConclusionSuccess, 18700*time.Millisecond),
				step(6, "go test ./...", model.ConclusionFailure, 41300*time.Millisecond),
			},
		},
		{ID: 101, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: start, CompletedAt: start.Add(31 * time.Second)},
	}
}

func step(number int, name string, conclusion model.Conclusion, duration time.Duration) model.Step {
	start := time.Date(2026, 6, 7, 17, 43, 0, 0, time.UTC)
	return model.Step{
		Number:      number,
		Name:        name,
		Status:      model.StatusCompleted,
		Conclusion:  conclusion,
		StartedAt:   start,
		CompletedAt: start.Add(duration),
	}
}

func artifactFixtures() []model.Artifact {
	return []model.Artifact{
		{ID: 901, Name: "coverage", SizeInBytes: 1262848},
		{ID: 902, Name: "old-report", SizeInBytes: 52480, Expired: true},
	}
}

func TestArtifactsKeyFocusesArtifactsPane(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m = m.Update(KeyMsg{Key: "a"})
	if m.Focus != FocusArtifacts {
		t.Fatalf("focus = %q, want artifacts", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if m.SelectedArtifact != 1 {
		t.Fatalf("selected artifact = %d, want 1", m.SelectedArtifact)
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentDownloadArtifact || m.Intent.ArtifactID != 902 {
		t.Fatalf("intent = %#v, want download for 902", m.Intent)
	}
}

func TestArtifactsKeyIsNoOpWithoutArtifacts(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil)
	m = m.Update(KeyMsg{Key: "a"})
	if m.Focus == FocusArtifacts {
		t.Fatal("artifacts focus must be unreachable when no artifacts exist")
	}
}

func TestTabCyclesThroughArtifactsWhenPresent(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m.Focus = FocusJobs
	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusSteps {
		t.Fatalf("first tab = %q, want steps", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusArtifacts {
		t.Fatalf("second tab = %q, want artifacts", m.Focus)
	}
	m = m.Update(KeyMsg{Key: "tab"})
	if m.Focus != FocusJobs {
		t.Fatalf("third tab = %q, want jobs", m.Focus)
	}
}

func TestDownloadKeyTriggersFromAnyFocusWithSelection(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m = m.Update(KeyMsg{Key: "d"})
	if m.Intent.Kind != IntentDownloadArtifact || m.Intent.ArtifactID != 901 {
		t.Fatalf("intent = %#v, want download for selected artifact 901", m.Intent)
	}
}

func TestWithArtifactsClampsSelection(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifactFixtures())
	m.SelectedArtifact = 5
	m = m.WithArtifacts(artifactFixtures()[:1])
	if m.SelectedArtifact != 0 {
		t.Fatalf("selection not clamped: %d", m.SelectedArtifact)
	}
}

func TestViewSizeKeepsArtifactsVisibleWithManySteps(t *testing.T) {
	steps := make([]model.Step, 0, 14)
	for i := 1; i <= 14; i++ {
		steps = append(steps, model.Step{Name: fmt.Sprintf("step %d", i), Number: i, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess})
	}
	jobs := []model.Job{{ID: 1, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Steps: steps}}
	m := NewModel(model.Run{ID: 1}, jobs).WithArtifacts(artifactFixtures())

	view := ansi.Strip(ViewSize(m, 80, 20))
	lines := strings.Split(view, "\n")
	if len(lines) > 20 {
		t.Fatalf("ViewSize must fit the height budget: %d lines > 20\n%s", len(lines), view)
	}
	if !strings.Contains(view, "Artifacts (2)") || !strings.Contains(view, "coverage") {
		t.Fatalf("artifacts must stay visible when steps overflow:\n%s", view)
	}
	if !strings.Contains(view, "more") {
		t.Fatalf("step overflow must be indicated:\n%s", view)
	}
	if !strings.Contains(view, "a artifacts") {
		t.Fatalf("hint line must survive the height budget:\n%s", view)
	}
}

func TestViewSizeKeepsSelectedStepVisible(t *testing.T) {
	steps := make([]model.Step, 0, 20)
	for i := 1; i <= 20; i++ {
		steps = append(steps, model.Step{Name: fmt.Sprintf("step %d", i), Number: i, Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess})
	}
	jobs := []model.Job{{ID: 1, Name: "build", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, Steps: steps}}
	m := NewModel(model.Run{ID: 1}, jobs).WithArtifacts(artifactFixtures())
	m.Focus = FocusSteps
	m.SelectedStep = 17

	view := ansi.Strip(ViewSize(m, 80, 20))
	if !strings.Contains(view, "step 18") {
		t.Fatalf("selected step must stay in the window:\n%s", view)
	}
}

func TestContextualOpenAndCopyOnDownloadedArtifact(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts([]model.Artifact{{ID: 901, Name: "coverage"}})
	m.Focus = FocusArtifacts

	// Not downloaded: o/y keep their browser/copy-URL meanings.
	if got := m.Update(KeyMsg{Key: "o"}).Intent.Kind; got != IntentBrowser {
		t.Fatalf("o on a non-downloaded artifact = %q, want browser", got)
	}
	if got := m.Update(KeyMsg{Key: "y"}).Intent.Kind; got != IntentCopyURL {
		t.Fatalf("y on a non-downloaded artifact = %q, want copy URL", got)
	}

	m = m.WithDownload(901, DownloadStatus{State: DownloadStateDone, Path: "/tmp/coverage"})
	next := m.Update(KeyMsg{Key: "o"})
	if next.Intent.Kind != IntentOpenArtifactDir || next.Intent.ArtifactID != 901 {
		t.Fatalf("o on a downloaded artifact = %+v, want open_artifact_dir for 901", next.Intent)
	}
	next = m.Update(KeyMsg{Key: "y"})
	if next.Intent.Kind != IntentCopyArtifactPath || next.Intent.ArtifactID != 901 {
		t.Fatalf("y on a downloaded artifact = %+v, want copy_artifact_path for 901", next.Intent)
	}

	// Outside the artifacts pane the download must not hijack o/y.
	m.Focus = FocusJobs
	if got := m.Update(KeyMsg{Key: "o"}).Intent.Kind; got != IntentBrowser {
		t.Fatalf("o outside the artifacts pane = %q, want browser", got)
	}
}

func TestWithDownloadKeepsValueSemantics(t *testing.T) {
	m := NewModel(model.Run{ID: 1}, nil).WithArtifacts([]model.Artifact{{ID: 901, Name: "coverage"}})
	frozen := m.WithDownload(901, DownloadStatus{State: DownloadStateDownloading})
	mutated := frozen.WithDownload(901, DownloadStatus{State: DownloadStateDone, Path: "/x"})
	if frozen.Download(901).State != DownloadStateDownloading {
		t.Fatal("WithDownload mutated the prior model value")
	}
	if mutated.Download(901).State != DownloadStateDone {
		t.Fatal("WithDownload failed to record the new status")
	}
	if mutated.DownloadedCount() != 1 {
		t.Fatalf("DownloadedCount = %d, want 1", mutated.DownloadedCount())
	}
}

// The height budget and the rendered artifacts block must agree in
// every selection/window/download state (codex r1 finding 5: the old
// count added an overflow line even when the window hid rows above,
// not below).
func TestArtifactsBlockLinesMatchesRenderedBlock(t *testing.T) {
	artifacts := make([]model.Artifact, 8)
	for i := range artifacts {
		artifacts[i] = model.Artifact{ID: int64(900 + i), Name: "artifact", SizeInBytes: 1024}
	}
	for _, count := range []int{1, 4, 5, 6, 8} {
		m := NewModel(model.Run{ID: 1}, nil).WithArtifacts(artifacts[:count])
		m.Focus = FocusArtifacts
		for selected := range count {
			m.SelectedArtifact = selected
			for _, downloaded := range []bool{false, true} {
				probe := m
				if downloaded {
					probe = m.WithDownload(artifacts[selected].ID, DownloadStatus{State: DownloadStateDone, Path: "/tmp/x"})
				}
				rendered := len(renderArtifactsBlock(probe, 80))
				budget := artifactsBlockLines(probe)
				if rendered != budget {
					t.Fatalf("count=%d selected=%d downloaded=%v: rendered %d lines, budget %d", count, selected, downloaded, rendered, budget)
				}
			}
		}
	}
}

// A running step reads its elapsed time, not "queued": "queued" is only
// for a step that has not started (the live-run report).
func TestViewReadsRunningStepsAsElapsed(t *testing.T) {
	now := time.Date(2026, 10, 20, 11, 32, 0, 0, time.UTC)
	job := model.Job{ID: 7, Name: "bdd", Status: model.StatusInProgress, StartedAt: now.Add(-20 * time.Minute), Steps: []model.Step{
		{Number: 1, Name: "Set up job", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess, StartedAt: now.Add(-20 * time.Minute), CompletedAt: now.Add(-20*time.Minute + 15*time.Second)},
		{Number: 9, Name: "Cleanup", Status: model.StatusCompleted, Conclusion: model.ConclusionSkipped},
		{Number: 29, Name: "Run tests", Status: model.StatusInProgress, StartedAt: now.Add(-12*time.Minute - 40*time.Second)},
		{Number: 30, Name: "Upload artifacts", Status: model.StatusQueued},
	}}
	m := NewModel(model.Run{ID: 3902, Name: "E2E", RunNumber: 3902, Status: model.StatusInProgress}, []model.Job{job})
	m.Now = now
	plain := ansi.Strip(View(m, 120))
	for _, want := range []string{"Run tests", "12m40s", "20m00s", "15.0s", "—"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view missing %q:\n%s", want, plain)
		}
	}
	for line := range strings.SplitSeq(plain, "\n") {
		if strings.Contains(line, "Run tests") && strings.Contains(line, "queued") {
			t.Fatalf("running step reads queued: %q", line)
		}
		if strings.Contains(line, "Upload artifacts") && !strings.Contains(line, "queued") {
			t.Fatalf("unstarted step does not read queued: %q", line)
		}
	}
}

// A fresher job listing keeps the cursor on the same job and step.
func TestWithJobsKeepsTheCursor(t *testing.T) {
	steps := []model.Step{{Number: 1}, {Number: 2}, {Number: 3}}
	m := NewModel(model.Run{ID: 1}, []model.Job{{ID: 10, Steps: steps}, {ID: 20, Steps: steps}})
	m.SelectedJob, m.SelectedStep = 1, 2
	m = m.WithJobs([]model.Job{{ID: 5}, {ID: 10, Steps: steps}, {ID: 20, Steps: steps}})
	if m.SelectedJob != 2 || m.SelectedStep != 2 {
		t.Fatalf("cursor = job %d step %d, want job 2 step 2", m.SelectedJob, m.SelectedStep)
	}
	m = m.WithJobs([]model.Job{{ID: 10, Steps: steps[:1]}})
	if m.SelectedJob != 0 || m.SelectedStep != 0 {
		t.Fatalf("cursor = job %d step %d, want clamped to 0/0", m.SelectedJob, m.SelectedStep)
	}
	// The selected job gone: the cursor lands on another job at its
	// first step, not the old step index.
	m = m.WithJobs([]model.Job{{ID: 10, Steps: steps}, {ID: 20, Steps: steps}})
	m.SelectedJob, m.SelectedStep = 1, 2
	m = m.WithJobs([]model.Job{{ID: 10, Steps: steps}, {ID: 30, Steps: steps}})
	if m.SelectedJob != 1 || m.SelectedStep != 0 {
		t.Fatalf("cursor = job %d step %d, want job 1 step 0", m.SelectedJob, m.SelectedStep)
	}
	m.SelectedJob = -1
	if m = m.WithJobs(m.Jobs); m.SelectedJob != 0 {
		t.Fatalf("a negative cursor = %d, want 0", m.SelectedJob)
	}
}

// The status decides, not the timestamp: GitHub stamps started_at on a
// queued job too, and a job waiting for a runner is not running.
func TestDurationReadsStatusFirst(t *testing.T) {
	now := time.Date(2026, 10, 20, 11, 32, 0, 0, time.UTC)
	start := now.Add(-3 * time.Minute)
	for _, tc := range []struct {
		status     model.Status
		start, end time.Time
		want       string
	}{
		{model.StatusQueued, start, time.Time{}, "queued"},
		{model.StatusPending, start, time.Time{}, "queued"},
		{model.StatusRequested, time.Time{}, time.Time{}, "queued"},
		{model.StatusWaiting, start, time.Time{}, "waiting"},
		{model.StatusInProgress, start, time.Time{}, "3m00s"},
		{model.StatusInProgress, time.Time{}, time.Time{}, "running"},
		{model.StatusInProgress, now.Add(time.Minute), time.Time{}, "0s"},
		{model.StatusInProgress, now.Add(-17 * time.Second), time.Time{}, "17s"},
		{model.StatusCompleted, start, now, "3m00s"},
		{model.StatusCompleted, time.Time{}, time.Time{}, "—"},
		{model.StatusCompleted, now, start, "—"},
	} {
		if got := duration(tc.status, tc.start, tc.end, now); got != tc.want {
			t.Errorf("duration(%s, %v, %v) = %q, want %q", tc.status, tc.start, tc.end, got, tc.want)
		}
	}
}

// A running job's steps open on the step it is on, which in a long
// list would otherwise sit below the window; a poll moves the cursor
// on with it unless the user moved it.
func TestRunningStepIsSelected(t *testing.T) {
	steps := func(running int) []model.Step {
		var out []model.Step
		for i := range 40 {
			status := model.StatusCompleted
			switch {
			case i == running:
				status = model.StatusInProgress
			case i > running:
				status = model.StatusQueued
			}
			out = append(out, model.Step{Number: i + 1, Name: fmt.Sprintf("step %d", i+1), Status: status})
		}
		return out
	}
	lint := model.Job{ID: 1, Name: "lint", Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess}
	build := func(running int) model.Job {
		return model.Job{ID: 2, Name: "build", Status: model.StatusInProgress, Steps: steps(running)}
	}
	m := NewModel(model.Run{ID: 1, Status: model.StatusInProgress}, []model.Job{lint, build(25)})
	if m.SelectedJob != 1 || m.SelectedStep != 25 {
		t.Fatalf("opened on job %d step %d, want the running job at step 25", m.SelectedJob, m.SelectedStep)
	}
	if plain := ansi.Strip(ViewSize(m, 120, 24)); !strings.Contains(plain, "step 26") {
		t.Fatalf("the running step is off the window:\n%s", plain)
	}
	m = m.WithJobs([]model.Job{lint, build(26)})
	if m.SelectedStep != 26 {
		t.Fatalf("after a poll step = %d, want it to follow to 26", m.SelectedStep)
	}
	m.Focus = FocusSteps
	m = m.Update(KeyMsg{Key: "k"})
	m = m.WithJobs([]model.Job{lint, build(27)})
	if m.SelectedStep != 25 {
		t.Fatalf("a moved cursor = %d, want it left at 25", m.SelectedStep)
	}
	m.Focus = FocusJobs
	m = m.Update(KeyMsg{Key: "k"})
	m = m.Update(KeyMsg{Key: "j"})
	if m.SelectedStep != 27 {
		t.Fatalf("re-selecting the running job: step = %d, want 27", m.SelectedStep)
	}
}

// A job GitHub has not handed a runner yet has no steps: the pane says
// so instead of showing two bare dividers.
func TestQueuedJobWithoutStepsSaysWhy(t *testing.T) {
	m := NewModel(model.Run{ID: 1, Status: model.StatusInProgress}, []model.Job{{ID: 1, Name: "report", Status: model.StatusQueued}})
	if plain := ansi.Strip(View(m, 120)); !strings.Contains(plain, "waiting for a runner · steps appear once it starts") {
		t.Fatalf("no hint for a job without steps:\n%s", plain)
	}
}
