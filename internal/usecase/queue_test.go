package usecase

import (
	"context"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestQueueServiceInspectRunBlockedByWorkflowGroup(t *testing.T) {
	ctx := context.Background()
	pos0, pos1 := 0, 1
	github := &queueTestGitHub{
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "deploy-prod",
			Members:   []model.ConcurrencyMember{{RunID: 30433644, Status: model.StatusPending, Position: &pos1, Target: true}},
		}},
		group: model.ConcurrencyGroup{
			GroupName: "deploy-prod",
			Members: []model.ConcurrencyMember{
				{RunID: 30433642, Status: model.StatusInProgress, Position: &pos0},
				{RunID: 30433644, Status: model.StatusPending, Position: &pos1, Target: true},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusBlocked {
		t.Fatalf("status = %q, want %q", report.Status, model.ConcurrencyStatusBlocked)
	}
	if report.Decision.Action != model.ConcurrencyActionWait {
		t.Fatalf("decision action = %q, want %q", report.Decision.Action, model.ConcurrencyActionWait)
	}
	if got := report.BlockedByCount(); got != 1 {
		t.Fatalf("blocked-by count = %d, want 1", got)
	}
	if !report.Groups[0].Members[0].BlocksTarget {
		t.Fatalf("visible group member was not marked as a blocker: %#v", report.Groups[0].Members[0])
	}
	if len(github.aheadFilters) != 1 || github.aheadFilters[0].AheadOfRun != 30433644 {
		t.Fatalf("ahead filters = %+v, want ahead_of_run", github.aheadFilters)
	}
}

func TestQueueServiceInspectRunBlockedByJobLevelGroup(t *testing.T) {
	ctx := context.Background()
	jobID := int64(798245260)
	holderJobID := int64(798245259)
	pos0, pos2 := 0, 2
	github := &queueTestGitHub{
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "ci-build",
			Members: []model.ConcurrencyMember{{
				RunID:       30433644,
				JobID:       &jobID,
				Status:      model.StatusPending,
				Position:    &pos2,
				PositionURL: "https://api.github.com/repos/indrasvat/gh-hound/actions/concurrency_groups/ci-build?ahead_of_job=798245260",
			}},
		}},
		group: model.ConcurrencyGroup{
			GroupName: "ci-build",
			Members: []model.ConcurrencyMember{
				{RunID: 30433642, JobID: &holderJobID, Status: model.StatusInProgress, Position: &pos0},
				{RunID: 30433644, JobID: &jobID, Status: model.StatusPending, Position: &pos2},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusBlocked {
		t.Fatalf("status = %q, want %q", report.Status, model.ConcurrencyStatusBlocked)
	}
	if len(github.aheadFilters) != 1 || github.aheadFilters[0].AheadOfJob != jobID {
		t.Fatalf("ahead filters = %+v, want ahead_of_job=%d", github.aheadFilters, jobID)
	}
	if !report.Groups[0].Members[1].Target {
		t.Fatalf("job-level member for target run was not marked as target: %#v", report.Groups[0].Members[1])
	}
	if !report.Groups[0].Members[0].BlocksTarget {
		t.Fatalf("job-level holder was not marked as blocker: %#v", report.Groups[0].Members[0])
	}
}

func TestQueueServiceKeepsConfiguredTargetWhenAheadPayloadOmitsIt(t *testing.T) {
	ctx := context.Background()
	pos0, pos1 := 0, 1
	github := &queueTestGitHub{
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "deploy-prod",
			Members: []model.ConcurrencyMember{{
				RunID:    30433644,
				Status:   model.StatusPending,
				Position: &pos1,
			}},
		}},
		group: model.ConcurrencyGroup{
			GroupName: "deploy-prod",
			Members: []model.ConcurrencyMember{
				{RunID: 30433642, Status: model.StatusInProgress, Position: &pos0},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusBlocked {
		t.Fatalf("status = %q, want blocked: %#v", report.Status, report.Groups)
	}
	if len(report.Groups[0].Members) != 2 {
		t.Fatalf("members = %#v, want blocker plus configured target", report.Groups[0].Members)
	}
	if !report.Groups[0].Members[0].BlocksTarget {
		t.Fatalf("holder was not marked as blocker: %#v", report.Groups[0].Members[0])
	}
	if !report.Groups[0].Members[1].Target {
		t.Fatalf("configured target was not preserved: %#v", report.Groups[0].Members[1])
	}
}

func TestQueueServiceMergesMultipleJobFiltersForOneGroup(t *testing.T) {
	ctx := context.Background()
	jobID1, jobID2, holderJobID := int64(798245260), int64(798245261), int64(798245259)
	pos0, pos1, pos2 := 0, 1, 2
	groupCount := 1
	github := &queueTestGitHub{
		runGroups: []model.ConcurrencyGroup{{
			GroupName:            "ci-build",
			ConfiguredGroupCount: &groupCount,
			Members: []model.ConcurrencyMember{
				{RunID: 30433644, JobID: &jobID1, Status: model.StatusPending, Position: &pos1},
				{RunID: 30433644, JobID: &jobID2, Status: model.StatusPending, Position: &pos2},
			},
		}},
		groupsByFilter: map[ConcurrencyAheadFilter]model.ConcurrencyGroup{
			{AheadOfJob: jobID1}: {
				GroupName: "ci-build",
				Members: []model.ConcurrencyMember{
					{RunID: 30433642, JobID: &holderJobID, Status: model.StatusInProgress, Position: &pos0},
					{RunID: 30433644, JobID: &jobID1, Status: model.StatusPending},
				},
			},
			{AheadOfJob: jobID2}: {
				GroupName: "ci-build",
				Members: []model.ConcurrencyMember{
					{RunID: 30433642, JobID: &holderJobID, Status: model.StatusInProgress, Position: &pos0},
					{RunID: 30433644, JobID: &jobID2, Status: model.StatusPending},
				},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if len(report.Groups) != 1 {
		t.Fatalf("group count = %d, want 1 merged group: %#v", len(report.Groups), report.Groups)
	}
	if report.ConfiguredGroupCount != 1 {
		t.Fatalf("configured_group_count = %d, want API count 1", report.ConfiguredGroupCount)
	}
	if len(report.Blockers) != 1 {
		t.Fatalf("blocker count = %d, want deduped holder once: %#v", len(report.Blockers), report.Blockers)
	}
	if report.Groups[0].Members[1].Position == nil || *report.Groups[0].Members[1].Position != pos1 {
		t.Fatalf("job position was not preserved from run-group endpoint: %#v", report.Groups[0].Members[1])
	}
}

func TestQueueServiceMarksBlockersBeforeAnyTargetMember(t *testing.T) {
	ctx := context.Background()
	jobID1, jobID2, holderJobID := int64(798245260), int64(798245261), int64(798245259)
	pos0, pos1, pos2 := 0, 1, 2
	github := &queueTestGitHub{
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "ci-build",
			Members: []model.ConcurrencyMember{
				{RunID: 30433644, JobID: &jobID1, Status: model.StatusPending, Position: &pos1},
				{RunID: 30433644, JobID: &jobID2, Status: model.StatusPending, Position: &pos2},
			},
		}},
		groupsByFilter: map[ConcurrencyAheadFilter]model.ConcurrencyGroup{
			{AheadOfJob: jobID1}: {
				GroupName: "ci-build",
				Members: []model.ConcurrencyMember{
					{RunID: 30433644, JobID: &jobID1, Status: model.StatusPending},
				},
			},
			{AheadOfJob: jobID2}: {
				GroupName: "ci-build",
				Members: []model.ConcurrencyMember{
					{RunID: 30433642, JobID: &holderJobID, Status: model.StatusInProgress, Position: &pos0},
					{RunID: 30433644, JobID: &jobID2, Status: model.StatusPending},
				},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusBlocked {
		t.Fatalf("status = %q, want blocked: %#v", report.Status, report.Groups[0].Members)
	}
	if len(report.Blockers) != 1 {
		t.Fatalf("blocker count = %d, want one blocker before second target: %#v", len(report.Blockers), report.Groups[0].Members)
	}
	if !report.Groups[0].Members[1].BlocksTarget {
		t.Fatalf("member before later target was not marked as blocker: %#v", report.Groups[0].Members)
	}
}

func TestQueueServiceInspectRunNotConcurrency(t *testing.T) {
	ctx := context.Background()
	github := &queueTestGitHub{runGroups: []model.ConcurrencyGroup{}}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusNotConcurrency {
		t.Fatalf("status = %q, want %q", report.Status, model.ConcurrencyStatusNotConcurrency)
	}
	if report.Decision.Action != model.ConcurrencyActionInspectGatesOrRunner {
		t.Fatalf("decision action = %q, want %q", report.Decision.Action, model.ConcurrencyActionInspectGatesOrRunner)
	}
}

func TestQueueServiceTargetNotMemberMapsToNotConcurrency(t *testing.T) {
	ctx := context.Background()
	github := &queueTestGitHub{
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "deploy-prod",
			Members:   []model.ConcurrencyMember{{RunID: 30433644, Status: model.StatusPending}},
		}},
		groupErr: APIError{Kind: APIErrorUnknown, Status: 422, Message: "target is not in this concurrency group"},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectRun(ctx, "indrasvat/gh-hound", 30433644)
	if err != nil {
		t.Fatalf("InspectRun returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusNotConcurrency {
		t.Fatalf("status = %q, want not_concurrency", report.Status)
	}
}

func TestQueueServiceInspectJobMarksWorkflowScopedTarget(t *testing.T) {
	ctx := context.Background()
	jobID := int64(798245260)
	pos0, pos1 := 0, 1
	github := &queueTestGitHub{
		job: model.Job{ID: jobID, RunID: 30433644, Name: "build"},
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "deploy-prod",
			Members:   []model.ConcurrencyMember{{RunID: 30433644, Status: model.StatusPending, Position: &pos1}},
		}},
		group: model.ConcurrencyGroup{
			GroupName: "deploy-prod",
			Members: []model.ConcurrencyMember{
				{RunID: 30433642, Status: model.StatusInProgress, Position: &pos0},
				{RunID: 30433644, Status: model.StatusPending},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectJob(ctx, "indrasvat/gh-hound", jobID)
	if err != nil {
		t.Fatalf("InspectJob returned error: %v", err)
	}
	if report.Status != model.ConcurrencyStatusBlocked {
		t.Fatalf("status = %q, want blocked", report.Status)
	}
	if !report.Groups[0].Members[1].Target {
		t.Fatalf("workflow-scoped member for job target was not marked target: %#v", report.Groups[0].Members[1])
	}
	if report.Groups[0].TargetPosition == nil || *report.Groups[0].TargetPosition != pos1 {
		t.Fatalf("target_position = %v, want %d", report.Groups[0].TargetPosition, pos1)
	}
}

func TestQueueServiceInspectJobResolvesParentRun(t *testing.T) {
	ctx := context.Background()
	jobID := int64(798245260)
	holderJobID := int64(798245259)
	pos0, pos1 := 0, 1
	github := &queueTestGitHub{
		job: model.Job{ID: jobID, RunID: 30433644, Name: "build"},
		runGroups: []model.ConcurrencyGroup{{
			GroupName: "ci-build",
			Members:   []model.ConcurrencyMember{{RunID: 30433644, JobID: &jobID, Status: model.StatusPending, Position: &pos1, Target: true}},
		}},
		group: model.ConcurrencyGroup{
			GroupName: "ci-build",
			Members: []model.ConcurrencyMember{
				{RunID: 30433642, JobID: &holderJobID, Status: model.StatusInProgress, Position: &pos0},
				{RunID: 30433644, JobID: &jobID, Status: model.StatusPending, Position: &pos1, Target: true},
			},
		},
	}

	report, err := (QueueService{GitHub: github, Jobs: github}).InspectJob(ctx, "indrasvat/gh-hound", jobID)
	if err != nil {
		t.Fatalf("InspectJob returned error: %v", err)
	}
	if !github.getJobCalled {
		t.Fatal("InspectJob did not resolve the parent run through GetJob")
	}
	if report.Target.Kind != model.ConcurrencyTargetJob || report.Target.JobID != jobID || report.Target.RunID != 30433644 {
		t.Fatalf("target = %+v, want job %d on run 30433644", report.Target, jobID)
	}
	if report.Status != model.ConcurrencyStatusBlocked {
		t.Fatalf("status = %q, want %q", report.Status, model.ConcurrencyStatusBlocked)
	}
	if len(github.aheadFilters) != 1 || github.aheadFilters[0].AheadOfJob != jobID {
		t.Fatalf("ahead filters = %+v, want ahead_of_job", github.aheadFilters)
	}
}

type queueTestGitHub struct {
	runGroups      []model.ConcurrencyGroup
	group          model.ConcurrencyGroup
	groupsByFilter map[ConcurrencyAheadFilter]model.ConcurrencyGroup
	groupErr       error
	job            model.Job
	aheadFilters   []ConcurrencyAheadFilter
	getJobCalled   bool
}

func (g *queueTestGitHub) ListConcurrencyGroups(context.Context, string, ConcurrencyListOptions) ([]model.ConcurrencyGroupSummary, error) {
	return nil, nil
}

func (g *queueTestGitHub) GetConcurrencyGroup(_ context.Context, _ string, _ string, filter ConcurrencyAheadFilter) (model.ConcurrencyGroup, error) {
	g.aheadFilters = append(g.aheadFilters, filter)
	if g.groupErr != nil {
		return model.ConcurrencyGroup{}, g.groupErr
	}
	if group, ok := g.groupsByFilter[filter]; ok {
		return group, nil
	}
	if g.group.GroupName == "" {
		return model.ConcurrencyGroup{}, nil
	}
	return g.group, nil
}

func (g *queueTestGitHub) ListRunConcurrencyGroups(context.Context, string, int64, ConcurrencyListOptions) ([]model.ConcurrencyGroup, error) {
	return g.runGroups, nil
}

func (g *queueTestGitHub) GetJob(context.Context, string, int64) (model.Job, error) {
	g.getJobCalled = true
	return g.job, nil
}
