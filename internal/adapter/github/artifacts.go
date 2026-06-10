package github

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type artifactDTO struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	SizeInBytes int64     `json:"size_in_bytes"`
	Expired     bool      `json:"expired"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Digest      string    `json:"digest"`
	WorkflowRun *struct {
		ID         int64  `json:"id"`
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
	} `json:"workflow_run"`
}

type artifactsResponse struct {
	TotalCount int           `json:"total_count"`
	Artifacts  []artifactDTO `json:"artifacts"`
}

func (c *Client) ListArtifacts(ctx context.Context, repo string, runID int64) ([]model.Artifact, error) {
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/artifacts")
	var artifacts []model.Artifact
	for page := 1; ; page++ {
		values := url.Values{
			"per_page": []string{"100"},
			"page":     []string{strconv.Itoa(page)},
		}
		var decoded artifactsResponse
		if err := c.getJSON(ctx, resource, values, &decoded); err != nil {
			return nil, err
		}
		for _, dto := range decoded.Artifacts {
			artifacts = append(artifacts, mapArtifact(dto))
		}
		if len(decoded.Artifacts) == 0 || len(artifacts) >= decoded.TotalCount {
			return artifacts, nil
		}
	}
}

func mapArtifact(dto artifactDTO) model.Artifact {
	artifact := model.Artifact{
		ID:          dto.ID,
		Name:        dto.Name,
		SizeInBytes: dto.SizeInBytes,
		Expired:     dto.Expired,
		CreatedAt:   dto.CreatedAt,
		ExpiresAt:   dto.ExpiresAt,
		UpdatedAt:   dto.UpdatedAt,
		Digest:      dto.Digest,
	}
	if dto.WorkflowRun != nil {
		artifact.RunID = dto.WorkflowRun.ID
		artifact.HeadBranch = dto.WorkflowRun.HeadBranch
		artifact.HeadSHA = dto.WorkflowRun.HeadSHA
	}
	return artifact
}

// DownloadArtifact returns the artifact's zip archive as a stream. The
// API responds 302 with a short-lived signed URL; the signed URL must
// never be logged or surfaced, so traces record only the resource name.
func (c *Client) DownloadArtifact(ctx context.Context, repo string, artifactID int64) (io.ReadCloser, error) {
	resource := resourcePath(repo, "actions/artifacts/"+strconv.FormatInt(artifactID, 10)+"/zip")
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+resource, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	resp, err := noRedirectClient(c.http).Do(req)
	if err != nil {
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Duration: time.Since(start), Err: err.Error()})
		return nil, err
	}
	c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), RateRemaining: resp.Header.Get("X-RateLimit-Remaining")})

	switch {
	case resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect:
		location, err := resp.Location()
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if !location.IsAbs() {
			base, parseErr := url.Parse(c.baseURL + resource)
			if parseErr != nil {
				return nil, parseErr
			}
			location = base.ResolveReference(location)
		}
		return c.fetchArtifactStream(ctx, location.String())
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return resp.Body, nil
	case resp.StatusCode == http.StatusGone:
		_ = resp.Body.Close()
		return nil, usecase.ArtifactExpiredError{Name: strconv.FormatInt(artifactID, 10)}
	default:
		defer func() {
			_ = resp.Body.Close()
		}()
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, mapReadHTTPError(http.MethodGet, resource, resp.StatusCode, resp.Header, limited)
	}
}

func (c *Client) fetchArtifactStream(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// The signed blob URL is self-authorizing. Use a bare client so an
	// injected auth transport can never leak the GitHub token to the
	// storage host. No client timeout: a wall-clock cap would abort
	// large streaming transfers; cancellation flows through ctx.
	bare := &http.Client{Transport: http.DefaultTransport}
	resp, err := bare.Do(req)
	if err != nil {
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: "github-actions-artifact-download", Duration: time.Since(start), Err: err.Error()})
		return nil, err
	}
	c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: "github-actions-artifact-download", Status: resp.StatusCode, Duration: time.Since(start)})
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode == http.StatusGone {
			return nil, usecase.ArtifactExpiredError{Name: "artifact download link"}
		}
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		// Synthetic resource name: the signed URL itself must never
		// appear in errors or traces.
		return nil, mapReadHTTPError(http.MethodGet, "github-actions-artifact-download", resp.StatusCode, resp.Header, limited)
	}
	return resp.Body, nil
}

func (c *Client) GetRunAttempt(ctx context.Context, repo string, runID int64, attempt int) (model.Run, error) {
	var dto runDTO
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/attempts/"+strconv.Itoa(attempt))
	if err := c.getJSON(ctx, resource, nil, &dto); err != nil {
		return model.Run{}, err
	}
	return mapRun(dto)
}

func (c *Client) ListJobsForAttempt(ctx context.Context, repo string, runID int64, attempt int) ([]model.Job, error) {
	resource := resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/attempts/"+strconv.Itoa(attempt)+"/jobs")
	values := url.Values{"per_page": []string{"100"}}
	var decoded jobsResponse
	if err := c.getJSON(ctx, resource, values, &decoded); err != nil {
		return nil, err
	}
	return mapJobs(decoded.Jobs)
}
