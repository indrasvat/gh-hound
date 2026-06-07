package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestFetchJobLogRefetchesWhenRedirectExpired(t *testing.T) {
	var logEndpointCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/indrasvat/gh-hound/actions/jobs/399444496/logs":
			call := logEndpointCalls.Add(1)
			if call == 1 {
				http.Redirect(w, r, "/expired", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/fresh", http.StatusFound)
		case "/expired":
			http.Error(w, "expired", http.StatusNotFound)
		case "/fresh":
			_, _ = w.Write([]byte("##[error]Process completed with exit code 1\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	logText, err := client.FetchJobLog(context.Background(), "indrasvat/gh-hound", 399444496)
	if err != nil {
		t.Fatalf("FetchJobLog returned error: %v", err)
	}
	if logText != "##[error]Process completed with exit code 1\n" {
		t.Fatalf("log = %q", logText)
	}
	if got := logEndpointCalls.Load(); got != 2 {
		t.Fatalf("log endpoint calls = %s, want 2", strconv.FormatInt(got, 10))
	}
}
