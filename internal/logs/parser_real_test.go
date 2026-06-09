package logs

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseRealGitHubJobLogs(t *testing.T) {
	if os.Getenv("GH_HOUND_REAL_LOGS") != "1" {
		t.Skip("set GH_HOUND_REAL_LOGS=1 to fetch and parse live GitHub Actions job logs")
	}
	repos := strings.Fields(os.Getenv("GH_HOUND_REAL_REPOS"))
	if len(repos) == 0 {
		repos = []string{
			"openclaw/openclaw",
			"cli/cli",
			"charmbracelet/bubbletea",
		}
	}
	jobsPerRepo := envInt("GH_HOUND_REAL_LOG_JOBS", 2)

	for _, repo := range repos {
		t.Run(strings.ReplaceAll(repo, "/", "_"), func(t *testing.T) {
			jobs := latestCompletedJobs(t, repo, jobsPerRepo*3)
			parsedJobs := 0
			for _, job := range jobs {
				raw, err := ghAPIOutput(fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repo, job.ID))
				if err != nil {
					t.Logf("skip %s job=%d name=%q: log unavailable: %v", repo, job.ID, job.Name, err)
					continue
				}
				start := time.Now()
				doc := Parse(raw)
				elapsed := time.Since(start)
				if elapsed > 2*time.Second {
					t.Fatalf("Parse(%s job %d) took %s, want <= 2s", repo, job.ID, elapsed)
				}
				if len(doc.Lines) == 0 {
					t.Fatalf("Parse(%s job %d) returned no lines", repo, job.ID)
				}
				if len(doc.Commands) == 0 {
					t.Fatalf("Parse(%s job %d %q) returned no workflow commands from %d lines", repo, job.ID, job.Name, len(doc.Lines))
				}
				if len(doc.Folds) == 0 {
					t.Fatalf("Parse(%s job %d %q) returned no folds from real Actions log", repo, job.ID, job.Name)
				}
				parsedJobs++
				t.Logf("%s job=%d name=%q lines=%d commands=%d folds=%d annotations=%d masks=%d parse=%s", repo, job.ID, job.Name, len(doc.Lines), len(doc.Commands), len(doc.Folds), len(doc.Annotations), len(doc.Masks), elapsed)
				if parsedJobs >= jobsPerRepo {
					break
				}
			}
			if parsedJobs == 0 {
				t.Fatalf("no downloadable GitHub job logs parsed for %s", repo)
			}
		})
	}
}

type workflowRunsResponse struct {
	WorkflowRuns []workflowRun `json:"workflow_runs"`
}

type workflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
}

type workflowJobsResponse struct {
	Jobs []workflowJob `json:"jobs"`
}

type workflowJob struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

func latestCompletedJobs(t *testing.T, repo string, limit int) []workflowJob {
	t.Helper()
	rawRuns := ghAPI(t, fmt.Sprintf("repos/%s/actions/runs?per_page=30&status=completed", repo))
	var runs workflowRunsResponse
	if err := json.Unmarshal([]byte(rawRuns), &runs); err != nil {
		t.Fatalf("decode runs for %s: %v", repo, err)
	}
	var out []workflowJob
	for _, run := range runs.WorkflowRuns {
		if run.Conclusion == "skipped" {
			continue
		}
		rawJobs := ghAPI(t, fmt.Sprintf("repos/%s/actions/runs/%d/jobs?per_page=100", repo, run.ID))
		var jobs workflowJobsResponse
		if err := json.Unmarshal([]byte(rawJobs), &jobs); err != nil {
			t.Fatalf("decode jobs for %s run %d: %v", repo, run.ID, err)
		}
		for _, job := range jobs.Jobs {
			if job.Status == "completed" && job.ID != 0 {
				out = append(out, job)
				if len(out) >= limit {
					return out
				}
			}
		}
	}
	t.Fatalf("no completed job with logs found for %s", repo)
	return nil
}

func ghAPI(t *testing.T, endpoint string) string {
	t.Helper()
	output, err := ghAPIOutput(endpoint)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return output
}

func ghAPIOutput(endpoint string) (string, error) {
	command := exec.Command("gh", "api", "-X", "GET", endpoint)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh api %s: %w\n%s", endpoint, err, output)
	}
	return string(output), nil
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
