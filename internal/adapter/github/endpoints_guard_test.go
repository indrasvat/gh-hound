package github

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The workflow-run timing endpoint and the billing/usage endpoints are
// closing down (PRD §20, item 13). Pace derives durations from run
// timestamps alone; no request path may reach those endpoints.
func TestNoTimingOrBillingEndpoints(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)/timing\b|/billing\b|/settings/billing|actions/usage`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if loc := forbidden.FindIndex(body); loc != nil {
			t.Fatalf("%s reaches a closing endpoint: %q", file, body[loc[0]:loc[1]])
		}
	}
}
