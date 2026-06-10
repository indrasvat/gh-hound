package log

import (
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func timeDoc() logs.Document {
	return logs.Parse("2026-06-10T15:52:00.0000000Z building\n2026-06-10T15:53:14.2803225Z non-200 OK status code: 401\n2026-06-10T15:54:00.0000000Z done")
}

func TestTimestampJumpModal(t *testing.T) {
	m := NewModel(timeDoc(), 1, 6)
	m = m.Update(KeyMsg{Key: "t"})
	if !m.InputMode {
		t.Fatal("t must open the time-input modal")
	}
	for _, key := range []string{"1", "5", ":", "5", "3"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.InputMode {
		t.Fatal("enter must close the modal")
	}
	if m.Offset != 2 {
		t.Fatalf("jump to first line at/after 15:53 should set offset 2, got %d", m.Offset)
	}
}

func TestTimestampJumpBareClockFormat(t *testing.T) {
	doc := logs.Parse("17:42:53.114Z go test ./... -race\n17:43:10.000Z --- FAIL: TestX\n17:44:00.000Z FAIL")
	m := NewModel(doc, 1, 6)
	m = m.Update(KeyMsg{Key: "t"})
	for _, key := range []string{"1", "7", ":", "4", "3"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Offset != 2 {
		t.Fatalf("bare-clock logs must jump too, got offset %d", m.Offset)
	}
}

func TestTimestampJumpEscCancels(t *testing.T) {
	m := NewModel(timeDoc(), 1, 6)
	m = m.Update(KeyMsg{Key: "t"})
	m = m.Update(KeyMsg{Key: "esc"})
	if m.InputMode || m.Offset != 1 {
		t.Fatalf("esc must cancel without jumping: input=%v offset=%d", m.InputMode, m.Offset)
	}
	// t must not break / search afterwards
	m = m.Update(KeyMsg{Key: "/"})
	for _, key := range []string{"4", "0", "1"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Search.Total != 1 {
		t.Fatalf("search must still work after a cancelled time jump: %d matches", m.Search.Total)
	}
}

func TestTimestampJumpHandlesMidnightWrap(t *testing.T) {
	doc := logs.Parse("23:59:50.000Z winding down\n23:59:58.000Z almost\n00:01:00.000Z next day work\n00:02:00.000Z more")
	m := NewModel(doc, 1, 6)
	m = m.Update(KeyMsg{Key: "t"})
	for _, key := range []string{"0", "0", ":", "0", "1"} {
		m = m.Update(KeyMsg{Key: key})
	}
	m = m.Update(KeyMsg{Key: "enter"})
	if m.Offset != 3 {
		t.Fatalf("00:01 must land on the post-midnight line, got offset %d", m.Offset)
	}
}
