package layout

import "testing"

func TestBreakpointForTargetGeometries(t *testing.T) {
	tests := []struct {
		width  int
		height int
		want   Breakpoint
	}{
		{80, 24, BreakpointNarrow},
		{120, 40, BreakpointDefault},
		{200, 60, BreakpointWide},
	}
	for _, tt := range tests {
		if got := BreakpointFor(tt.width, tt.height); got != tt.want {
			t.Fatalf("BreakpointFor(%d,%d) = %q, want %q", tt.width, tt.height, got, tt.want)
		}
	}
}

func TestRunsColumnsCollapseByBreakpoint(t *testing.T) {
	if got := RunsColumns(BreakpointNarrow); !equal(got, []string{"status", "workflow", "number", "age"}) {
		t.Fatalf("narrow columns = %#v", got)
	}
	if got := RunsColumns(BreakpointDefault); !equal(got, []string{"status", "workflow", "event", "number", "duration", "age"}) {
		t.Fatalf("default columns = %#v", got)
	}
	if got := RunsColumns(BreakpointWide); !equal(got, []string{"status", "workflow", "event", "actor", "sha", "number", "duration", "age"}) {
		t.Fatalf("wide columns = %#v", got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
