package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListArtifactsPaginatesToCompletion(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/actions/runs/42/artifacts") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("per_page = %q, want 100", r.URL.Query().Get("per_page"))
		}
		pages++
		page := r.URL.Query().Get("page")
		artifacts := `[{"id":1,"name":"a","size_in_bytes":10,"expired":false,"digest":"sha256:aa"},{"id":2,"name":"b","size_in_bytes":20,"expired":true}]`
		if page == "2" {
			artifacts = `[{"id":3,"name":"c","size_in_bytes":30,"workflow_run":{"id":42,"head_branch":"main","head_sha":"abc"}}]`
		}
		_, _ = fmt.Fprintf(w, `{"total_count":3,"artifacts":%s}`, artifacts)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	artifacts, err := client.ListArtifacts(context.Background(), "indrasvat/gh-hound", 42)
	if err != nil {
		t.Fatalf("ListArtifacts returned error: %v", err)
	}
	if pages != 2 {
		t.Fatalf("pages fetched = %d, want 2", pages)
	}
	if len(artifacts) != 3 {
		t.Fatalf("artifacts = %d, want 3", len(artifacts))
	}
	if artifacts[0].Name != "a" || !artifacts[1].Expired || artifacts[2].RunID != 42 || artifacts[2].HeadBranch != "main" {
		t.Fatalf("mapping wrong: %#v", artifacts)
	}
}

func TestDownloadArtifactFollowsRedirectAndStreams(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, _ := writer.Create("hello.txt")
	_, _ = entry.Write([]byte("hi"))
	_ = writer.Close()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/repos/indrasvat/gh-hound/actions/artifacts/7/zip", func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			// the test transport adds no auth; the assertion below covers the blob hop
			t.Fatalf("unexpected auth header on API hop: %q", auth)
		}
		http.Redirect(w, r, server.URL+"/blob/signed", http.StatusFound)
	})
	mux.HandleFunc("/blob/signed", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buf.Bytes())
	})

	client := NewClient(server.URL, server.Client())
	body, err := client.DownloadArtifact(context.Background(), "indrasvat/gh-hound", 7)
	if err != nil {
		t.Fatalf("DownloadArtifact returned error: %v", err)
	}
	defer func() { _ = body.Close() }()
	downloaded, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("stream read failed: %v", err)
	}
	if !bytes.Equal(downloaded, buf.Bytes()) {
		t.Fatalf("downloaded %d bytes, want %d", len(downloaded), buf.Len())
	}
}

func TestDownloadArtifactMapsGoneToExpiredError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Artifact has expired"})
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	_, err := client.DownloadArtifact(context.Background(), "indrasvat/gh-hound", 7)
	if err == nil {
		t.Fatal("expected error for 410 Gone")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error should mention expiry: %v", err)
	}
}

type alwaysAuthTransport struct{}

func (alwaysAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer leaked-token")
	return http.DefaultTransport.RoundTrip(clone)
}

func TestDownloadArtifactNeverForwardsAuthToBlobHost(t *testing.T) {
	var blobAuth string
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		blobAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("zipbytes"))
	}))
	defer blob.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blob.URL+"/signed", http.StatusFound)
	}))
	defer api.Close()

	// Hostile injected client: adds auth to EVERY request regardless of
	// host. The blob hop must still see no Authorization header.
	client := NewClient(api.URL, &http.Client{Transport: alwaysAuthTransport{}})
	body, err := client.DownloadArtifact(context.Background(), "indrasvat/gh-hound", 7)
	if err != nil {
		t.Fatalf("DownloadArtifact returned error: %v", err)
	}
	defer func() { _ = body.Close() }()
	if _, err := io.ReadAll(body); err != nil {
		t.Fatal(err)
	}
	if blobAuth != "" {
		t.Fatalf("blob host received Authorization header: %q", blobAuth)
	}
}

func TestListJobsForAttemptPaginates(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/attempts/2/jobs") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		pages++
		var jobs strings.Builder
		jobs.WriteString(`[`)
		count := 100
		start := 0
		if r.URL.Query().Get("page") == "2" {
			count = 1
			start = 100
		}
		for i := range count {
			if i > 0 {
				jobs.WriteString(",")
			}
			_, _ = fmt.Fprintf(&jobs, `{"id":%d,"name":"job-%d","status":"completed","conclusion":"success"}`, start+i+1, start+i+1)
		}
		jobs.WriteString(`]`)
		_, _ = fmt.Fprintf(w, `{"total_count":101,"jobs":%s}`, jobs.String())
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	jobs, err := client.ListJobsForAttempt(context.Background(), "x/y", 42, 2)
	if err != nil {
		t.Fatalf("ListJobsForAttempt error: %v", err)
	}
	if pages != 2 || len(jobs) != 101 {
		t.Fatalf("must paginate: pages=%d jobs=%d", pages, len(jobs))
	}
}
