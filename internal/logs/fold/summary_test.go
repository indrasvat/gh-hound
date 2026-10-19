package fold

import (
	"strings"
	"testing"
)

func TestMarkerGroupsTheHiddenCount(t *testing.T) {
	if got := (Summary{Hidden: 1436, FirstLine: 7, LastLine: 1442}).Marker(); !strings.Contains(got, "· 1,436 lines hidden (L7–L1442)") {
		t.Fatalf("marker = %q", got)
	}
}
