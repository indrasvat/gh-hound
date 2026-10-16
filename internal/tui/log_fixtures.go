package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/indrasvat/gh-hound/internal/logs"
	logscreen "github.com/indrasvat/gh-hound/internal/tui/screens/log"
)

// sampleNoisyLogModel is a folded e2e log: a two-hour readiness poll in a
// step group, a retry block with one rare exit code, then the failure.
func sampleNoisyLogModel() logscreen.Model {
	var b strings.Builder
	t := time.Date(2026, 10, 15, 16, 12, 2, 0, time.UTC)
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, "%s "+format+"\n", append([]any{t.Format("15:04:05Z")}, args...)...)
	}
	fields := "name=e2e-cluster-lifecycle"
	line("##[group]Run make e2e")
	line("INFO Feature: cluster lifecycle %s", fields)
	line("INFO   When I create a cluster with 3 nodes %s", fields)
	for i := range 720 {
		t = t.Add(9 * time.Second)
		line("INFO checking if cluster is running %s", fields)
		t = t.Add(time.Second)
		line("INFO state :: &{ 0x38fa%08x 0x38fa%08x } %s", uint32(i*2654435761), uint32(i*40503), fields)
	}
	t = t.Add(2 * time.Second)
	line("ERRO timed out after 2h0m0s waiting for cluster %s", fields)
	line("##[endgroup]")
	for i := range 12 {
		code := 0
		if i == 7 {
			code = 137
		}
		line("attempt %d/12: kubectl get nodes", i+1)
		line("  -> probe finished with exit code %d", code)
		t = t.Add(5 * time.Second)
	}
	line("--- FAIL: TestFeatures/cluster_lifecycle (7205.31s)")
	line("##[error]Process completed with exit code 1.")
	return logscreen.Fold(context.Background(), logscreen.NewModel(logs.Parse(b.String()), 1, 6))
}
