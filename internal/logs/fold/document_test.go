package fold

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func TestForDocumentKeepsFoldsInsideGroups(t *testing.T) {
	var raw []string
	raw = append(raw, "##[group]wait for db")
	for i := range 20 {
		raw = append(raw, fmt.Sprintf("waiting for db (%02d:00:%02d)", 10, i))
	}
	raw = append(raw, "##[endgroup]")
	for i := range 20 {
		raw = append(raw, fmt.Sprintf("waiting for db (%02d:01:%02d)", 10, i))
	}
	doc := logs.Parse(strings.Join(raw, "\n"))
	p, err := ForDocument(context.Background(), doc, Default())
	if err != nil {
		t.Fatal(err)
	}
	group := doc.Folds[0]
	for _, s := range p.Segments {
		if s.Kind != Folded {
			continue
		}
		inside := s.Start >= group.StartLine && s.End <= group.EndLine
		outside := s.Start > group.EndLine
		if !inside && !outside {
			t.Fatalf("fold %d-%d crosses group %d-%d", s.Start+1, s.End, group.StartLine, group.EndLine)
		}
	}
	if hidden := p.HiddenMask(); hidden[group.StartLine-1] || hidden[group.EndLine] {
		t.Fatal("group line or the line after the group was hidden")
	}
	if p.Rows() >= len(doc.Lines) {
		t.Fatalf("nothing folded: rows=%d lines=%d", p.Rows(), len(doc.Lines))
	}
}

func TestForRangePlansAWindow(t *testing.T) {
	var raw []string
	for i := range 60 {
		raw = append(raw, fmt.Sprintf("10:00:%02d waiting", i))
	}
	doc := logs.Parse(strings.Join(raw, "\n"))
	p, err := ForRange(context.Background(), doc, 11, 30, Default())
	if err != nil || len(p.Lines) != 20 || p.Lines[0] != doc.Lines[10].Text {
		t.Fatalf("range plan lines=%d err=%v", len(p.Lines), err)
	}
	if p.Rows() >= 20 {
		t.Fatalf("range not folded: rows=%d", p.Rows())
	}
	if p, _ := ForRange(context.Background(), doc, 50, 40, Default()); len(p.Lines) != 0 {
		t.Fatal("empty range must plan nothing")
	}
}

func TestForDocumentKeepsShortStampedAnnotations(t *testing.T) {
	var raw []string
	for i := range 10 {
		raw = append(raw, fmt.Sprintf("10:00:%02dZ ::error file=a.go,line=%d::flake %d", i, i+1, i))
	}
	doc := logs.Parse(strings.Join(raw, "\n"))
	if len(doc.Annotations) != 10 {
		t.Fatalf("parser found %d annotations, want 10", len(doc.Annotations))
	}
	p, err := ForDocument(context.Background(), doc, Default())
	if err != nil {
		t.Fatal(err)
	}
	for i, hidden := range p.HiddenMask() {
		if hidden {
			t.Fatalf("annotation line %d hidden: %q", i+1, raw[i])
		}
	}
	if !(Structural{}).Protect(raw[0]) {
		t.Fatalf("structural guard misses %q", raw[0])
	}
}
