package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestMutationEndpointsUseExpectedMethodPathAndBodies(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-GitHub-Api-Version") != APIVersion {
			t.Fatalf("missing api version header")
		}
		switch r.URL.Path {
		case "/repos/indrasvat/gh-hound/actions/runs/571/rerun":
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rerun body: %v", err)
			}
			if !body["enable_debug_logging"] {
				t.Fatalf("rerun body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/indrasvat/gh-hound/actions/runs/571/rerun-failed-jobs":
			// Live-verified 2026-06-10: this endpoint accepts the debug
			// body on API v2026-03-10 (201 on run 27245877203).
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rerun-failed body: %v", err)
			}
			if !body["enable_debug_logging"] {
				t.Fatalf("rerun-failed body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/indrasvat/gh-hound/actions/jobs/399/rerun":
			// Live-verified 2026-06-10: job rerun accepts the debug body
			// (201 on job 80701207312) — pin debug=true here so the
			// "--debug combines with all rerun forms" claim has teeth.
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode job rerun body: %v", err)
			}
			if !body["enable_debug_logging"] {
				t.Fatalf("job rerun body = %#v, want debug true", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/indrasvat/gh-hound/actions/runs/571/cancel",
			"/repos/indrasvat/gh-hound/actions/runs/571/force-cancel":
			w.WriteHeader(http.StatusAccepted)
		case "/repos/indrasvat/gh-hound/actions/workflows/release.yml/dispatches":
			var body usecase.DispatchRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode dispatch body: %v", err)
			}
			if body.Ref != "main" || body.Inputs["version"] != "v0.12.0" {
				t.Fatalf("dispatch body = %#v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected mutation path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ctx := context.Background()
	calls := []func() error{
		func() error { _, err := client.RerunRun(ctx, "indrasvat/gh-hound", 571, true); return err },
		func() error { _, err := client.RerunFailedJobs(ctx, "indrasvat/gh-hound", 571, true); return err },
		func() error { _, err := client.RerunJob(ctx, "indrasvat/gh-hound", 399, true); return err },
		func() error { _, err := client.CancelRun(ctx, "indrasvat/gh-hound", 571); return err },
		func() error { _, err := client.ForceCancelRun(ctx, "indrasvat/gh-hound", 571); return err },
		func() error {
			_, err := client.DispatchWorkflow(ctx, "indrasvat/gh-hound", "release.yml", usecase.DispatchRequest{
				Ref: "main",
				Inputs: map[string]string{
					"version": "v0.12.0",
				},
			})
			return err
		},
	}
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("mutation returned error: %v", err)
		}
	}
	if len(seen) != 6 {
		t.Fatalf("mutation calls = %#v", seen)
	}
}

func TestMutationErrorsAreTyped(t *testing.T) {
	tests := []struct {
		status int
		want   usecase.ActionErrorKind
	}{
		{http.StatusForbidden, usecase.ActionErrorPermission},
		{http.StatusConflict, usecase.ActionErrorConflict},
		{http.StatusTooManyRequests, usecase.ActionErrorRateLimit},
	}
	for _, tt := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", tt.status)
		}))
		client := NewClient(server.URL, server.Client())
		_, err := client.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
		server.Close()
		actionErr, ok := usecase.AsActionError(err)
		if !ok || actionErr.Kind != tt.want {
			t.Fatalf("status %d error = %#v, want kind %s", tt.status, err, tt.want)
		}
	}
}

func TestMutationRateLimitErrorCarriesRetryMetadata(t *testing.T) {
	resetAt := time.Unix(1781033060, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "17")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
	actionErr, ok := usecase.AsActionError(err)
	if !ok {
		t.Fatalf("CancelRun err = %#v, want action error", err)
	}
	if actionErr.Kind != usecase.ActionErrorRateLimit || actionErr.RetryAfter != 17*time.Second || !actionErr.ResetAt.Equal(resetAt) {
		t.Fatalf("rate limit action error = %#v", actionErr)
	}
}

