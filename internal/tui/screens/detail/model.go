package detail

import "github.com/indrasvat/gh-hound/internal/model"

type Focus string

const (
	FocusJobs  Focus = "jobs"
	FocusSteps Focus = "steps"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone        IntentKind = ""
	IntentFailure     IntentKind = "failure"
	IntentLog         IntentKind = "log"
	IntentWatch       IntentKind = "watch"
	IntentRerunJob    IntentKind = "rerun_job"
	IntentRerunFailed IntentKind = "rerun_failed"
	IntentCancel      IntentKind = "cancel"
	IntentForceCancel IntentKind = "force_cancel"
	IntentBrowser     IntentKind = "browser"
	IntentPreviousRun IntentKind = "previous_run"
	IntentNextRun     IntentKind = "next_run"
	IntentBack        IntentKind = "back"
)

type Intent struct {
	Kind  IntentKind
	RunID int64
	JobID int64
	Step  int
}

type Model struct {
	Repo         string
	Run          model.Run
	Jobs         []model.Job
	SelectedJob  int
	SelectedStep int
	Focus        Focus
	Intent       Intent
}

func NewModel(run model.Run, jobs []model.Job) Model {
	m := Model{Run: run, Jobs: append([]model.Job(nil), jobs...), Focus: FocusJobs}
	m.jumpFailure()
	return m
}

func (m Model) WithRepo(repo string) Model {
	m.Repo = repo
	return m
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "tab":
		if m.Focus == FocusJobs {
			m.Focus = FocusSteps
		} else {
			m.Focus = FocusJobs
		}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "n":
		m.jumpFailure()
	case "enter":
		m.Intent = m.intentFor(IntentFailure)
	case "l":
		m.Intent = m.intentFor(IntentLog)
	case "w":
		m.Intent = m.intentFor(IntentWatch)
	case "r":
		m.Intent = m.intentFor(IntentRerunJob)
	case "R":
		m.Intent = m.intentFor(IntentRerunFailed)
	case "x":
		m.Intent = m.intentFor(IntentCancel)
	case "X":
		m.Intent = m.intentFor(IntentForceCancel)
	case "o":
		m.Intent = m.intentFor(IntentBrowser)
	case "J":
		m.Intent = Intent{Kind: IntentNextRun, RunID: m.Run.ID}
	case "K":
		m.Intent = Intent{Kind: IntentPreviousRun, RunID: m.Run.ID}
	case "esc":
		m.Intent = Intent{Kind: IntentBack, RunID: m.Run.ID}
	}
	return m
}

func (m *Model) move(delta int) {
	if m.Focus == FocusJobs {
		m.SelectedJob = clamp(m.SelectedJob+delta, len(m.Jobs))
		m.SelectedStep = clamp(m.SelectedStep, len(m.selectedJob().Steps))
		return
	}
	m.SelectedStep = clamp(m.SelectedStep+delta, len(m.selectedJob().Steps))
}

func (m *Model) jumpFailure() {
	for jobIndex, job := range m.Jobs {
		for stepIndex, step := range job.Steps {
			if step.Conclusion == model.ConclusionFailure || step.Conclusion == model.ConclusionActionRequired || step.Conclusion == model.ConclusionTimedOut {
				m.SelectedJob = jobIndex
				m.SelectedStep = stepIndex
				m.Focus = FocusSteps
				return
			}
		}
	}
	for jobIndex, job := range m.Jobs {
		if job.Conclusion == model.ConclusionFailure || job.Conclusion == model.ConclusionActionRequired || job.Conclusion == model.ConclusionTimedOut {
			m.SelectedJob = jobIndex
			m.SelectedStep = 0
			m.Focus = FocusJobs
			return
		}
	}
}

func (m Model) selectedJob() model.Job {
	if len(m.Jobs) == 0 || m.SelectedJob < 0 || m.SelectedJob >= len(m.Jobs) {
		return model.Job{}
	}
	return m.Jobs[m.SelectedJob]
}

func (m Model) SelectedJobModel() (model.Job, bool) {
	if len(m.Jobs) == 0 || m.SelectedJob < 0 || m.SelectedJob >= len(m.Jobs) {
		return model.Job{}, false
	}
	return m.Jobs[m.SelectedJob], true
}

func (m Model) selectedStep() model.Step {
	job := m.selectedJob()
	if len(job.Steps) == 0 || m.SelectedStep < 0 || m.SelectedStep >= len(job.Steps) {
		return model.Step{}
	}
	return job.Steps[m.SelectedStep]
}

func (m Model) SelectedStepModel() (model.Step, bool) {
	job, ok := m.SelectedJobModel()
	if !ok || len(job.Steps) == 0 || m.SelectedStep < 0 || m.SelectedStep >= len(job.Steps) {
		return model.Step{}, false
	}
	return job.Steps[m.SelectedStep], true
}

func (m Model) intentFor(kind IntentKind) Intent {
	job := m.selectedJob()
	step := m.selectedStep()
	return Intent{Kind: kind, RunID: m.Run.ID, JobID: job.ID, Step: step.Number}
}

func clamp(value, length int) int {
	if length <= 0 || value < 0 {
		return 0
	}
	if value >= length {
		return length - 1
	}
	return value
}
