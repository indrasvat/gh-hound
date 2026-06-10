package timejump

import (
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func sampleDoc() logs.Document {
	return logs.Parse(strings.Join([]string{
		"17:42:00.000Z ##[group]Set up job",
		"17:42:01.000Z runner ready",
		"17:42:02.000Z ##[endgroup]",
		"17:42:03.000Z ##[group]go test ./...",
		"17:42:53.000Z === RUN TestX",
		"17:44:10.000Z --- FAIL: TestX",
		"17:44:11.000Z ##[error]Process completed with exit code 1.",
		"17:44:12.000Z ##[endgroup]",
	}, "\n"))
}

func TestEntriesIncludeStepsFailureAndGaps(t *testing.T) {
	m := New(sampleDoc())
	var kinds []EntryKind
	for _, entry := range m.Entries {
		kinds = append(kinds, entry.Kind)
	}
	var joined strings.Builder
	for _, entry := range m.Entries {
		joined.WriteString(entry.Label + "|")
	}
	if !strings.Contains(joined.String(), "go test ./...") {
		t.Fatalf("step entries missing: %s", joined.String())
	}
	hasFailure, hasGap := false, false
	for _, kind := range kinds {
		if kind == EntryFailure {
			hasFailure = true
		}
		if kind == EntryGap {
			hasGap = true
		}
	}
	if !hasFailure {
		t.Fatalf("failure entry missing: %v", kinds)
	}
	if !hasGap {
		t.Fatalf("gap entry missing (77s gap exists): %v", kinds)
	}
}

func TestPickerSelectionCommitsJump(t *testing.T) {
	m := New(sampleDoc())
	m = m.Update("j")
	_, action := m.Commit()
	if action.Kind != ActionJump || action.Line == 0 {
		t.Fatalf("picker enter must jump to the selected entry: %#v", action)
	}
}

func TestTypedAbsoluteCommit(t *testing.T) {
	m := New(sampleDoc())
	for _, key := range []string{"1", "7", ":", "4", "4"} {
		m = m.Update(key)
	}
	_, action := m.Commit()
	if action.Kind != ActionJump || action.Line != 5 {
		t.Fatalf("17:44 must jump to line 5: %#v", action)
	}
}

func TestTypedRelativeCommit(t *testing.T) {
	m := New(sampleDoc())
	for _, key := range []string{"+", "3", "0", "s"} {
		m = m.Update(key)
	}
	_, action := m.Commit()
	if action.Kind != ActionRelative || action.DeltaSeconds != 30 {
		t.Fatalf("+30s must produce a relative action: %#v", action)
	}
	m2 := New(sampleDoc())
	for _, key := range []string{"-", "2", "m"} {
		m2 = m2.Update(key)
	}
	_, action = m2.Commit()
	if action.Kind != ActionRelative || action.DeltaSeconds != -120 {
		t.Fatalf("-2m must be -120s: %#v", action)
	}
}

func TestTypedRangeCommit(t *testing.T) {
	m := New(sampleDoc())
	for _, key := range []string{"1", "7", ":", "4", "2", "-", "1", "7", ":", "4", "3"} {
		m = m.Update(key)
	}
	_, action := m.Commit()
	if action.Kind != ActionRange {
		t.Fatalf("A-B must produce a range action: %#v", action)
	}
	if action.Line != 1 || action.EndLine != 4 {
		t.Fatalf("range 17:42-17:43 spans lines 1-4: %#v", action)
	}
}

func TestInvalidInputGivesFeedback(t *testing.T) {
	m := New(sampleDoc())
	for _, key := range []string{"9", "9", ":", "9", "9"} {
		m = m.Update(key)
	}
	next, action := m.Commit()
	if action.Kind != ActionInvalid {
		t.Fatalf("nonsense query must be invalid: %#v", action)
	}
	if next.Feedback == "" {
		t.Fatal("invalid input must set visible feedback")
	}
}

func TestViewRendersPickerAndInput(t *testing.T) {
	m := New(sampleDoc())
	view := View(m, 100)
	for _, want := range []string{"t→▌", "go test ./...", "gap"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestRangeDoesNotLeakAcrossMidnight(t *testing.T) {
	doc := logs.Parse(strings.Join([]string{
		"10:00:00.000Z day one work",
		"10:05:00.000Z day one more",
		"23:59:00.000Z day one end",
		"00:01:00.000Z day two start",
		"10:02:00.000Z day two mid",
	}, "\n"))
	m := New(doc)
	for _, key := range []string{"1", "0", ":", "0", "0", "-", "1", "0", ":", "0", "5"} {
		m = m.Update(key)
	}
	_, action := m.Commit()
	if action.Kind != ActionRange {
		t.Fatalf("expected range action: %#v", action)
	}
	if action.EndLine != 2 {
		t.Fatalf("range 10:00-10:05 must end at line 2 (day one), not leak into day two: %#v", action)
	}
}

func TestRangeCrossingMidnight(t *testing.T) {
	doc := logs.Parse(strings.Join([]string{
		"23:58:00.000Z before",
		"23:59:00.000Z almost",
		"00:01:00.000Z after wrap",
		"00:30:00.000Z later",
	}, "\n"))
	m := New(doc)
	for _, key := range []string{"2", "3", ":", "5", "9", "-", "0", "0", ":", "0", "5"} {
		m = m.Update(key)
	}
	_, action := m.Commit()
	if action.Kind != ActionRange || action.Line != 2 || action.EndLine != 3 {
		t.Fatalf("23:59-00:05 must span the wrap (lines 2-3): %#v", action)
	}
}