func TestSecondaryRateLimit403MapsToRateLimit(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		body    string
		want    usecase.ActionErrorKind
	}{
		{"retry-after header", map[string]string{"Retry-After": "30"}, "slow down", usecase.ActionErrorRateLimit},
		{"secondary message", nil, "You have exceeded a secondary rate limit", usecase.ActionErrorRateLimit},
		{"plain permission", nil, "Resource not accessible by integration", usecase.ActionErrorPermission},
	}
	for _, tt := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range tt.headers {
				w.Header().Set(k, v)
			}
			http.Error(w, tt.body, http.StatusForbidden)
		}))
		client := NewClient(server.URL, server.Client())
		_, err := client.CancelRun(context.Background(), "indrasvat/gh-hound", 571)
		server.Close()
		actionErr, ok := usecase.AsActionError(err)
		if !ok || actionErr.Kind != tt.want {
			t.Fatalf("%s: kind = %#v, want %s", tt.name, err, tt.want)
		}
	}
}

// Live-verified 2026-06-10: list-workflows carries `state` for every
// workflow (indrasvat/gh-hound all active; indrasvat/vicaya holds a
// real disabled_inactivity — pinned in testdata/workflows.json). The
// enable/disable endpoints are documented PUTs with empty bodies and
// 204 responses; mutation PUTs were NOT fired live in this session
// (supervised round-trip happens at orchestrator level).
func TestWorkflowToggleEndpointsUsePutWithEscapedIdentifier(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.EscapedPath())
		if r.Header.Get("X-GitHub-Api-Version") != APIVersion {
			t.Fatalf("missing api version header")
		}
		if r.Method != http.MethodPut {
			t.Fatalf("workflow toggle method = %s, want PUT", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if len(body) != 0 {
			t.Fatalf("workflow toggle body = %q, want empty", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	ctx := context.Background()
	enabled, err := client.EnableWorkflow(ctx, "indrasvat/gh-hound", "ci.yml")
	if err != nil {
		t.Fatalf("EnableWorkflow: %v", err)
	}
	if enabled.Action != usecase.ActionEnableWorkflow || enabled.WorkflowID != "ci.yml" {
		t.Fatalf("enable result = %#v", enabled)
	}
	disabled, err := client.DisableWorkflow(ctx, "indrasvat/gh-hound", "290736476")
	if err != nil {
		t.Fatalf("DisableWorkflow: %v", err)
	}
	if disabled.Action != usecase.ActionDisableWorkflow {
		t.Fatalf("disable result = %#v", disabled)
	}
	// The full workflow file path is a valid selector too (verified live
	// via GET with %2F escaping); slashes must be escaped, not routed.
	if _, err := client.DisableWorkflow(ctx, "indrasvat/gh-hound", ".github/workflows/ci.yml"); err != nil {
		t.Fatalf("DisableWorkflow(path): %v", err)
	}
	want := []string{
		"PUT /repos/indrasvat/gh-hound/actions/workflows/ci.yml/enable",
		"PUT /repos/indrasvat/gh-hound/actions/workflows/290736476/disable",
		"PUT /repos/indrasvat/gh-hound/actions/workflows/.github%2Fworkflows%2Fci.yml/disable",
	}
	if len(seen) != len(want) {
		t.Fatalf("calls = %#v, want %#v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("call %d = %q, want %q", i, seen[i], want[i])
		}
	}
}

func TestWorkflowToggleErrorsAreTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.EnableWorkflow(context.Background(), "indrasvat/gh-hound", "ci.yml")
	actionErr, ok := usecase.AsActionError(err)
	if !ok {
		t.Fatalf("error %v is not an ActionError", err)
	}
	if actionErr.Kind != usecase.ActionErrorPermission {
		t.Fatalf("kind = %q, want permission", actionErr.Kind)
	}
}
