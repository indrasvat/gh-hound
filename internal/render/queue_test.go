package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

func fixtureQueueResult() QueueResult {
	pos0, pos1 := 0, 1
	active := 2
	return QueueResult{
		Repo:      "indrasvat/gh-hound",
		FetchedAt: time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC),
		Status:    "blocked",
		Target:    QueueTarget{Kind: "run", RunID: 30433644},
		Decision: QueueDecision{
			Action:     "wait",
			Confidence: "high",
			Reason:     "run 30433644 is pending behind deploy-prod",
		},
		Summary:              "deploy-prod has 1 item ahead of run 30433644.",
		ConfiguredGroupCount: 1,
		Groups: []QueueGroup{{
			GroupName:         "deploy-prod",
			Scope:             "workflow",
			State:             "blocked",
			ActiveMemberCount: &active,
			TargetPosition:    &pos1,
			BlockedByCount:    1,
			Members: []QueueMember{
				{RunID: 30433642, Status: "in_progress", Position: &pos0, BlocksTarget: true},
				{RunID: 30433644, Status: "pending", Position: &pos1, Target: true},
			},
		}},
		Blockers: []QueueBlocker{{GroupName: "deploy-prod", RunID: 30433642, Status: "in_progress"}},
	}
}

func TestQueueJSONMatchesPublicContract(t *testing.T) {
	var out bytes.Buffer
	if err := WriteQueue(&out, FormatJSON, fixtureQueueResult()); err != nil {
		t.Fatalf("WriteQueue returned error: %v", err)
	}
	decoded := decodeJSONMap(t, out.Bytes())
	for _, key := range []string{"repo", "target", "fetched_at", "status", "decision", "summary", "configured_group_count", "groups", "blockers"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("queue result missing key %q in %s", key, out.String())
		}
	}
	if _, ok := decoded["error"]; ok {
		t.Fatalf("queue success result must omit error: %s", out.String())
	}
	decision := decoded["decision"].(map[string]any)
	if decision["action"] != "wait" {
		t.Fatalf("decision.action = %v, want wait", decision["action"])
	}
}

func TestQueueExitCodes(t *testing.T) {
	if got := QueueExitCode(QueueResult{Status: "blocked"}); got != ExitPending {
		t.Fatalf("blocked exit = %d, want %d", got, ExitPending)
	}
	for _, status := range []string{"not_concurrency", "inactive", "listed", "unsupported"} {
		if got := QueueExitCode(QueueResult{Status: status}); got != ExitOK {
			t.Fatalf("%s exit = %d, want %d", status, got, ExitOK)
		}
	}
	if got := QueueExitCode(QueueResult{Status: "error", Error: &QueueError{Kind: "network"}}); got != ExitError {
		t.Fatalf("error exit = %d, want %d", got, ExitError)
	}
}

func TestQueueResultFromReportPreservesActiveMemberCount(t *testing.T) {
	report := model.ConcurrencyReport{
		Repo:      "indrasvat/gh-hound",
		Status:    model.ConcurrencyStatusListed,
		FetchedAt: time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC),
		Target:    model.ConcurrencyTarget{Kind: model.ConcurrencyTargetGroup, GroupName: "deploy-prod"},
		Decision: model.ConcurrencyDecision{
			Action: model.ConcurrencyActionListOnly,
			Reason: "listed concurrency group members",
		},
		Groups: []model.ConcurrencyGroup{{
			GroupName:         "deploy-prod",
			ActiveMemberCount: 100,
			Members:           []model.ConcurrencyMember{{RunID: 30433642, Status: model.StatusPending}},
		}},
	}
	result := QueueResultFromReport(report)
	if result.Groups[0].ActiveMemberCount == nil || *result.Groups[0].ActiveMemberCount != 100 {
		t.Fatalf("active_member_count = %v, want API total 100", result.Groups[0].ActiveMemberCount)
	}
}

func TestQueueResultFromReportPreservesKnownZeroActiveMemberCount(t *testing.T) {
	report := model.ConcurrencyReport{
		Repo:      "indrasvat/gh-hound",
		Status:    model.ConcurrencyStatusListed,
		FetchedAt: time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC),
		Target:    model.ConcurrencyTarget{Kind: model.ConcurrencyTargetGroup, GroupName: "deploy-prod"},
		Decision: model.ConcurrencyDecision{
			Action: model.ConcurrencyActionListOnly,
			Reason: "listed concurrency group members",
		},
		Groups: []model.ConcurrencyGroup{{GroupName: "deploy-prod"}},
	}
	result := QueueResultFromReport(report)
	if result.Groups[0].ActiveMemberCount == nil || *result.Groups[0].ActiveMemberCount != 0 {
		t.Fatalf("active_member_count = %v, want known zero", result.Groups[0].ActiveMemberCount)
	}
}

func TestQueueMarkdownAndXMLAreStructured(t *testing.T) {
	result := fixtureQueueResult()
	var md bytes.Buffer
	if err := WriteQueue(&md, FormatMarkdown, result); err != nil {
		t.Fatalf("markdown: %v", err)
	}
	for _, want := range []string{"# gh-hound queue", "deploy-prod", "30433642"} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("markdown missing %q\n%s", want, md.String())
		}
	}
	var xml bytes.Buffer
	if err := WriteQueue(&xml, FormatXML, result); err != nil {
		t.Fatalf("xml: %v", err)
	}
	for _, want := range []string{"<queue_result", "<group", "<member"} {
		if !strings.Contains(xml.String(), want) {
			t.Fatalf("xml missing %q\n%s", want, xml.String())
		}
	}
}

func TestQueueSchemaDefinesNestedContracts(t *testing.T) {
	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema.json invalid: %v", err)
	}
	defs := schema["$defs"].(map[string]any)
	for _, name := range []string{"queue_result", "queue_target", "queue_group", "queue_member", "queue_blocker", "queue_error"} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("schema.json must define $defs.%s", name)
		}
	}
	group := defs["queue_group"].(map[string]any)
	props := group["properties"].(map[string]any)
	if _, ok := props["last_acquired_at"]; !ok {
		t.Fatal("queue_group must define last_acquired_at")
	}
	members := props["members"].(map[string]any)
	items := members["items"].(map[string]any)
	if items["$ref"] != "#/$defs/queue_member" {
		t.Fatalf("queue_group.members must reference queue_member, got %#v", items)
	}
	for _, name := range []string{"queue_result", "queue_target", "queue_group", "queue_member", "queue_blocker", "queue_error"} {
		def := defs[name].(map[string]any)
		if def["additionalProperties"] != false {
			t.Fatalf("$defs.%s must set additionalProperties false", name)
		}
	}
	target := defs["queue_target"].(map[string]any)
	if _, ok := target["oneOf"]; !ok {
		t.Fatal("queue_target must define oneOf target-kind requirements")
	}
	targetProps := target["properties"].(map[string]any)
	kind := targetProps["kind"].(map[string]any)
	if !enumContains(kind["enum"].([]any), "unknown") {
		t.Fatal("queue_target.kind must allow unknown for validation-error envelopes")
	}
	result := defs["queue_result"].(map[string]any)
	required := result["required"].([]any)
	if !enumContains(required, "fetched_at") {
		t.Fatal("queue_result must require fetched_at")
	}
	queueErr := defs["queue_error"].(map[string]any)
	errProps := queueErr["properties"].(map[string]any)
	errorKind := errProps["kind"].(map[string]any)
	if _, ok := errorKind["enum"]; !ok {
		t.Fatal("queue_error.kind must be an enum")
	}
}

func enumContains(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(raw))
	}
	return decoded
}
