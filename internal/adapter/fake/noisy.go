package fake

import (
	"fmt"
	"strings"
	"time"
)

// noisyLog is a long job log dominated by repeats: a two-hour readiness
// poll inside a step group, a retry block with one rare exit code, two
// interleaved workers, a reconnect loop of error lines, then the failure.
func noisyLog() string {
	var b strings.Builder
	t := time.Date(2026, 10, 15, 16, 12, 2, 0, time.UTC)
	stamp := func() string { return t.Format("2006-01-02T15:04:05.0000000Z") }
	clock := func() string { return t.Format("2006-01-02 15:04:05") }
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, "%s "+format+"\n", append([]any{stamp()}, args...)...)
	}
	fields := "name=e2e-cluster-lifecycle scenario=lifecycle"
	line("##[group]Run make e2e")
	line("INFO[%s] Feature: cluster lifecycle                     %s", clock(), fields)
	line("INFO[%s]   Scenario: create, scale and delete a cluster  %s", clock(), fields)
	line("INFO[%s]     When I create a cluster with 3 nodes        %s", clock(), fields)
	ptr := uint64(0x38fa20000000)
	for i := range 720 {
		t = t.Add(9 * time.Second)
		line("INFO[%s] checking if cluster is running             %s", clock(), fields)
		t = t.Add(time.Second)
		ptr = ptr*6364136223846793005 + 1442695040888963407
		line("INFO[%s] state :: &{  0x%012x 0x%012x 0x%012x}  %s", clock(), 0x38fa20000000+ptr%0xfffffff, 0x38fa20000000+(ptr>>8)%0xfffffff, 0x38fa20000000+(ptr>>16)%0xfffffff, fields)
		_ = i
	}
	t = t.Add(2 * time.Second)
	line("ERRO[%s] timed out after 2h0m0s waiting for cluster to be running %s", clock(), fields)
	line("##[endgroup]")
	line("##[group]Collect diagnostics")
	for i := range 12 {
		code := 0
		if i == 7 {
			code = 137
		}
		line("attempt %d/12: kubectl get nodes", i+1)
		line("  -> probe finished with exit code %d", code)
		line("  response time: %dms", 40+(i*37)%300)
		line("  request-id: %08x-%04x", 0x7093e1ae+i*7919, 0x0188+i)
		line("  sleeping %ds before retry", 1+i%5)
		t = t.Add(5 * time.Second)
	}
	for i := range 60 {
		if (i*7)%3 == 0 {
			line("[worker-a] queue empty, sleeping")
		} else {
			line("[worker-b] lease held by w-%06x", 0xa3f9c1+i*977)
		}
		t = t.Add(time.Second)
	}
	for i := range 18 {
		line("WARN[%s] db reconnect %d/18 failed: dial tcp 10.0.3.7:5432: connection refused", clock(), i+1)
		t = t.Add(2 * time.Second)
	}
	line("##[endgroup]")
	line("--- FAIL: TestFeatures/cluster_lifecycle (7205.31s)")
	line("FAIL")
	line("##[error]Process completed with exit code 1.")
	return b.String()
}
