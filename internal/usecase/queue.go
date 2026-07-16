package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

type JobGetter interface {
	GetJob(context.Context, string, int64) (model.Job, error)
}

type QueueService struct {
	GitHub GitHubConcurrency
	Jobs   JobGetter
}

func (s QueueService) InspectRun(ctx context.Context, repo string, runID int64) (model.ConcurrencyReport, error) {
	if err := validateQueueTarget(repo, runID, "run"); err != nil {
		return model.ConcurrencyReport{}, err
	}
	if s.GitHub == nil {
		return model.ConcurrencyReport{}, fmt.Errorf("queue diagnostics are unavailable for this adapter")
	}
	groups, err := s.GitHub.ListRunConcurrencyGroups(ctx, repo, runID, ConcurrencyListOptions{})
	if err != nil {
		return model.ConcurrencyReport{}, err
	}
	groups, err = s.groupsAheadOfTarget(ctx, repo, groups, model.ConcurrencyTarget{Kind: model.ConcurrencyTargetRun, RunID: runID})
	if err != nil {
		return model.ConcurrencyReport{}, err
	}
	return buildQueueReport(repo, model.ConcurrencyTarget{Kind: model.ConcurrencyTargetRun, RunID: runID}, groups), nil
}

func (s QueueService) InspectJob(ctx context.Context, repo string, jobID int64) (model.ConcurrencyReport, error) {
	if err := validateQueueTarget(repo, jobID, "job"); err != nil {
		return model.ConcurrencyReport{}, err
	}
	if s.GitHub == nil {
		return model.ConcurrencyReport{}, fmt.Errorf("queue diagnostics are unavailable for this adapter")
	}
	if s.Jobs == nil {
		return model.ConcurrencyReport{}, fmt.Errorf("job lookup is unavailable for queue diagnostics")
	}
	job, err := s.Jobs.GetJob(ctx, repo, jobID)
	if err != nil {
		return model.ConcurrencyReport{}, err
	}
	groups, err := s.GitHub.ListRunConcurrencyGroups(ctx, repo, job.RunID, ConcurrencyListOptions{})
	if err != nil {
		return model.ConcurrencyReport{}, err
	}
	target := model.ConcurrencyTarget{Kind: model.ConcurrencyTargetJob, RunID: job.RunID, JobID: jobID}
	groups, err = s.groupsAheadOfTarget(ctx, repo, groups, target)
	if err != nil {
		return model.ConcurrencyReport{}, err
	}
	return buildQueueReport(repo, target, groups), nil
}

func (s QueueService) groupsAheadOfTarget(ctx context.Context, repo string, configured []model.ConcurrencyGroup, target model.ConcurrencyTarget) ([]model.ConcurrencyGroup, error) {
	if len(configured) == 0 {
		return configured, nil
	}
	groups := make([]model.ConcurrencyGroup, 0, len(configured))
	for _, group := range configured {
		filters := aheadFiltersForGroup(group, target)
		if len(filters) == 0 {
			groups = append(groups, group)
			continue
		}
		var merged model.ConcurrencyGroup
		appended := false
		for _, filter := range filters {
			live, err := s.GitHub.GetConcurrencyGroup(ctx, repo, group.GroupName, filter)
			switch {
			case err == nil:
				live = mergeConfiguredQueueEvidence(group, live, target, filter)
				if appended {
					merged = mergeConcurrencyGroups(merged, live)
				} else {
					merged = live
				}
				appended = true
			case isConcurrencyGroupInactive(err), isConcurrencyTargetNotMember(err):
				continue
			default:
				return nil, err
			}
		}
		if !appended {
			group.Members = nil
			group.ActiveMemberCount = 0
			groups = append(groups, group)
			continue
		}
		groups = append(groups, merged)
	}
	return groups, nil
}

func aheadFiltersForGroup(group model.ConcurrencyGroup, target model.ConcurrencyTarget) []ConcurrencyAheadFilter {
	switch target.Kind {
	case model.ConcurrencyTargetRun:
		var filters []ConcurrencyAheadFilter
		for _, member := range group.Members {
			if member.RunID != target.RunID {
				continue
			}
			if filter, ok := aheadFilterFromPositionURL(member.PositionURL); ok {
				filters = append(filters, filter)
				continue
			}
			if member.JobID != nil && *member.JobID > 0 {
				filters = append(filters, ConcurrencyAheadFilter{AheadOfJob: *member.JobID})
			} else {
				filters = append(filters, ConcurrencyAheadFilter{AheadOfRun: target.RunID})
			}
		}
		if len(filters) == 0 {
			filters = append(filters, ConcurrencyAheadFilter{AheadOfRun: target.RunID})
		}
		return dedupeAheadFilters(filters)
	case model.ConcurrencyTargetJob:
		if target.JobID <= 0 {
			return nil
		}
		return []ConcurrencyAheadFilter{{AheadOfJob: target.JobID}}
	default:
		return nil
	}
}

