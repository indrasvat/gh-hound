package github

import (
	"fmt"

	"github.com/indrasvat/gh-hound/internal/model"
)

type runsResponse struct {
	WorkflowRuns []runDTO `json:"workflow_runs"`
}

type jobsResponse struct {
	TotalCount int      `json:"total_count"`
	Jobs       []jobDTO `json:"jobs"`
}

type workflowsResponse struct {
	Workflows []workflowDTO `json:"workflows"`
}

type loginDTO struct {
	Login string `json:"login"`
}

type pullRequestDTO struct {
	Number int `json:"number"`
}

type runDTO struct {
	ID              int64            `json:"id"`
	Name            string           `json:"name"`
	DisplayTitle    string           `json:"display_title"`
	Status          model.Status     `json:"status"`
	Conclusion      model.Conclusion `json:"conclusion"`
	Event           string           `json:"event"`
	HeadBranch      string           `json:"head_branch"`
	HeadSHA         string           `json:"head_sha"`
	Path            string           `json:"path"`
	RunNumber       int              `json:"run_number"`
	RunAttempt      int              `json:"run_attempt"`
	WorkflowID      int64            `json:"workflow_id"`
	Actor           loginDTO         `json:"actor"`
	TriggeringActor loginDTO         `json:"triggering_actor"`
	CreatedAt       modelTime        `json:"created_at"`
	UpdatedAt       modelTime        `json:"updated_at"`
	RunStartedAt    modelTime        `json:"run_started_at"`
	HTMLURL         string           `json:"html_url"`
	JobsURL         string           `json:"jobs_url"`
	LogsURL         string           `json:"logs_url"`
	PullRequests    []pullRequestDTO `json:"pull_requests"`
}

type jobDTO struct {
	ID              int64            `json:"id"`
	RunID           int64            `json:"run_id"`
	Status          model.Status     `json:"status"`
	Conclusion      model.Conclusion `json:"conclusion"`
	StartedAt       modelTime        `json:"started_at"`
	CompletedAt     modelTime        `json:"completed_at"`
	Name            string           `json:"name"`
	Steps           []stepDTO        `json:"steps"`
	Labels          []string         `json:"labels"`
	RunnerName      string           `json:"runner_name"`
	RunnerGroupName string           `json:"runner_group_name"`
	WorkflowName    string           `json:"workflow_name"`
	HeadBranch      string           `json:"head_branch"`
	HTMLURL         string           `json:"html_url"`
	CheckRunURL     string           `json:"check_run_url"`
}

type stepDTO struct {
	Name        string           `json:"name"`
	Status      model.Status     `json:"status"`
	Conclusion  model.Conclusion `json:"conclusion"`
	Number      int              `json:"number"`
	StartedAt   modelTime        `json:"started_at"`
	CompletedAt modelTime        `json:"completed_at"`
}

type workflowDTO struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

type annotationDTO struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Level     string `json:"annotation_level"`
	Message   string `json:"message"`
	Title     string `json:"title"`
}

func mapRuns(dtos []runDTO) ([]model.Run, error) {
	runs := make([]model.Run, 0, len(dtos))
	for _, dto := range dtos {
		run, err := mapRun(dto)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func mapRun(dto runDTO) (model.Run, error) {
	if !dto.Status.Valid() {
		return model.Run{}, fmt.Errorf("invalid run status %q", dto.Status)
	}
	if !dto.Conclusion.Valid() {
		return model.Run{}, fmt.Errorf("invalid run conclusion %q", dto.Conclusion)
	}
	pullRequests := make([]int, 0, len(dto.PullRequests))
	for _, pullRequest := range dto.PullRequests {
		pullRequests = append(pullRequests, pullRequest.Number)
	}
	return model.Run{
		ID:              dto.ID,
		Name:            dto.Name,
		DisplayTitle:    dto.DisplayTitle,
		Status:          dto.Status,
		Conclusion:      dto.Conclusion,
		Event:           dto.Event,
		HeadBranch:      dto.HeadBranch,
		HeadSHA:         dto.HeadSHA,
		Path:            dto.Path,
		RunNumber:       dto.RunNumber,
		RunAttempt:      dto.RunAttempt,
		WorkflowID:      dto.WorkflowID,
		Actor:           dto.Actor.Login,
		TriggeringActor: dto.TriggeringActor.Login,
		CreatedAt:       dto.CreatedAt.Time,
		UpdatedAt:       dto.UpdatedAt.Time,
		RunStartedAt:    dto.RunStartedAt.Time,
		HTMLURL:         dto.HTMLURL,
		JobsURL:         dto.JobsURL,
		LogsURL:         dto.LogsURL,
		PullRequests:    pullRequests,
	}, nil
}

func mapJobs(dtos []jobDTO) ([]model.Job, error) {
	jobs := make([]model.Job, 0, len(dtos))
	for _, dto := range dtos {
		job, err := mapJob(dto)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func mapJob(dto jobDTO) (model.Job, error) {
	if !dto.Status.Valid() {
		return model.Job{}, fmt.Errorf("invalid job status %q", dto.Status)
	}
	if !dto.Conclusion.Valid() {
		return model.Job{}, fmt.Errorf("invalid job conclusion %q", dto.Conclusion)
	}
	steps := make([]model.Step, 0, len(dto.Steps))
	for _, step := range dto.Steps {
		if !step.Status.Valid() {
			return model.Job{}, fmt.Errorf("invalid step status %q", step.Status)
		}
		if !step.Conclusion.Valid() {
			return model.Job{}, fmt.Errorf("invalid step conclusion %q", step.Conclusion)
		}
		steps = append(steps, model.Step{
			Name:        step.Name,
			Status:      step.Status,
			Conclusion:  step.Conclusion,
			Number:      step.Number,
			StartedAt:   step.StartedAt.Time,
			CompletedAt: step.CompletedAt.Time,
		})
	}
	return model.Job{
		ID:              dto.ID,
		RunID:           dto.RunID,
		Status:          dto.Status,
		Conclusion:      dto.Conclusion,
		StartedAt:       dto.StartedAt.Time,
		CompletedAt:     dto.CompletedAt.Time,
		Name:            dto.Name,
		Steps:           steps,
		Labels:          append([]string(nil), dto.Labels...),
		RunnerName:      dto.RunnerName,
		RunnerGroupName: dto.RunnerGroupName,
		WorkflowName:    dto.WorkflowName,
		HeadBranch:      dto.HeadBranch,
		HTMLURL:         dto.HTMLURL,
		CheckRunURL:     dto.CheckRunURL,
	}, nil
}
