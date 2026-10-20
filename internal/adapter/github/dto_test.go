package github

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

// A job name can carry attacker text (a fork PR's title through
// `name: ${{ github.event.pull_request.title }}`): no control or
// bidi-override rune survives the mapping to reach the terminal.
func TestMapJobKeepsControlRunesOffTheScreen(t *testing.T) {
	hostile := "bdd \x1b]52;c;cGF5bG9hZA==\x07\x1b[2J\u202eevil\nnext\ttab"
	job, err := mapJob(jobDTO{
		Name:   hostile,
		Status: model.StatusInProgress,
		Labels: []string{"ubuntu\x1b[31m"},
		Steps:  []stepDTO{{Name: hostile, Status: model.StatusInProgress}},
	})
	if err != nil {
		t.Fatalf("mapJob: %v", err)
	}
	run, err := mapRun(runDTO{Name: hostile, DisplayTitle: hostile, Status: model.StatusInProgress})
	if err != nil {
		t.Fatalf("mapRun: %v", err)
	}
	for _, got := range []string{job.Name, job.Steps[0].Name, job.Labels[0], run.Name, run.DisplayTitle} {
		if strings.ContainsAny(got, "\x1b\x07\n\t\u202e") {
			t.Fatalf("control rune survived: %q", got)
		}
	}
	if want := "bdd ]52;c;cGF5bG9hZA==[2Jevil next tab"; job.Name != want {
		t.Fatalf("name = %q, want %q", job.Name, want)
	}
	if plain := "Run tests (ubuntu, 1.26) — ✓ 测试"; displayText(plain) != plain {
		t.Fatalf("a plain name changed: %q", displayText(plain))
	}
}
