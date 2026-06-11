package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

// The GET shape below is pinned from the official REST reference
// (docs.github.com/en/rest/actions/workflow-runs, API 2022-11-28
// example payload, fetched 2026-06-10). The live endpoint was verified
// the same day against indrasvat/gh-hound run 27319423642: 200 with []
// on a non-waiting run. Live POST verification is deferred to the PR's
// live-verification phase (scratch gated environment).
func TestListPendingDeploymentsMapsDocsPinnedPayload(t *testing.T) {
	payload, err := os.ReadFile("testdata/pending_deployments.json")
	if err != nil {
		t.Fatalf("read pinned payload: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/runs/571/pending_deployments" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-GitHub-Api-Version") != APIVersion {
			t.Fatal("missing api version header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	pending, err := client.ListPendingDeployments(context.Background(), "indrasvat/gh-hound", 571)
	if err != nil {
		t.Fatalf("ListPendingDeployments returned error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d entries, want 1", len(pending))
	}
	got := pending[0]
	if got.EnvironmentID != 161088068 || got.EnvironmentName != "staging" {
		t.Fatalf("environment = %#v", got)
	}
	if got.WaitTimer != 30 {
		t.Fatalf("wait_timer = %d, want 30", got.WaitTimer)
	}
	if !got.CurrentUserCanApprove {
		t.Fatal("current_user_can_approve must map true")
	}
	if len(got.Reviewers) != 2 {
		t.Fatalf("reviewers = %#v", got.Reviewers)
	}
	if got.Reviewers[0].Type != "User" || got.Reviewers[0].Name != "octocat" {
		t.Fatalf("user reviewer = %#v", got.Reviewers[0])
	}
	if got.Reviewers[1].Type != "Team" || got.Reviewers[1].Name != "justice-league" {
		t.Fatalf("team reviewer = %#v", got.Reviewers[1])
	}
}

// POST body pinned from the official docs: environment_ids, state, and
// comment are all required — comment is ALWAYS sent, defaulting to the
// documented gh-hound comment when the caller left it blank.
func TestReviewPendingDeploymentsPostsExactBody(t *testing.T) {
	var rawBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/runs/571/pending_deployments" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		rawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id": 42}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	result, err := client.ReviewPendingDeployments(context.Background(), "indrasvat/gh-hound", 571, usecase.DeploymentReview{
		EnvironmentIDs: []int64{161171787},
		State:          usecase.DeploymentApproved,
		Comment:        "",
	})
	if err != nil {
		t.Fatalf("ReviewPendingDeployments returned error: %v", err)
	}
	if result.Action != usecase.ActionApproveDeployment || result.RunID != 571 {
		t.Fatalf("result = %#v", result)
	}
	var body struct {
		EnvironmentIDs []int64 `json:"environment_ids"`
		State          string  `json:"state"`
		Comment        *string `json:"comment"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("decode body: %v\n%s", err, rawBody)
	}
	if len(body.EnvironmentIDs) != 1 || body.EnvironmentIDs[0] != 161171787 {
		t.Fatalf("environment_ids = %#v", body.EnvironmentIDs)
	}
	if body.State != "approved" {
		t.Fatalf("state = %q", body.State)
	}
	if body.Comment == nil || *body.Comment != usecase.DefaultReviewComment {
		t.Fatalf("comment = %v, want default %q always present", body.Comment, usecase.DefaultReviewComment)
	}
}

func TestReviewPendingDeploymentsSendsRejectedStateAndUserComment(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	result, err := client.ReviewPendingDeployments(context.Background(), "indrasvat/gh-hound", 571, usecase.DeploymentReview{
		EnvironmentIDs: []int64{1, 2},
		State:          usecase.DeploymentRejected,
		Comment:        "not on a friday",
	})
	if err != nil {
		t.Fatalf("ReviewPendingDeployments returned error: %v", err)
	}
	if result.Action != usecase.ActionRejectDeployment {
		t.Fatalf("action = %q", result.Action)
	}
	if body["state"] != "rejected" || body["comment"] != "not on a friday" {
		t.Fatalf("body = %#v", body)
	}
}

func TestReviewPendingDeployments422MapsToValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.ReviewPendingDeployments(context.Background(), "indrasvat/gh-hound", 571, usecase.DeploymentReview{
		EnvironmentIDs: []int64{9},
		State:          usecase.DeploymentApproved,
	})
	actionErr, ok := usecase.AsActionError(err)
	if !ok || actionErr.Kind != usecase.ActionErrorValidation {
		t.Fatalf("422 error = %#v, want validation kind", err)
	}
}