func aheadFilterFromPositionURL(value string) (ConcurrencyAheadFilter, bool) {
	if strings.TrimSpace(value) == "" {
		return ConcurrencyAheadFilter{}, false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ConcurrencyAheadFilter{}, false
	}
	query := parsed.Query()
	if raw := query.Get("ahead_of_job"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		return ConcurrencyAheadFilter{AheadOfJob: id}, err == nil && id > 0
	}
	if raw := query.Get("ahead_of_run"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		return ConcurrencyAheadFilter{AheadOfRun: id}, err == nil && id > 0
	}
	return ConcurrencyAheadFilter{}, false
}

func dedupeAheadFilters(filters []ConcurrencyAheadFilter) []ConcurrencyAheadFilter {
	seen := make(map[ConcurrencyAheadFilter]struct{}, len(filters))
	out := make([]ConcurrencyAheadFilter, 0, len(filters))
	for _, filter := range filters {
		if filter.AheadOfRun <= 0 && filter.AheadOfJob <= 0 {
			continue
		}
		if _, ok := seen[filter]; ok {
			continue
		}
		seen[filter] = struct{}{}
		out = append(out, filter)
	}
	return out
}

func isConcurrencyGroupInactive(err error) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && apiErr.Kind == APIErrorNotFound
}

func isConcurrencyTargetNotMember(err error) bool {
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 422 {
		return false
	}
	message := strings.ToLower(apiErr.Message)
	return strings.Contains(message, "not in this concurrency group") ||
		strings.Contains(message, "not in the concurrency group") ||
		strings.Contains(message, "not a member")
}

func validateQueueTarget(repo string, id int64, field string) error {
	if strings.TrimSpace(repo) == "" {
		return fmt.Errorf("repo is required")
	}
	if id <= 0 {
		return fmt.Errorf("%s id must be positive", field)
	}
	return nil
}

func buildQueueReport(repo string, target model.ConcurrencyTarget, groups []model.ConcurrencyGroup) model.ConcurrencyReport {
	groups = markConcurrencyTargets(groups, target)
	report := model.ConcurrencyReport{
		Repo:                 repo,
		Target:               target,
		FetchedAt:            time.Now().UTC(),
		Groups:               groups,
		ConfiguredGroupCount: configuredGroupCount(groups),
	}
	for i := range groups {
		report.Blockers = append(report.Blockers, markBlockersForGroup(&groups[i])...)
	}
	report.Groups = groups
	if len(report.Blockers) == 0 {
		report.Status = model.ConcurrencyStatusNotConcurrency
		report.Decision = model.ConcurrencyDecision{
			Action:     model.ConcurrencyActionInspectGatesOrRunner,
			Confidence: "high",
			Reason:     "GitHub did not expose a concurrency blocker for this target",
		}
		report.Summary = "no concurrency blocker found"
		return report
	}
	report.Status = model.ConcurrencyStatusBlocked
	report.Decision = model.ConcurrencyDecision{
		Action:     model.ConcurrencyActionWait,
		Confidence: "high",
		Reason:     "target is pending behind a GitHub Actions concurrency group",
	}
	report.Summary = fmt.Sprintf("%d item(s) ahead of target", len(report.Blockers))
	return report
}

func configuredGroupCount(groups []model.ConcurrencyGroup) int {
	for _, group := range groups {
		if group.ConfiguredGroupCount != nil {
			return *group.ConfiguredGroupCount
		}
	}
	return len(groups)
}

func markConcurrencyTargets(groups []model.ConcurrencyGroup, target model.ConcurrencyTarget) []model.ConcurrencyGroup {
	out := make([]model.ConcurrencyGroup, len(groups))
	copy(out, groups)
	for i := range out {
		members := make([]model.ConcurrencyMember, len(out[i].Members))
		copy(members, out[i].Members)
		for j := range members {
			switch target.Kind {
			case model.ConcurrencyTargetRun:
				if members[j].RunID == target.RunID {
					members[j].Target = true
				}
			case model.ConcurrencyTargetJob:
				if members[j].RunID == target.RunID && (members[j].JobID == nil || *members[j].JobID == target.JobID) {
					members[j].Target = true
				}
			}
			if members[j].Target && out[i].TargetPosition == nil && members[j].Position != nil {
				pos := *members[j].Position
				out[i].TargetPosition = &pos
			}
		}
		out[i].Members = members
	}
	return out
}

