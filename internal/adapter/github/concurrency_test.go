package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

var _ usecase.GitHubConcurrency = (*Client)(nil)

func TestListConcurrencyGroupsPaginatesWithCursorAndMapsSummaries(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/indrasvat/gh-hound/actions/concurrency_groups" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != APIVersion {
			t.Fatalf("api version = %q", got)
		}
		switch requests {
		case 1:
			if r.URL.Query().Get("per_page") != "100" || r.URL.Query().Get("after") != "seed" {
				t.Fatalf("first query = %v", r.URL.Query())
			}
			w.Header().Set("Link", `<`+serverURL(r)+`/repos/indrasvat/gh-hound/actions/concurrency_groups?after=next-cursor>; rel="next"`)
			_, _ = fmt.Fprint(w, `{"total_count":2,"concurrency_groups":[{"group_name":"deploy/prod","group_url":"https://api.github.com/repos/indrasvat/gh-hound/actions/concurrency_groups/deploy%2Fprod","last_acquired_at":"2026-01-15T16:14:23Z"}]}`)
		case 2:
			if r.URL.Query().Get("after") != "next-cursor" {
				t.Fatalf("second query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, `{"total_count":2,"concurrency_groups":[{"group_name":"ci-build","group_url":"https://api.github.com/repos/indrasvat/gh-hound/actions/concurrency_groups/ci-build","last_acquired_at":"2026-01-15T16:13:55Z"}]}`)
		default:
			t.Fatalf("unexpected extra request %d", requests)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	groups, err := client.ListConcurrencyGroups(context.Background(), "indrasvat/gh-hound", usecase.ConcurrencyListOptions{PerPage: 100, After: "seed"})
	if err != nil {
		t.Fatalf("ListConcurrencyGroups error: %v", err)
	}
	if requests != 2 || len(groups) != 2 {
		t.Fatalf("requests=%d groups=%d, want 2/2", requests, len(groups))
	}
	if groups[0].GroupName != "deploy/prod" || groups[0].LastAcquiredAt == nil {
		t.Fatalf("first group mapped wrong: %#v", groups[0])
	}
	if groups[1].GroupName != "ci-build" {
		t.Fatalf("second group mapped wrong: %#v", groups[1])
	}
}

func TestGetConcurrencyGroupEscapesGroupNameAndForwardsAheadFilter(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, `{"group_name":"deploy/prod","group_url":"https://api.github.com/repos/octocat/Hello-World/actions/concurrency_groups/deploy%2Fprod","total_count":2,"group_members":[{"run_id":30433642,"run_name":"Deploy","run_url":"https://api.github.com/repos/octocat/Hello-World/actions/runs/30433642","run_html_url":"https://github.com/octocat/Hello-World/actions/runs/30433642","status":"in_progress"},{"run_id":30433644,"run_name":"Deploy hotfix","run_url":"https://api.github.com/repos/octocat/Hello-World/actions/runs/30433644","run_html_url":"https://github.com/octocat/Hello-World/actions/runs/30433644","job_id":798245260,"job_name":"deploy","job_url":"https://api.github.com/repos/octocat/Hello-World/actions/jobs/798245260","job_html_url":"https://github.com/octocat/Hello-World/actions/runs/30433644/job/798245260","status":"pending"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	group, err := client.GetConcurrencyGroup(context.Background(), "octocat/Hello-World", "deploy/prod", usecase.ConcurrencyAheadFilter{AheadOfJob: 798245260})
	if err != nil {
		t.Fatalf("GetConcurrencyGroup error: %v", err)
	}
	if gotPath != "/repos/octocat/Hello-World/actions/concurrency_groups/deploy%2Fprod" {
		t.Fatalf("path = %q, want escaped group segment", gotPath)
	}
	if gotQuery != "ahead_of_job=798245260" {
		t.Fatalf("query = %q", gotQuery)
	}
	if group.GroupName != "deploy/prod" || group.ActiveMemberCount != 2 {
		t.Fatalf("group mapped wrong: %#v", group)
	}
	if group.ConfiguredGroupCount != nil {
		t.Fatalf("live group total_count must be active-member count, configured ptr=%v", *group.ConfiguredGroupCount)
	}
	if len(group.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(group.Members))
	}
	job := group.Members[1]
	if job.JobID == nil || *job.JobID != 798245260 || job.JobName != "deploy" || job.Status != model.StatusPending {
		t.Fatalf("job member mapped wrong: %#v", job)
	}
}

func TestGetConcurrencyGroupRejectsConflictingAheadFilters(t *testing.T) {
	client := NewClient("http://127.0.0.1:0", nil)
	_, err := client.GetConcurrencyGroup(context.Background(), "o/r", "deploy", usecase.ConcurrencyAheadFilter{AheadOfRun: 1, AheadOfJob: 2})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want validation failure", err)
	}
}

func TestGetConcurrencyGroupInactive404MapsToNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.GetConcurrencyGroup(context.Background(), "o/r", "inactive", usecase.ConcurrencyAheadFilter{})
	var apiErr usecase.APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != usecase.APIErrorNotFound {
		t.Fatalf("err = %v, want APIError not_found", err)
	}
}

func TestListRunConcurrencyGroupsMapsConfiguredCountPositionsAndCursors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/repos/octocat/Hello-World/actions/runs/30433644/concurrency_groups" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch requests {
		case 1:
			if r.URL.Query().Get("before") != "older" {
				t.Fatalf("first query = %v", r.URL.Query())
			}
			w.Header().Set("Link", `<`+serverURL(r)+`/repos/octocat/Hello-World/actions/runs/30433644/concurrency_groups?after=more>; rel="next"`)
			_, _ = fmt.Fprint(w, `{"total_count":2,"concurrency_groups":[{"group_name":"deploy-prod","group_url":"https://api.github.com/repos/octocat/Hello-World/actions/concurrency_groups/deploy-prod","group_members":[{"run_id":30433642,"run_name":"Deploy","run_url":"https://api.github.com/repos/octocat/Hello-World/actions/runs/30433642","run_html_url":"https://github.com/octocat/Hello-World/actions/runs/30433642","status":"in_progress","position":0,"position_url":"https://api.github.com/repos/octocat/Hello-World/actions/concurrency_groups/deploy-prod?ahead_of_run=30433642"}]}]}`)
		case 2:
			if r.URL.Query().Get("after") != "more" {
				t.Fatalf("second query = %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, `{"total_count":2,"concurrency_groups":[{"group_name":"ci-build","group_url":"https://api.github.com/repos/octocat/Hello-World/actions/concurrency_groups/ci-build","group_members":[{"run_id":30433644,"run_name":"Deploy hotfix","run_url":"https://api.github.com/repos/octocat/Hello-World/actions/runs/30433644","run_html_url":"https://github.com/octocat/Hello-World/actions/runs/30433644","status":"pending","position":2,"position_url":"https://api.github.com/repos/octocat/Hello-World/actions/concurrency_groups/ci-build?ahead_of_job=798245260","job_id":798245260,"job_name":"build","job_url":"https://api.github.com/repos/octocat/Hello-World/actions/jobs/798245260","job_html_url":"https://github.com/octocat/Hello-World/actions/runs/30433644/job/798245260"}]}]}`)
		default:
			t.Fatalf("unexpected extra request %d", requests)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	groups, err := client.ListRunConcurrencyGroups(context.Background(), "octocat/Hello-World", 30433644, usecase.ConcurrencyListOptions{Before: "older"})
	if err != nil {
		t.Fatalf("ListRunConcurrencyGroups error: %v", err)
	}
	if requests != 2 || len(groups) != 2 {
		t.Fatalf("requests=%d groups=%d, want 2/2", requests, len(groups))
	}
	for _, group := range groups {
		if group.ConfiguredGroupCount == nil || *group.ConfiguredGroupCount != 2 {
			t.Fatalf("configured group count missing on %#v", group)
		}
	}
	if groups[0].ActiveMemberCount != 1 || groups[0].Members[0].Position == nil || *groups[0].Members[0].Position != 0 {
		t.Fatalf("first group positions wrong: %#v", groups[0])
	}
	if groups[1].Members[0].JobID == nil || *groups[1].Members[0].Position != 2 {
		t.Fatalf("second group member wrong: %#v", groups[1].Members[0])
	}
}

func TestListRunConcurrencyGroupsRequiresPositiveRunID(t *testing.T) {
	client := NewClient("http://127.0.0.1:0", nil)
	if _, err := client.ListRunConcurrencyGroups(context.Background(), "o/r", 0, usecase.ConcurrencyListOptions{}); err == nil {
		t.Fatal("expected validation failure for missing run id")
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
