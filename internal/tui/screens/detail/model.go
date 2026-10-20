package detail

import (
	"maps"
	"time"

	"github.com/indrasvat/gh-hound/internal/model"
)

type Focus string

const (
	FocusJobs      Focus = "jobs"
	FocusSteps     Focus = "steps"
	FocusArtifacts Focus = "artifacts"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone             IntentKind = ""
	IntentFailure          IntentKind = "failure"
	IntentLog              IntentKind = "log"
	IntentWatch            IntentKind = "watch"
	IntentRerunJob         IntentKind = "rerun_job"
	IntentRerunFailed      IntentKind = "rerun_failed"
	IntentCancel           IntentKind = "cancel"
	IntentForceCancel      IntentKind = "force_cancel"
	IntentBrowser          IntentKind = "browser"
	IntentCopyURL          IntentKind = "copy_url"
	IntentCopySHA          IntentKind = "copy_sha"
	IntentPreviousRun      IntentKind = "previous_run"
	IntentNextRun          IntentKind = "next_run"
	IntentBack             IntentKind = "back"
	IntentDownloadArtifact IntentKind = "download_artifact"
	IntentOpenArtifactDir  IntentKind = "open_artifact_dir"
	IntentCopyArtifactPath IntentKind = "copy_artifact_path"
)

// DownloadState tracks one artifact's download lifecycle inside this
// session. The zero value means "never attempted".
type DownloadState string

const (
	DownloadStateNone        DownloadState = ""
	DownloadStateDownloading DownloadState = "downloading"
	DownloadStateExtracting  DownloadState = "extracting"
	DownloadStateDone        DownloadState = "done"
	DownloadStateFailed      DownloadState = "failed"
)

// DownloadStatus is per-artifact, session-scoped UI state: where a
// download stands, how many archive bytes have arrived, and — once
// done — the absolute extraction path the o/y actions operate on.
type DownloadStatus struct {
	State     DownloadState
	Bytes     int64
	Path      string
	FileCount int
	Reason    string
}

type Intent struct {
	Kind       IntentKind
	RunID      int64
	JobID      int64
	Step       int
	ArtifactID int64
}

type Model struct {
	Repo      string
	Run       model.Run
	Jobs      []model.Job
	Artifacts []model.Artifact
	// PendingDeployments holds the gate state for a waiting run; the
	// detail view renders the pending-environments panel from it.
	PendingDeployments []model.PendingDeployment
	SelectedJob        int
	SelectedStep       int
	SelectedArtifact   int
	Focus              Focus
	Intent             Intent

	// Downloads maps artifact ID to its session download status. The
	// map is copied on write (WithDownload) to keep value semantics.
	Downloads map[int64]DownloadStatus

	// Loading state is transient render input set by the app from its
	// pending load each frame — never persisted, so a cancelled load
	// can not strand a skeleton.
	Loading     bool
	LoadingLine string

	// SpinnerFrame indexes icons.SpinnerFrames for the downloading and
	// extracting row annotations; the app advances it on poll ticks
	// while a download is live (the shared-spinner contract).
	SpinnerFrame int

	// Now is the clock a running job or step's elapsed time reads
	// against; zero means the wall clock.
	Now time.Time
}

