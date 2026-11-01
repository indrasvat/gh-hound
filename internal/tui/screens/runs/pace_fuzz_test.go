package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// FuzzPaceRows: whatever the runs, durations, baselines and width, no
// row overflows the frame, the Actor column starts in the same cell on
// every row as in the header, and a Pace note is never cut.
func FuzzPaceRows(f *testing.F) {
	f.Add([]byte{0, 10, 1, 200, 2, 255, 3, 4}, uint8(80))
	f.Add([]byte{1, 1, 1, 1, 1, 1}, uint8(120))
	f.Add([]byte{5, 250, 4, 9, 0, 0, 1, 77}, uint8(40))
	f.Fuzz(func(t *testing.T, raw []byte, width uint8) {
		w := int(width)%180 + 46 // the narrowest body a 48-column frame (minFrameWidth) holds
		now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
		var rows []model.Run
		pace := map[int64]usecase.PaceBaseline{}
		names := []string{"CI", "Release · a very long workflow name that keeps going", "", "Docs"}
		for i := 0; i+1 < len(raw) && len(rows) < 12; i += 2 {
			kind, mag := raw[i], raw[i+1]
			took := time.Duration(int(mag)*int(mag)) * time.Second // up to ~18h
			run := model.Run{
				ID: int64(len(rows) + 1), RunNumber: 100 + len(rows), Name: names[int(kind)%len(names)],
				Actor: "indrasvat", Event: "push", HeadBranch: "main", RunAttempt: 1,
				Status: model.StatusCompleted, Conclusion: model.ConclusionSuccess,
				RunStartedAt: now.Add(-took - time.Minute), UpdatedAt: now.Add(-time.Minute),
			}
			switch kind % 5 {
			case 1:
				run.Status, run.Conclusion = model.StatusInProgress, model.ConclusionNone
				run.RunStartedAt = now.Add(-took)
			case 2:
				run.Conclusion = model.ConclusionFailure
			case 3:
				run.Status, run.Conclusion, run.RunStartedAt = model.StatusQueued, model.ConclusionNone, time.Time{}
			case 4:
				run.RunAttempt = 2
			}
			if kind&0x80 == 0 {
				b := usecase.PaceBaseline{}
				for j := range 6 {
					b.Samples = append(b.Samples, usecase.PaceSample{RunID: int64(1000 + j), Duration: time.Duration(int(kind)+j*7+1) * time.Second})
				}
				pace[run.ID] = b
			}
			rows = append(rows, run)
		}
		if len(rows) == 0 {
			return
		}
		// The all-green home, with every row green: nothing overflows.
		green := make([]model.Run, len(rows))
		for i, run := range rows {
			run.Status, run.Conclusion = model.StatusCompleted, model.ConclusionSuccess
			green[i] = run
		}
		home := NewModel(usecase.LaunchContext{Repo: "o/r", State: usecase.LaunchStateRuns, Runs: green})
		home.Pace = pace
		for line := range strings.SplitSeq(ansi.Strip(ViewSize(home, w, 40, now)), "\n") {
			if ansi.StringWidth(line) > w {
				t.Fatalf("width %d: all-green row overflows (%d): %q", w, ansi.StringWidth(line), line)
			}
		}

		rows[0].Conclusion = model.ConclusionFailure // keep the standard table
		rows[0].Status = model.StatusCompleted
		m := NewModel(usecase.LaunchContext{Repo: "o/r", State: usecase.LaunchStateRuns, Runs: rows})
		m.Pace = pace
		view := ansi.Strip(ViewSize(m, w, 40, now))
		lines := strings.Split(view, "\n")
		header := lines[0]
		before, _, ok := strings.Cut(header, "Actor")
		for _, line := range lines[1:] {
			if ansi.StringWidth(line) > w {
				t.Fatalf("width %d: row overflows (%d): %q", w, ansi.StringWidth(line), line)
			}
		}
		if !ok {
			return
		}
		col := len([]rune(before))
		for i, run := range rows {
			line := []rune(lines[1+i])
			if len(line) <= col || !strings.HasPrefix(string(line[col:]), "@indr") {
				t.Fatalf("width %d: Actor column moved on row %d: %q", w, i, string(line))
			}
			mark := usecase.PaceMarkFor(run, m.Pace[run.ID], now)
			c := columnsFor(w)
			room := c.duration > 0 || c.label-ansi.StringWidth(mark.Text)-1 >= minNoteLabel
			if mark.Note() && room && !strings.Contains(string(line), mark.Text) {
				t.Fatalf("width %d: Pace note %q cut from %q", w, mark.Text, string(line))
			}
		}
	})
}
