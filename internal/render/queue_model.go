package render

import (
	"github.com/indrasvat/gh-hound/internal/model"
)

func QueueResultFromReport(report model.ConcurrencyReport) QueueResult {
	groups := make([]QueueGroup, 0, len(report.Groups))
	for _, group := range report.Groups {
		var activeMemberCount *int
		if len(group.Members) > 0 {
			count := len(group.Members)
			activeMemberCount = &count
		}
		if group.ActiveMemberCount > 0 {
			count := group.ActiveMemberCount
			activeMemberCount = &count
		}
		members := make([]QueueMember, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, QueueMember{
				RunID:        member.RunID,
				RunName:      member.RunName,
				RunHTMLURL:   member.RunHTMLURL,
				JobID:        member.JobID,
				JobName:      member.JobName,
				JobHTMLURL:   member.JobHTMLURL,
				Status:       string(member.Status),
				Position:     member.Position,
				Target:       member.Target,
				BlocksTarget: member.BlocksTarget,
			})
		}
		groups = append(groups, QueueGroup{
			GroupName:            group.GroupName,
			GroupURL:             group.GroupURL,
			LastAcquiredAt:       group.LastAcquiredAt,
			Scope:                string(group.Scope()),
			State:                queueGroupState(report.Status, group),
			ConfiguredGroupCount: group.ConfiguredGroupCount,
			ActiveMemberCount:    activeMemberCount,
			TargetPosition:       group.TargetPosition,
			BlockedByCount:       group.BlockedByCount(),
			Members:              members,
		})
	}
	blockers := make([]QueueBlocker, 0, len(report.Blockers))
	for _, blocker := range report.Blockers {
		blockers = append(blockers, QueueBlocker{
			GroupName: queueReportGroupName(report.Groups, blocker),
			RunID:     blocker.RunID,
			JobID:     blocker.JobID,
			Status:    string(blocker.Status),
		})
	}
	return QueueResult{
		Repo:                 report.Repo,
		Target:               queueTargetFromModel(report.Target),
		FetchedAt:            report.FetchedAt,
		Status:               string(report.Status),
		Decision:             queueDecisionFromModel(report.Decision),
		Summary:              report.Summary,
		ConfiguredGroupCount: report.ConfiguredGroupCount,
		Groups:               groups,
		Blockers:             blockers,
	}
}

func queueGroupState(status model.ConcurrencyStatus, group model.ConcurrencyGroup) string {
	switch status {
	case model.ConcurrencyStatusListed, model.ConcurrencyStatusInactive, model.ConcurrencyStatusUnsupported, model.ConcurrencyStatusError:
		return string(status)
	}
	if group.BlockedByCount() > 0 {
		return string(model.ConcurrencyStatusBlocked)
	}
	if len(group.Members) == 0 {
		return string(model.ConcurrencyStatusNotConcurrency)
	}
	return string(model.ConcurrencyStatusNotConcurrency)
}

func queueTargetFromModel(target model.ConcurrencyTarget) QueueTarget {
	return QueueTarget{
		Kind:      string(target.Kind),
		RunID:     target.RunID,
		JobID:     target.JobID,
		GroupName: target.GroupName,
	}
}

func queueDecisionFromModel(decision model.ConcurrencyDecision) QueueDecision {
	return QueueDecision{
		Action:       string(decision.Action),
		Confidence:   decision.Confidence,
		Reason:       decision.Reason,
		NextCommands: decision.NextCommands,
	}
}

func queueReportGroupName(groups []model.ConcurrencyGroup, blocker model.ConcurrencyMember) string {
	for _, group := range groups {
		for _, member := range group.Members {
			if member.RunID == blocker.RunID && sameQueueJobID(member.JobID, blocker.JobID) {
				return group.GroupName
			}
		}
	}
	return ""
}

func sameQueueJobID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
