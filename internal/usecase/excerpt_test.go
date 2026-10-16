package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/logs"
)

func TestExcerptFoldsAPollThatFillsTheTail(t *testing.T) {
	var b strings.Builder
	b.WriteString("2026-10-15T16:12:02.0000000Z Scenario: create a cluster\n")
	for i := range 400 {
		fmt.Fprintf(&b, "2026-10-15T16:%02d:%02d.0000000Z checking if cluster is running\n", 12+i/60, i%60)
	}
	doc := logs.Parse(b.String())
	if doc.Failure.Found {
		t.Fatal("fixture should use the tail fallback")
	}
	got, repeats := excerpt(context.Background(), doc)
	lines := strings.Split(got, "\n")
	if lines[0] != "Scenario: create a cluster" {
		t.Fatalf("excerpt should reach back to the context before the poll:\n%s", got)
	}
	if !strings.Contains(got, "⋯ ×400") || !strings.Contains(got, "lines hidden (L3–L400)") {
		t.Fatalf("missing fold marker:\n%s", got)
	}
	if len(lines) > excerptTailLines {
		t.Fatalf("excerpt grew to %d rows", len(lines))
	}
	if len(repeats) != 1 || repeats[0].Cycles != 400 || repeats[0].StartLine != 3 || repeats[0].Hidden != 398 || repeats[0].From != "16:12:00" {
		t.Fatalf("repeats = %+v", repeats)
	}
}

func TestExcerptUnchangedWithoutRepeats(t *testing.T) {
	raw := "setup\nbuild a\nbuild b\n--- FAIL: TestX (0.00s)\n##[error]Process completed with exit code 1."
	got, repeats := excerpt(context.Background(), logs.Parse(raw))
	if repeats != nil || !strings.HasPrefix(got, "setup") || !strings.HasSuffix(got, "exit code 1.") {
		t.Fatalf("excerpt=%q repeats=%v", got, repeats)
	}
}

func TestExcerptKeepsFailureWindowWhenRetriesRepeatInsideIt(t *testing.T) {
	var b strings.Builder
	b.WriteString("download deps\n")
	for i := range 50 {
		fmt.Fprintf(&b, "attempt %d: connect: connection refused\n", i+1)
	}
	b.WriteString("##[error]Process completed with exit code 1.\n")
	got, _ := excerpt(context.Background(), logs.Parse(b.String()))
	if !strings.Contains(got, "##[error]Process completed with exit code 1.") || !strings.Contains(got, "⋯ ") {
		t.Fatalf("excerpt:\n%s", got)
	}
}

func TestExcerptStopsAtTheFailureWindow(t *testing.T) {
	var b strings.Builder
	for i := range 100 {
		fmt.Fprintf(&b, "setup step %d\n", i)
	}
	b.WriteString("error: failure\n")
	for range 100 {
		b.WriteString("cleanup tick\n")
	}
	doc := logs.Parse(b.String())
	text, _ := excerpt(context.Background(), doc)
	last := doc.Failure.EndLine
	if !doc.Failure.Found || last >= 200 {
		t.Fatalf("fixture: failure window %+v", doc.Failure)
	}
	for _, m := range regexp.MustCompile(`–L(\d+)\)`).FindAllStringSubmatch(text, -1) {
		if n, _ := strconv.Atoi(m[1]); n > last {
			t.Fatalf("excerpt summarizes past L%d:\n%s", last, text)
		}
	}
	if !strings.Contains(text, "error: failure") {
		t.Fatalf("excerpt lost the failure:\n%s", text)
	}
}