func mergeConfiguredQueueEvidence(configured, live model.ConcurrencyGroup, target model.ConcurrencyTarget, filter ConcurrencyAheadFilter) model.ConcurrencyGroup {
	if live.GroupName == "" {
		live.GroupName = configured.GroupName
	}
	if live.GroupURL == "" {
		live.GroupURL = configured.GroupURL
	}
	if live.LastAcquiredAt == nil {
		live.LastAcquiredAt = configured.LastAcquiredAt
	}
	live.ConfiguredGroupCount = configured.ConfiguredGroupCount
	if live.TargetPosition == nil {
		live.TargetPosition = configured.TargetPosition
	}
	configuredByKey := make(map[string]model.ConcurrencyMember, len(configured.Members))
	for _, member := range configured.Members {
		configuredByKey[concurrencyMemberKey(member)] = member
	}
	for i := range live.Members {
		if configuredMember, ok := configuredByKey[concurrencyMemberKey(live.Members[i])]; ok {
			live.Members[i] = mergeConcurrencyMemberEvidence(configuredMember, live.Members[i])
		}
	}
	seen := make(map[string]struct{}, len(live.Members))
	for _, member := range live.Members {
		seen[concurrencyMemberKey(member)] = struct{}{}
	}
	for _, member := range configured.Members {
		key := concurrencyMemberKey(member)
		if _, ok := seen[key]; ok || !concurrencyMemberMatchesAheadFilter(member, target, filter) {
			continue
		}
		seen[key] = struct{}{}
		live.Members = append(live.Members, member)
	}
	if live.ActiveMemberCount == 0 && len(live.Members) > 0 {
		live.ActiveMemberCount = len(live.Members)
	}
	return live
}

func concurrencyMemberMatchesAheadFilter(member model.ConcurrencyMember, target model.ConcurrencyTarget, filter ConcurrencyAheadFilter) bool {
	switch target.Kind {
	case model.ConcurrencyTargetRun:
		if member.RunID != target.RunID {
			return false
		}
		if filter.AheadOfJob > 0 {
			return member.JobID != nil && *member.JobID == filter.AheadOfJob
		}
		if filter.AheadOfRun > 0 {
			return member.JobID == nil
		}
		return false
	case model.ConcurrencyTargetJob:
		return member.RunID == target.RunID && (member.JobID == nil || *member.JobID == target.JobID)
	default:
		return false
	}
}

func mergeConcurrencyGroups(left, right model.ConcurrencyGroup) model.ConcurrencyGroup {
	if left.GroupName == "" {
		left.GroupName = right.GroupName
	}
	if left.GroupURL == "" {
		left.GroupURL = right.GroupURL
	}
	if left.LastAcquiredAt == nil {
		left.LastAcquiredAt = right.LastAcquiredAt
	}
	if left.ConfiguredGroupCount == nil {
		left.ConfiguredGroupCount = right.ConfiguredGroupCount
	}
	if left.TargetPosition == nil {
		left.TargetPosition = right.TargetPosition
	}
	if right.ActiveMemberCount > left.ActiveMemberCount {
		left.ActiveMemberCount = right.ActiveMemberCount
	}
	seen := make(map[string]int, len(left.Members))
	for i, member := range left.Members {
		seen[concurrencyMemberKey(member)] = i
	}
	for _, member := range right.Members {
		key := concurrencyMemberKey(member)
		if index, ok := seen[key]; ok {
			left.Members[index] = mergeConcurrencyMemberEvidence(left.Members[index], member)
			continue
		}
		seen[key] = len(left.Members)
		left.Members = append(left.Members, member)
	}
	return left
}

func mergeConcurrencyMemberEvidence(left, right model.ConcurrencyMember) model.ConcurrencyMember {
	if right.RunName == "" {
		right.RunName = left.RunName
	}
	if right.RunURL == "" {
		right.RunURL = left.RunURL
	}
	if right.RunHTMLURL == "" {
		right.RunHTMLURL = left.RunHTMLURL
	}
	if right.JobID == nil {
		right.JobID = left.JobID
	}
	if right.JobName == "" {
		right.JobName = left.JobName
	}
	if right.JobURL == "" {
		right.JobURL = left.JobURL
	}
	if right.JobHTMLURL == "" {
		right.JobHTMLURL = left.JobHTMLURL
	}
	if right.Position == nil {
		right.Position = left.Position
	}
	if right.PositionURL == "" {
		right.PositionURL = left.PositionURL
	}
	return right
}

func concurrencyMemberKey(member model.ConcurrencyMember) string {
	job := int64(0)
	if member.JobID != nil {
		job = *member.JobID
	}
	return fmt.Sprintf("%d/%d", member.RunID, job)
}

func markBlockersForGroup(group *model.ConcurrencyGroup) []model.ConcurrencyMember {
	targetsAfter := make([]bool, len(group.Members))
	seenTarget := false
	for i := len(group.Members) - 1; i >= 0; i-- {
		targetsAfter[i] = seenTarget
		if group.Members[i].Target {
			seenTarget = true
		}
	}
	if !seenTarget {
		return nil
	}
	blockers := make([]model.ConcurrencyMember, 0, len(group.Members))
	for i, member := range group.Members {
		if member.Target {
			continue
		}
		if targetsAfter[i] {
			group.Members[i].BlocksTarget = true
			blockers = append(blockers, group.Members[i])
		}
	}
	return blockers
}
