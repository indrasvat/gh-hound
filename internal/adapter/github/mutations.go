package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func (c *Client) RerunRun(ctx context.Context, repo string, runID int64, debug bool) (usecase.ActionResult, error) {
	result := usecase.ActionResult{Action: usecase.ActionRerunRun, Repo: repo, RunID: runID, Message: "Re-run queued"}
	body := map[string]bool{"enable_debug_logging": debug}
	return result, c.postJSON(ctx, resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/rerun"), body)
}

func (c *Client) RerunFailedJobs(ctx context.Context, repo string, runID int64) (usecase.ActionResult, error) {
	result := usecase.ActionResult{Action: usecase.ActionRerunFailedJobs, Repo: repo, RunID: runID, Message: "Re-run failed jobs queued"}
	return result, c.postJSON(ctx, resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/rerun-failed-jobs"), nil)
}

func (c *Client) RerunJob(ctx context.Context, repo string, jobID int64) (usecase.ActionResult, error) {
	result := usecase.ActionResult{Action: usecase.ActionRerunJob, Repo: repo, JobID: jobID, Message: "Job re-run queued"}
	return result, c.postJSON(ctx, resourcePath(repo, "actions/jobs/"+strconv.FormatInt(jobID, 10)+"/rerun"), nil)
}

func (c *Client) CancelRun(ctx context.Context, repo string, runID int64) (usecase.ActionResult, error) {
	result := usecase.ActionResult{Action: usecase.ActionCancelRun, Repo: repo, RunID: runID, Message: "Run cancel requested"}
	return result, c.postJSON(ctx, resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/cancel"), nil)
}

func (c *Client) ForceCancelRun(ctx context.Context, repo string, runID int64) (usecase.ActionResult, error) {
	result := usecase.ActionResult{Action: usecase.ActionForceCancelRun, Repo: repo, RunID: runID, Message: "Run force-cancel requested"}
	return result, c.postJSON(ctx, resourcePath(repo, "actions/runs/"+strconv.FormatInt(runID, 10)+"/force-cancel"), nil)
}

func (c *Client) DispatchWorkflow(ctx context.Context, repo, workflowID string, request usecase.DispatchRequest) (usecase.ActionResult, error) {
	result := usecase.ActionResult{Action: usecase.ActionDispatch, Repo: repo, WorkflowID: workflowID, Message: "Workflow dispatch queued"}
	escapedWorkflowID := url.PathEscape(workflowID)
	return result, c.postJSON(ctx, resourcePath(repo, "actions/workflows/"+escapedWorkflowID+"/dispatches"), request)
}

func (c *Client) postJSON(ctx context.Context, resource string, body any) error {
	return c.queue.Do(ctx, func(ctx context.Context) error {
		start := time.Now()
		var reader io.Reader
		if body != nil {
			var encoded bytes.Buffer
			if err := json.NewEncoder(&encoded).Encode(body); err != nil {
				return err
			}
			reader = &encoded
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+resource, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", APIVersion)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Duration: time.Since(start), Err: err.Error()})
			return usecase.ActionError{Kind: usecase.ActionErrorNetwork, Message: err.Error()}
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		c.traceHTTP(ctx, traceRecord{Method: req.Method, Resource: resource, Status: resp.StatusCode, Duration: time.Since(start), RateRemaining: resp.Header.Get("X-RateLimit-Remaining")})
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return mapActionHTTPError(resp.StatusCode, bytes.TrimSpace(payload))
	})
}

func mapActionHTTPError(status int, payload []byte) error {
	kind := usecase.ActionErrorUnknown
	switch status {
	case http.StatusForbidden:
		kind = usecase.ActionErrorPermission
	case http.StatusConflict:
		kind = usecase.ActionErrorConflict
	case http.StatusTooManyRequests:
		kind = usecase.ActionErrorRateLimit
	}
	message := string(payload)
	if message == "" {
		message = fmt.Sprintf("github api returned status %d", status)
	}
	return usecase.ActionError{Kind: kind, Status: status, Message: message}
}
