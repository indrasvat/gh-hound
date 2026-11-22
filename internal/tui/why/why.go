// Package why words a red run's verdict for the screens: the runs
// list's short cell and the detail view's explanation.
package why

import "github.com/indrasvat/gh-hound/internal/model"

// Short is the runs-list word for a verdict: the Duration cell for a
// held or missed run, the label's detail for the other reasons.
func Short(v model.RunVerdict) string {
	switch v.Reason {
	case model.ReasonAwaitingApproval:
		return "approve"
	case model.ReasonApprovalMissed:
		return "missed"
	case model.ReasonBlockedByPolicy:
		return "blocked by policy"
	case model.ReasonCouldNotStart:
		return "could not start"
	case model.ReasonNoJobRan:
		return "no job ran"
	case model.ReasonRunnerLost:
		return "runner lost"
	case model.ReasonKilled:
		return "killed · exit 137"
	case model.ReasonDiskFull:
		return "disk full"
	}
	return ""
}

// InDuration reports a reason that replaces the Duration cell: a run
// that never started has no duration to show.
func InDuration(v model.RunVerdict) bool {
	return v.Reason == model.ReasonAwaitingApproval || v.Reason == model.ReasonApprovalMissed
}

// Explanation is the detail view's answer for a run in which no job
// ran: a headline, then what it means and what to do.
func Explanation(v model.RunVerdict, run model.Run) (string, []string) {
	switch v.Reason {
	case model.ReasonAwaitingApproval:
		if run.FromFork {
			return "Waiting for a maintainer to approve", []string{
				"a run from a fork waits until a write collaborator approves it",
			}
		}
		return "Waiting for a maintainer to approve", []string{
			"GitHub holds bot and agent pull requests until someone approves",
		}
	case model.ReasonApprovalMissed:
		return "Never approved", []string{
			"held for approval, then ended red when its pull request closed",
			"nothing in the code ran · nothing to fix here",
		}
	case model.ReasonBlockedByPolicy:
		// The API names no cause: a pull_request_target run that could
		// not start is, almost always, GitHub's default block.
		return "Likely blocked by the repo's Actions policy", []string{
			"GitHub blocks pull_request_target in public repos by default (enforced 2 Dec 2026)",
			"allow it in an Actions event policy, or move the workflow to pull_request",
		}
	case model.ReasonCouldNotStart:
		return "GitHub could not start this run", []string{
			"check the workflow file and the repo's Actions policy",
		}
	case model.ReasonNoJobRan:
		return "GitHub ended this run before any job started", []string{
			"the run page may say why · o opens it",
		}
	}
	return "", nil
}
