package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionOutputIncludesBannerMetadataAndTagline(t *testing.T) {
	var out bytes.Buffer

	cfg := buildInfo{
		Version: "v0.1.0",
		Commit:  "a1b2c3d",
		Date:    "2026-06-07T00:00:00Z",
	}

	if err := printVersion(&out, cfg); err != nil {
		t.Fatalf("printVersion returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"██╗  ██╗ ██████╗ ██╗   ██╗███╗   ██╗██████╗",
		"v0.1.0 · commit a1b2c3d · built 2026-06-07T00:00:00Z",
		"Hunt down your GitHub Actions CI",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q\noutput:\n%s", want, got)
		}
	}
}
