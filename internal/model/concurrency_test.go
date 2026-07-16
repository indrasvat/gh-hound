package model

import "testing"

func TestConcurrencyGroupFindsTargetAndBlockers(t *testing.T) {
	holder := 0
	targetPos := 2
	group := ConcurrencyGroup{
		GroupName: "Deploy-Prod",
		Members: []ConcurrencyMember{
			{RunID: 101, Status: StatusInProgress, Position: &holder},
			{RunID: 102, Status: StatusPending},
			{RunID: 103, Status: StatusPending, Position: &targetPos, Target: true},
		},
	}

	target, ok := group.TargetMember()
	if !ok {
		t.Fatal("target member not found")
	}
	if target.RunID != 103 {
		t.Fatalf("target run = %d, want 103", target.RunID)
	}
	if got := group.BlockedByCount(); got != 2 {
		t.Fatalf("blocked-by count = %d, want 2", got)
	}
	if got := group.Scope(); got != ConcurrencyScopeWorkflow {
		t.Fatalf("scope = %q, want %q", got, ConcurrencyScopeWorkflow)
	}
}

func TestConcurrencyGroupCountsExplicitBlockersAcrossMultipleTargets(t *testing.T) {
	jobOne, jobTwo, holderJob := int64(1), int64(2), int64(3)
	group := ConcurrencyGroup{
		GroupName: "ci-build",
		Members: []ConcurrencyMember{
			{RunID: 30433644, JobID: &jobOne, Status: StatusPending, Target: true},
			{RunID: 30433642, JobID: &holderJob, Status: StatusInProgress, BlocksTarget: true},
			{RunID: 30433644, JobID: &jobTwo, Status: StatusPending, Target: true},
		},
	}

	if got := group.BlockedByCount(); got != 1 {
		t.Fatalf("blocked-by count = %d, want explicit blocker count 1", got)
	}
}

func TestConcurrencyGroupScopeDetectsJobLevelMembers(t *testing.T) {
	jobID := int64(9001)
	group := ConcurrencyGroup{
		GroupName: "ci-build",
		Members: []ConcurrencyMember{
			{RunID: 101, JobID: &jobID, Status: StatusPending, Target: true},
		},
	}

	if got := group.Scope(); got != ConcurrencyScopeJob {
		t.Fatalf("scope = %q, want %q", got, ConcurrencyScopeJob)
	}
}

func TestConcurrencyGroupNamesMatchCaseInsensitively(t *testing.T) {
	if !SameConcurrencyGroup("Deploy/Prod", "deploy/prod") {
		t.Fatal("group names should match case-insensitively")
	}
	if SameConcurrencyGroup("Deploy/Prod", "deploy/stage") {
		t.Fatal("different group names matched")
	}
}
