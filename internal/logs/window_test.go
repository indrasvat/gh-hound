package logs

import (
	"strings"
	"testing"
)

func TestFailureWindowStopsAtTerminalError(t *testing.T) {
	raw := strings.Join([]string{
		"2026-06-10T15:53:10.0000000Z building binaries",
		"2026-06-10T15:53:14.2803225Z non-200 OK status code: 401 Unauthorized body: \"{}\"",
		"2026-06-10T15:53:14.2824610Z creating release and uploading assets...",
		"2026-06-10T15:53:15.1241367Z non-200 OK status code: 401 Unauthorized body: \"{}\"",
		"2026-06-10T15:53:15.1281282Z ##[error]Process completed with exit code 1.",
		"2026-06-10T15:53:15.1485046Z Post job cleanup.",
		"2026-06-10T15:53:15.2560184Z git version 2.54.0",
		"2026-06-10T15:53:15.2637556Z Temporarily overriding HOME",
	}, "\n")
	doc := Parse(raw)
	if !doc.Failure.Found {
		t.Fatal("failure window not found")
	}
	var joined strings.Builder
	for _, line := range doc.Failure.Lines {
		joined.WriteString(line.Text + "\n")
	}
	if !strings.Contains(joined.String(), "401 Unauthorized") || !strings.Contains(joined.String(), "exit code 1") {
		t.Fatalf("window must keep the diagnostic:\n%s", joined.String())
	}
	if strings.Contains(joined.String(), "Post job cleanup") || strings.Contains(joined.String(), "git version") {
		t.Fatalf("window must stop at the terminal error line:\n%s", joined.String())
	}
}

func TestStripTimestampPrefix(t *testing.T) {
	in := "2026-06-10T15:53:15.1281282Z ##[error]Process completed with exit code 1."
	if got := StripTimestamp(in); got != "##[error]Process completed with exit code 1." {
		t.Fatalf("StripTimestamp = %q", got)
	}
	if got := StripTimestamp("no timestamp here"); got != "no timestamp here" {
		t.Fatalf("non-timestamped lines pass through: %q", got)
	}
}