func (m Model) clock() time.Time {
	if m.Now.IsZero() {
		return time.Now()
	}
	return m.Now
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

// WithJobs swaps in a fresher listing of the run's jobs, keeping the
// cursor on the same job (by ID) and step where they still exist.
func (m Model) WithJobs(jobs []model.Job) Model {
	selected := m.selectedJob().ID
	// A cursor resting on the running step follows it to the next one;
	// a cursor the user moved elsewhere stays.
	running, onRunning := runningStep(m.selectedJob())
	onRunning = onRunning && running == m.SelectedStep
	m.Jobs = append([]model.Job(nil), jobs...)
	kept := false
	for i, job := range m.Jobs {
		if job.ID == selected {
			m.SelectedJob, kept = i, true
			break
		}
	}
	if !kept {
		// The selected job is gone: its step index means nothing for
		// whichever job the cursor lands on.
		m.SelectedJob = min(max(m.SelectedJob, 0), max(len(m.Jobs)-1, 0))
		m.SelectedStep = 0
	}
	if len(m.Jobs) > 0 {
		m.SelectedStep = min(max(m.SelectedStep, 0), max(len(m.Jobs[m.SelectedJob].Steps)-1, 0))
		if step, ok := runningStep(m.Jobs[m.SelectedJob]); ok && kept && onRunning {
			m.SelectedStep = step
		}
	} else {
		m.SelectedStep = 0
	}
	return m
}

func (m Model) WithArtifacts(artifacts []model.Artifact) Model {
	m.Artifacts = append([]model.Artifact(nil), artifacts...)
	m.SelectedArtifact = clamp(m.SelectedArtifact, len(m.Artifacts))
	if len(m.Artifacts) == 0 && m.Focus == FocusArtifacts {
		m.Focus = FocusJobs
	}
	return m
}

func (m Model) WithPendingDeployments(pending []model.PendingDeployment) Model {
	m.PendingDeployments = append([]model.PendingDeployment(nil), pending...)
	return m
}

// WithDownload records one artifact's download status, copying the map
// so earlier Model values stay frozen.
func (m Model) WithDownload(artifactID int64, status DownloadStatus) Model {
	next := make(map[int64]DownloadStatus, len(m.Downloads)+1)
	maps.Copy(next, m.Downloads)
	next[artifactID] = status
	m.Downloads = next
	return m
}

// Download returns the selected-session download status for an artifact.
func (m Model) Download(artifactID int64) DownloadStatus {
	return m.Downloads[artifactID]
}

// DownloadedCount reports how many of this run's artifacts finished
// downloading this session (the pane-header chip).
func (m Model) DownloadedCount() int {
	count := 0
	for _, artifact := range m.Artifacts {
		if m.Downloads[artifact.ID].State == DownloadStateDone {
			count++
		}
	}
	return count
}

// selectedDownloadedPath returns the extraction path when the selected
// artifact's download is done — the gate for the contextual o/y keys.
func (m Model) selectedDownloadedPath() (int64, bool) {
	artifact, ok := m.SelectedArtifactModel()
	if !ok || m.Focus != FocusArtifacts {
		return 0, false
	}
	if m.Downloads[artifact.ID].State != DownloadStateDone {
		return 0, false
	}
	return artifact.ID, true
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "tab":
		switch m.Focus {
		case FocusJobs:
			m.Focus = FocusSteps
		case FocusSteps:
			if len(m.Artifacts) > 0 {
				m.Focus = FocusArtifacts
			} else {
				m.Focus = FocusJobs
			}
		default:
			m.Focus = FocusJobs
		}
	case "a":
		if len(m.Artifacts) > 0 {
			m.Focus = FocusArtifacts
		}
	case "d":
		if artifact, ok := m.SelectedArtifactModel(); ok {
			m.Intent = Intent{Kind: IntentDownloadArtifact, RunID: m.Run.ID, ArtifactID: artifact.ID}
		}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "n":
		m.jumpFailure()
	case "enter":
		if m.Focus == FocusArtifacts {
			if artifact, ok := m.SelectedArtifactModel(); ok {
				m.Intent = Intent{Kind: IntentDownloadArtifact, RunID: m.Run.ID, ArtifactID: artifact.ID}
			}
			break
		}
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
		// On a downloaded artifact, o opens the extracted folder; the
		// browser meaning survives everywhere else.
		if id, ok := m.selectedDownloadedPath(); ok {
			m.Intent = Intent{Kind: IntentOpenArtifactDir, RunID: m.Run.ID, ArtifactID: id}
			break
		}
		m.Intent = m.intentFor(IntentBrowser)
	case "y":
		// On a downloaded artifact, y copies the extraction path; the
		// copy-URL meaning survives everywhere else.
		if id, ok := m.selectedDownloadedPath(); ok {
			m.Intent = Intent{Kind: IntentCopyArtifactPath, RunID: m.Run.ID, ArtifactID: id}
			break
		}
		m.Intent = m.intentFor(IntentCopyURL)
	case "Y":
		m.Intent = m.intentFor(IntentCopySHA)
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
	switch m.Focus {
	case FocusJobs:
		m.SelectedJob = clamp(m.SelectedJob+delta, len(m.Jobs))
		m.SelectedStep = clamp(m.SelectedStep, len(m.selectedJob().Steps))
		if step, ok := runningStep(m.selectedJob()); ok {
			// A running job opens on the step it is on: in a long list
			// it would otherwise sit below the window.
			m.SelectedStep = step
		}
	case FocusArtifacts:
		m.SelectedArtifact = clamp(m.SelectedArtifact+delta, len(m.Artifacts))
	default:
		m.SelectedStep = clamp(m.SelectedStep+delta, len(m.selectedJob().Steps))
	}
}

func (m Model) SelectedArtifactModel() (model.Artifact, bool) {
	if len(m.Artifacts) == 0 || m.SelectedArtifact < 0 || m.SelectedArtifact >= len(m.Artifacts) {
		return model.Artifact{}, false
	}
	return m.Artifacts[m.SelectedArtifact], true
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
	// Nothing failed: a live run opens on its first running job, at the
	// step it is on.
	for jobIndex, job := range m.Jobs {
		if step, ok := runningStep(job); ok {
			m.SelectedJob, m.SelectedStep = jobIndex, step
			return
		}
	}
}

// runningStep is the index of the step a running job is on.
func runningStep(job model.Job) (int, bool) {
	if job.Status != model.StatusInProgress {
		return 0, false
	}
	for index, step := range job.Steps {
		if step.Status == model.StatusInProgress {
			return index, true
		}
	}
	return 0, false
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
