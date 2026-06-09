package usecase

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestResilienceForTaxonomyRows(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantClass ErrorClass
		wantTitle string
		wantSev   Severity
		wantRetry string
	}{
		{
			name:      "api rate limit",
			err:       ActionError{Kind: ActionErrorRateLimit, Status: 429, Message: "backing off, retry in 42s"},
			wantClass: ErrorClassRateLimit,
			wantTitle: "GitHub API · 429",
			wantSev:   SeverityError,
			wantRetry: "auto_resume",
		},
		{
			name:      "network",
			err:       &net.DNSError{Err: "no such host", Name: "api.github.com"},
			wantClass: ErrorClassNetwork,
			wantTitle: "Runs unavailable",
			wantSev:   SeverityWarn,
			wantRetry: "retry",
		},
		{
			name:      "log render",
			err:       LogRenderError{Message: "link expired"},
			wantClass: ErrorClassLogRender,
			wantTitle: "Log render failed",
			wantSev:   SeverityError,
			wantRetry: "refetch_log",
		},
		{
			name:      "mutation rejected",
			err:       ActionError{Kind: ActionErrorConflict, Status: 409, Message: "run already completed"},
			wantClass: ErrorClassMutationRejected,
			wantTitle: "Mutation rejected",
			wantSev:   SeverityError,
			wantRetry: "retry",
		},
		{
			name:      "read permission",
			err:       APIError{Kind: APIErrorPermission, Status: 403, Message: "Resource not accessible by personal access token"},
			wantClass: ErrorClassAccessDenied,
			wantTitle: "GitHub access denied",
			wantSev:   SeverityError,
			wantRetry: "reauth",
		},
	}
	for _, tt := range tests {
		got := ResilienceFor(tt.err, ErrorContext{CachedAge: 3 * time.Minute})
		if got.Class != tt.wantClass || got.Title != tt.wantTitle || got.Severity != tt.wantSev || got.RetryAction != tt.wantRetry {
			t.Fatalf("%s resilience = %#v", tt.name, got)
		}
		if !got.KeepCachedView {
			t.Fatalf("%s should keep cached view", tt.name)
		}
	}
}

func TestSuccessToastForActionResult(t *testing.T) {
	got := ResilienceForSuccess(ActionResult{
		Action:  ActionRerunFailedJobs,
		RunID:   572,
		Message: "Re-run queued",
	})
	if got.Class != ErrorClassSuccess || got.Severity != SeverityOK || got.Title != "Re-run queued" {
		t.Fatalf("success resilience = %#v", got)
	}
	if !strings.Contains(got.Message, "572") {
		t.Fatalf("success message should include run id: %#v", got)
	}
}

func TestUnknownErrorFallsBackToNetworkWarn(t *testing.T) {
	got := ResilienceFor(errors.New("dial tcp: i/o timeout"), ErrorContext{})
	if got.Class != ErrorClassNetwork || got.Severity != SeverityWarn {
		t.Fatalf("unknown error = %#v", got)
	}
}
