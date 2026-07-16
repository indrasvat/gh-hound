package model

import (
	"strings"
	"time"
)

type ConcurrencyStatus string

const (
	ConcurrencyStatusBlocked        ConcurrencyStatus = "blocked"
	ConcurrencyStatusNotConcurrency ConcurrencyStatus = "not_concurrency"
	ConcurrencyStatusInactive       ConcurrencyStatus = "inactive"
	ConcurrencyStatusListed         ConcurrencyStatus = "listed"
	ConcurrencyStatusUnsupported    ConcurrencyStatus = "unsupported"
	ConcurrencyStatusError          ConcurrencyStatus = "error"
)

type ConcurrencyDecisionAction string

const (
	ConcurrencyActionWait                 ConcurrencyDecisionAction = "wait"
	ConcurrencyActionInspectGatesOrRunner ConcurrencyDecisionAction = "inspect_gates_or_runner"
	ConcurrencyActionNormalWatch          ConcurrencyDecisionAction = "normal_watch"
	ConcurrencyActionListOnly             ConcurrencyDecisionAction = "list_only"
	ConcurrencyActionUnsupported          ConcurrencyDecisionAction = "unsupported"
	ConcurrencyActionError                ConcurrencyDecisionAction = "error"
)

type ConcurrencyScope string

const (
	ConcurrencyScopeWorkflow ConcurrencyScope = "workflow"
	ConcurrencyScopeJob      ConcurrencyScope = "job"
	ConcurrencyScopeMixed    ConcurrencyScope = "mixed"
	ConcurrencyScopeUnknown  ConcurrencyScope = "unknown"
)

type ConcurrencyTargetKind string

const (
	ConcurrencyTargetUnknown ConcurrencyTargetKind = "unknown"
	ConcurrencyTargetRun     ConcurrencyTargetKind = "run"
	ConcurrencyTargetJob     ConcurrencyTargetKind = "job"
	ConcurrencyTargetGroup   ConcurrencyTargetKind = "group"
	ConcurrencyTargetList    ConcurrencyTargetKind = "list"
)

type ConcurrencyTarget struct {
	Kind      ConcurrencyTargetKind
	RunID     int64
	JobID     int64
	GroupName string
}

type ConcurrencyDecision struct {
	Action       ConcurrencyDecisionAction
	Confidence   string
	Reason       string
	NextCommands []string
}

type ConcurrencyGroupSummary struct {
	GroupName      string
	GroupURL       string
	LastAcquiredAt *time.Time
}

type ConcurrencyGroup struct {
	GroupName            string
	GroupURL             string
	LastAcquiredAt       *time.Time
	ConfiguredGroupCount *int
	ActiveMemberCount    int
	TargetPosition       *int
	Members              []ConcurrencyMember
}

type ConcurrencyMember struct {
	RunID        int64
	RunName      string
	RunURL       string
	RunHTMLURL   string
	JobID        *int64
	JobName      string
	JobURL       string
	JobHTMLURL   string
	Status       Status
	Position     *int
	PositionURL  string
	Target       bool
	BlocksTarget bool
}

type ConcurrencyReport struct {
	Repo                 string
	Target               ConcurrencyTarget
	FetchedAt            time.Time
	Status               ConcurrencyStatus
	Decision             ConcurrencyDecision
	Summary              string
	ConfiguredGroupCount int
	Groups               []ConcurrencyGroup
	Blockers             []ConcurrencyMember
}

func (r ConcurrencyReport) BlockedByCount() int {
	count := 0
	for _, group := range r.Groups {
		count += group.BlockedByCount()
	}
	return count
}

func (g ConcurrencyGroup) TargetMember() (ConcurrencyMember, bool) {
	for _, member := range g.Members {
		if member.Target {
			return member, true
		}
	}
	return ConcurrencyMember{}, false
}

func (g ConcurrencyGroup) BlockedByCount() int {
	marked := 0
	for _, member := range g.Members {
		if member.BlocksTarget {
			marked++
		}
	}
	if marked > 0 {
		return marked
	}
	targetsAfter := make([]bool, len(g.Members))
	seenTarget := false
	for i := len(g.Members) - 1; i >= 0; i-- {
		targetsAfter[i] = seenTarget
		if g.Members[i].Target {
			seenTarget = true
		}
	}
	if !seenTarget {
		return 0
	}
	count := 0
	for i, member := range g.Members {
		if member.Target {
			continue
		}
		if targetsAfter[i] && (member.RunID != 0 || member.JobID != nil) {
			count++
		}
	}
	return count
}

func (g ConcurrencyGroup) Scope() ConcurrencyScope {
	if len(g.Members) == 0 {
		return ConcurrencyScopeUnknown
	}
	hasWorkflow, hasJob := false, false
	for _, member := range g.Members {
		if member.JobID == nil {
			hasWorkflow = true
		} else {
			hasJob = true
		}
	}
	switch {
	case hasWorkflow && hasJob:
		return ConcurrencyScopeMixed
	case hasJob:
		return ConcurrencyScopeJob
	case hasWorkflow:
		return ConcurrencyScopeWorkflow
	default:
		return ConcurrencyScopeUnknown
	}
}

func SameConcurrencyGroup(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
