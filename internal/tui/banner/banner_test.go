package banner

import (
	"strings"
	"testing"
)

func TestRenderVersionBannerMatchesCLIContract(t *testing.T) {
	got := RenderVersion(BuildInfo{
		Version: "v0.1.0",
		Commit:  "a1b2c3d",
		Date:    "2026-06-07T00:00:00Z",
	})
	for _, want := range []string{
		"██╗  ██╗ ██████╗ ██╗   ██╗███╗   ██╗██████╗",
		"v0.1.0 · commit a1b2c3d · built 2026-06-07T00:00:00Z",
		"Hunt down your GitHub Actions CI",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("banner missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "\ufe0f") {
		t.Fatalf("banner contains emoji variation selector")
	}
}
