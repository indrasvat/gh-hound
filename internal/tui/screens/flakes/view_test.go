package flakes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

// The clean verdict ("fresh scent") is unreachable in the fake lens —
// the flaky scenario always wobbles — so its view branch is pinned
// here directly (QA round 17: the clean rendering had no coverage).
func TestCleanVerdictRendersFreshScent(t *testing.T) {
	m := NewModel(usecase.FlakeReport{
		Status:      usecase.FlakeStatusClean,
		Workflow:    "ci.yml",
		RunsScanned: 6,
		Verdict:     "fresh scent — worth chasing: no flake history in the last 6 runs.",
	})
	view := ansi.Strip(View(m, 100))
	for _, want := range []string{"fresh scent", "6 runs sniffed on ci.yml · nothing wobbled."} {
		if !strings.Contains(view, want) {
			t.Fatalf("clean flakes view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "wobbled 6") || strings.Contains(view, "squirrel") {
		t.Fatalf("clean view leaked a flaky-verdict phrase:\n%s", view)
	}
}

// The thin-trail copy must agree in number with the sample — "only 1
// completed run", never "1 runs" (QA round 17 plural slip).
func TestInsufficientVerdictMatchesSampleNumber(t *testing.T) {
	one := ansi.Strip(View(NewModel(usecase.FlakeReport{
		Status:      usecase.FlakeStatusInsufficient,
		Workflow:    "ci.yml",
		RunsScanned: 1,
		SampleSize:  1,
	}), 100))
	if !strings.Contains(one, "only 1 completed run on") {
		t.Fatalf("singular sample must read 'run', not 'runs':\n%s", one)
	}
	if strings.Contains(one, "1 completed runs") {
		t.Fatalf("singular sample rendered a plural noun:\n%s", one)
	}

	many := ansi.Strip(View(NewModel(usecase.FlakeReport{
		Status:      usecase.FlakeStatusInsufficient,
		Workflow:    "ci.yml",
		RunsScanned: 2,
		SampleSize:  2,
	}), 100))
	if !strings.Contains(many, "only 2 completed runs on") {
		t.Fatalf("plural sample must read 'runs':\n%s", many)
	}
}
