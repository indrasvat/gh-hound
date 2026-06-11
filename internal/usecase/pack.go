package usecase

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

// A pack is the group of runs one event kicked off: same head_sha,
// same event, same repo. A push typically looses several workflows at
// once; the pack board watches them together instead of one at a time.

// DefaultPackMax caps how many runs one board tracks (config
// watch_group_max).
const DefaultPackMax = 10

// PackForRun selects the anchor run's pack from already-listed rows:
// every run sharing the anchor's head_sha AND event, deduped by ID,
// sorted by run ID ascending so board rows never reshuffle between
// repaints. The anchor always survives the cap. An anchor without a
// head_sha cannot be grouped and stays a pack of one.
func PackForRun(runs []model.Run, anchor model.Run, max int) []model.Run {
	if max <= 0 {
		max = DefaultPackMax
	}
	if strings.TrimSpace(anchor.HeadSHA) == "" {
		return []model.Run{anchor}
	}
	members := []model.Run{anchor}
	seen := map[int64]bool{anchor.ID: true}
	for _, run := range runs {
		if run.ID == 0 || seen[run.ID] {
			continue
		}
		if run.HeadSHA != anchor.HeadSHA || run.Event != anchor.Event {
			continue
		}
		seen[run.ID] = true
		members = append(members, run)
	}
	sortPack(members)
	if len(members) > max {
		members = capPack(members, anchor.ID, max)
	}
	return members
}

// capPack trims to max members while guaranteeing the anchor keeps its
// seat: the user asked to watch THAT run.
func capPack(members []model.Run, anchorID int64, max int) []model.Run {
	capped := members[:max:max]
	for _, run := range capped {
		if run.ID == anchorID {
			return capped
		}
	}
	capped = slices.Clone(capped[:max-1])
	for _, run := range members[max:] {
		if run.ID == anchorID {
			capped = append(capped, run)
			break
		}
	}
	sortPack(capped)
	return capped
}

func sortPack(members []model.Run) {
	slices.SortStableFunc(members, func(a, b model.Run) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
}

// PackState is one board's polling state: the scent (sha + event), the
// member rows, and the watch cadence.
type PackState struct {
	Repo    string
	HeadSHA string
	Event   string
	Branch  string
	// Max caps pack membership (config watch_group_max).
	Max      int
	Runs     []model.Run
	Terminal bool
	NextPoll time.Duration
}

// RunLister is the ONLY capability the pack watch is allowed: one runs
// list per tick covers every member. Narrower than GitHub on purpose —
// the type system enforces the board's no-job-fetch poll budget.
type RunLister interface {
	ListRuns(context.Context, RunFilter) ([]model.Run, error)
}

// PackWatchService polls a pack with exactly one head_sha-filtered
// list call per tick (PRD §9.5 budget × 1.5 ceiling), merging member
// updates and admitting same-scent joiners up to the cap.
type PackWatchService struct {
	Runs    RunLister
	MinPoll time.Duration
	MaxPoll time.Duration
}

// packListPerPage is sized for one page: a single event rarely
// triggers more than a handful of workflows, and the cap is 10ish.
const packListPerPage = 50

func (s PackWatchService) Tick(ctx context.Context, state PackState) (PackState, error) {
	minPoll := s.MinPoll
	if minPoll <= 0 {
		minPoll = 2 * time.Second
	}
	maxPoll := s.MaxPoll
	if maxPoll <= 0 {
		maxPoll = 30 * time.Second
	}
	if state.Max <= 0 {
		state.Max = DefaultPackMax
	}

	listed, err := s.Runs.ListRuns(ctx, RunFilter{
		Repo:    state.Repo,
		Branch:  state.Branch,
		HeadSHA: state.HeadSHA,
		PerPage: packListPerPage,
	})
	if err != nil {
		return PackState{}, err
	}

	byID := make(map[int64]model.Run, len(listed))
	for _, run := range listed {
		byID[run.ID] = run
	}
	members := make([]model.Run, 0, len(state.Runs))
	seen := make(map[int64]bool, len(state.Runs))
	for _, member := range state.Runs {
		if fresh, ok := byID[member.ID]; ok {
			member = fresh
		}
		members = append(members, member)
		seen[member.ID] = true
	}
	// New runs joining the pack mid-watch (queued workflows landing
	// late) fold in, capped — the budget math must not grow unbounded.
	for _, run := range listed {
		if seen[run.ID] || run.ID == 0 {
			continue
		}
		if run.HeadSHA != state.HeadSHA || run.Event != state.Event {
			continue
		}
		if len(members) >= state.Max {
			break
		}
		seen[run.ID] = true
		members = append(members, run)
	}
	sortPack(members)
	state.Runs = members

	state.Terminal = len(members) > 0
	for _, run := range members {
		if run.Status != model.StatusCompleted {
			state.Terminal = false
			break
		}
	}
	if state.Terminal {
		state.NextPoll = maxPoll
	} else {
		state.NextPoll = minPoll
	}
	return state, nil
}

// PackSummary aggregates the board header: still running, home
// (passed), lost (everything else that finished).
type PackSummary struct {
	Running int
	Home    int
	Lost    int
}

// String renders the header counts in the hound voice:
// `3 running · 1 home · 0 lost`.
func (s PackSummary) String() string {
	return fmt.Sprintf("%d running · %d home · %d lost", s.Running, s.Home, s.Lost)
}

// Settled reports whether the whole pack has finished.
func (s PackSummary) Settled() bool {
	return s.Running == 0 && s.Home+s.Lost > 0
}

func SummarizePack(runs []model.Run) PackSummary {
	var summary PackSummary
	for _, run := range runs {
		switch {
		case run.Status != model.StatusCompleted:
			summary.Running++
		case packRunHome(run):
			summary.Home++
		default:
			summary.Lost++
		}
	}
	return summary
}

func packRunHome(run model.Run) bool {
	switch run.Conclusion {
	case model.ConclusionSuccess, model.ConclusionSkipped, model.ConclusionNeutral:
		return true
	default:
		return false
	}
}

// WorstRunIndex picks the run the board's follow mode should track:
// the first lost run, else the first still-running one, else the top
// row. Ties break by board order so the cursor never jitters.
func WorstRunIndex(runs []model.Run) int {
	for index, run := range runs {
		if run.Status == model.StatusCompleted && !packRunHome(run) {
			return index
		}
	}
	for index, run := range runs {
		if run.Status != model.StatusCompleted {
			return index
		}
	}
	return 0
}
