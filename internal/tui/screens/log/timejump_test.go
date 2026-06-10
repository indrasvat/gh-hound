package log

import (
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func timeDoc() logs.Document {
	return logs.Parse("2026-06-10T15:52:00.0000000Z building\n2026-06-10T15:53:14.2803225Z non-200 OK status code: 401\n2026-06-10T15:54:00.0000000Z done")
}

func TestTimestampJumpModal(t *testing.T) {
	m := NewModel(timeDoc(), 1, 6).JumpTo("15:53")
	if m.Offset != 2 {
		t.Fatalf("jump to first line at/after 15:53 should set offset 2, got %d", m.Offset)
	}
	if m.LastJump != "15:53" {
		t.Fatalf("breadcrumb must record the jump: %q", m.LastJump)
	}
}

func TestTimestampJumpBareClockFormat(t *testing.T) {
	doc := logs.Parse("17:42:53.114Z go test ./... -race\n17:43:10.000Z --- FAIL: TestX\n17:44:00.000Z FAIL")
	if m := NewModel(doc, 1, 6).JumpTo("17:43"); m.Offset != 2 {
		t.Fatalf("bare-clock logs must jump too, got offset %d", m.Offset)
	}
}

func TestSearchStillWorksAlongsideJump(t *testing.T) {
	m := NewModel(timeDoc(), 1, 6)
	m = m.Update(KeyMsg{Key: "/"})
	for _, key := range []string{"4", "0", "1"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Search.Total != 1 {
		t.Fatalf("search must work: %d matches", m.Search.Total)
	}
}

func TestTimestampJumpHandlesMidnightWrap(t *testing.T) {
	doc := logs.Parse("23:59:50.000Z winding down\n23:59:58.000Z almost\n00:01:00.000Z next day work\n00:02:00.000Z more")
	m := NewModel(doc, 1, 6).JumpTo("00:01")
	if m.Offset != 3 {
		t.Fatalf("00:01 must land on the post-midnight line, got offset %d", m.Offset)
	}
}

func TestTimestampJumpAfterLastWrapSpanIsNoOp(t *testing.T) {
	doc := logs.Parse("23:59:50.000Z winding down\n00:01:00.000Z next day\n00:02:00.000Z more")
	m := NewModel(doc, 1, 6)
	m.Offset = 2
	m = m.JumpTo("00:03")
	if m.Offset != 2 || m.LastJump != "" {
		t.Fatalf("query past the final span must not jump: offset=%d lastJump=%q", m.Offset, m.LastJump)
	}
}

func TestRangeFilterLimitsVisibleRows(t *testing.T) {
	m := NewModel(timeDoc(), 1, 6)
	m = m.SetRange(2, 2, "15:53-15:53")
	rows := m.VisibleRowNumbers()
	if len(rows) != 1 || rows[0] != 2 {
		t.Fatalf("range must limit rows to [2], got %v", rows)
	}
	if m.RangeLabel == "" {
		t.Fatal("range label must be set for the header")
	}
	m = m.Update(KeyMsg{Key: "esc"})
	if m.RangeLabel != "" {
		t.Fatal("esc must clear the range first")
	}
	if rows := m.VisibleRowNumbers(); len(rows) != 2 || rows[0] != 2 {
		t.Fatalf("cleared range keeps position and restores subsequent rows, got %v", rows)
	}
}

func TestJumpRelativeMovesByDelta(t *testing.T) {
	m := NewModel(timeDoc(), 1, 6)
	m = m.JumpRelative(70) // 15:52:00 + 70s -> first line >= 15:53:10 is 15:53:14 (line 2)
	if m.Offset != 2 {
		t.Fatalf("relative jump +70s should land on line 2, got %d", m.Offset)
	}
	m = m.JumpRelative(-3600)
	if m.Offset != 1 {
		t.Fatalf("relative jump past start clamps to first line, got %d", m.Offset)
	}
}

func TestGReachesTheLastLine(t *testing.T) {
	doc := logs.Parse("00:01:00Z l1\n00:02:00Z l2\n00:03:00Z l3\n00:04:00Z l4\n00:05:00Z l5")
	m := NewModel(doc, 1, 3) // 3 body rows
	m = m.Update(KeyMsg{Key: "G"})
	rows := m.VisibleRowNumbers()
	if rows[len(rows)-1] != 5 {
		t.Fatalf("G must reach the final line, got rows %v", rows)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if got := m.VisibleRowNumbers(); got[len(got)-1] != 5 {
		t.Fatalf("j past end must not overscroll, got rows %v", got)
	}
}

func TestScrollClampsInsideActiveRange(t *testing.T) {
	doc := logs.Parse("00:01:00Z l1\n00:02:00Z l2\n00:03:00Z l3\n00:04:00Z l4\n00:05:00Z l5")
	m := NewModel(doc, 1, 2)
	m = m.SetRange(2, 4, "00:02-00:04")
	m = m.Update(KeyMsg{Key: "G"})
	rows := m.VisibleRowNumbers()
	if len(rows) == 0 || rows[len(rows)-1] != 4 || rows[0] < 2 {
		t.Fatalf("G inside a range must land on the range end, got %v", rows)
	}
	m = m.Update(KeyMsg{Key: "j"})
	if rows := m.VisibleRowNumbers(); len(rows) == 0 || rows[len(rows)-1] != 4 {
		t.Fatalf("j past the range end must not blank the pane, got %v", rows)
	}
	m = m.Update(KeyMsg{Key: "g"})
	if rows := m.VisibleRowNumbers(); rows[0] != 2 {
		t.Fatalf("g inside a range must land on the range start, got %v", rows)
	}
	m = m.Update(KeyMsg{Key: "k"})
	if rows := m.VisibleRowNumbers(); rows[0] != 2 {
		t.Fatalf("k above the range start must clamp, got %v", rows)
	}
}
