package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
)

func TestQueueModelNavigationOpenCopyAndBack(t *testing.T) {
	report := testReport(3)
	m := NewModel(report)
	m = m.Update(KeyMsg{Key: "j"})
	if m.Selected != 1 {
		t.Fatalf("selected = %d, want 1", m.Selected)
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentOpenRun || m.Intent.Run.ID != 30433643 {
		t.Fatalf("open intent = %#v", m.Intent)
	}
	m = m.Update(KeyMsg{Key: "y"})
	if m.Intent.Kind != IntentCopy {
		t.Fatalf("copy intent = %#v", m.Intent)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(m.Intent.Text), &decoded); err != nil {
		t.Fatalf("copy payload is not JSON: %v\n%s", err, m.Intent.Text)
	}
	if decoded["status"] != string(model.ConcurrencyStatusBlocked) {
		t.Fatalf("copied status = %v", decoded["status"])
	}
	target := decoded["target"].(map[string]any)
	if _, ok := target["RunID"]; ok {
		t.Fatalf("copy payload leaked raw Go field names: %s", m.Intent.Text)
	}
	if target["run_id"] != float64(30433644) {
		t.Fatalf("target.run_id = %v, want 30433644\n%s", target["run_id"], m.Intent.Text)
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.Intent.Kind != IntentBack {
		t.Fatalf("back intent = %#v", m.Intent)
	}
}

func TestQueueModelDoesNotOpenWithoutMembers(t *testing.T) {
	m := NewModel(model.ConcurrencyReport{Status: model.ConcurrencyStatusNotConcurrency})
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Intent.Kind != IntentNone {
		t.Fatalf("intent = %#v, want none", m.Intent)
	}
}

func testReport(count int) model.ConcurrencyReport {
	if count < 2 {
		count = 2
	}
	members := make([]model.ConcurrencyMember, 0, count)
	for i := range count {
		pos := i
		member := model.ConcurrencyMember{
			RunID:    int64(30433642 + i),
			RunName:  strings.Repeat("Deploy ", 8),
			Status:   model.StatusPending,
			Position: &pos,
		}
		if i == 0 {
			member.Status = model.StatusInProgress
			member.BlocksTarget = true
		}
		if i == count-1 {
			member.RunID = 30433644
			member.Target = true
		}
		members = append(members, member)
	}
	groupCount := 1
	group := model.ConcurrencyGroup{
		GroupName:            "deploy-prod",
		ConfiguredGroupCount: &groupCount,
		ActiveMemberCount:    len(members),
		Members:              members,
	}
	return model.ConcurrencyReport{
		Repo:   "indrasvat/gh-hound",
		Target: model.ConcurrencyTarget{Kind: model.ConcurrencyTargetRun, RunID: 30433644},
		Status: model.ConcurrencyStatusBlocked,
		Decision: model.ConcurrencyDecision{
			Action:     model.ConcurrencyActionWait,
			Confidence: "high",
			Reason:     "target is pending behind a GitHub Actions concurrency group",
		},
		Summary:              "1 item(s) ahead of target",
		ConfiguredGroupCount: 1,
		Groups:               []model.ConcurrencyGroup{group},
		Blockers:             []model.ConcurrencyMember{members[0]},
	}
}
