package sparkline

import "testing"

func TestRenderScalesValuesIntoStableWidth(t *testing.T) {
	got := Render([]int{1, 3, 9, 5, 2}, 5)
	if got != "▁▃█▄▂" {
		t.Fatalf("sparkline = %q", got)
	}
	if empty := Render(nil, 5); empty != "·····" {
		t.Fatalf("empty sparkline = %q", empty)
	}
}
