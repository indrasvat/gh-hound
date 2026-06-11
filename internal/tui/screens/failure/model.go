package failure

import (
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/model"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone         IntentKind = ""
	IntentFullLog      IntentKind = "full_log"
	IntentCopyExcerpt  IntentKind = "copy_excerpt"
	IntentBrowser      IntentKind = "browser"
	IntentRerunJob     IntentKind = "rerun_job"
	IntentRerunFailed  IntentKind = "rerun_failed"
	IntentOpenEvidence IntentKind = "open_evidence"
	IntentBack         IntentKind = "back"
)

type Intent struct {
	Kind   IntentKind
	Repo   string
	RunID  int64
	JobID  int64
	Offset int
	// EvidenceRun carries the run an evidence row points at for the
	// open_evidence intent.
	EvidenceRun model.Run
}

type Model struct {
	Repo       string
	RunID      int64
	Report     usecase.FailureReport
	Offset     int
	Excerpt    []logs.Line
	TotalLines int
	Intent     Intent

	// Flake is the scent panel: the failing job's non-clean verdict,
	// nil until the async scan lands (or when the trail is clean).
	Flake *usecase.JobFlake
	// FlakeWindow is the runs-scanned count behind the verdict, for
	// the "flaked N of last M runs" line.
	FlakeWindow int
	// PanelFocus routes j/k/enter to the flake panel instead of the
	// excerpt viewport; tab toggles it. The failure screen stays a
	// scroll viewport (no selection concept) — only the panel has a
	// row cursor.
	PanelFocus    bool
	PanelSelected int
}

func NewModel(repo string, runID int64, report usecase.FailureReport) Model {
	offset := report.Log.Failure.AnchorLine
	if offset > 1 {
		offset--
	}
	excerpt := excerptFrom(report.Log.Failure.Lines, offset, 5)
	return Model{
		Repo:       repo,
		RunID:      runID,
		Report:     report,
		Offset:     offset,
		Excerpt:    excerpt,
		TotalLines: len(report.Log.Lines),
	}
}

// WithFlake attaches the scent panel once the async verdict lands.
func (m Model) WithFlake(job usecase.JobFlake, window int) Model {
	flake := job
	m.Flake = &flake
	m.FlakeWindow = window
	if m.PanelSelected >= len(flake.Evidence) {
		m.PanelSelected = 0
	}
	return m
}

func (m Model) Update(msg KeyMsg) Model {
	m.Intent = Intent{}
	switch msg.Key {
	case "l":
		m.Intent = m.intent(IntentFullLog)
	case "y":
		m.Intent = m.intent(IntentCopyExcerpt)
	case "o":
		m.Intent = m.intent(IntentBrowser)
	case "r":
		m.Intent = m.intent(IntentRerunJob)
	case "R":
		m.Intent = m.intent(IntentRerunFailed)
	case "tab":
		if m.Flake != nil {
			m.PanelFocus = !m.PanelFocus
		}
	case "j", "down":
		m = m.move(1)
	case "k", "up":
		m = m.move(-1)
	case "enter":
		if m.PanelFocus && m.Flake != nil {
			if evidence, ok := m.selectedEvidence(); ok && evidence.Run.ID != 0 {
				intent := m.intent(IntentOpenEvidence)
				intent.EvidenceRun = evidence.Run
				m.Intent = intent
			}
		}
	case "esc":
		m.Intent = m.intent(IntentBack)
	}
	return m
}

// move drives whichever pane has focus: the evidence cursor when the
// panel is focused, the excerpt viewport otherwise.
func (m Model) move(delta int) Model {
	if m.PanelFocus && m.Flake != nil {
		next := m.PanelSelected + delta
		if next >= 0 && next < len(m.Flake.Evidence) {
			m.PanelSelected = next
		}
		return m
	}
	return m.scrollExcerpt(delta)
}

func (m Model) scrollExcerpt(delta int) Model {
	lines := m.Report.Log.Failure.Lines
	if len(lines) == 0 {
		return m
	}
	first := lines[0].Number
	last := lines[len(lines)-1].Number
	maxOffset := max(last-4, first)
	next := min(max(m.Offset+delta, first), maxOffset)
	if next == m.Offset {
		return m
	}
	m.Offset = next
	m.Excerpt = excerptFrom(lines, next, 5)
	return m
}

func (m Model) selectedEvidence() (usecase.FlakeEvidence, bool) {
	if m.Flake == nil || len(m.Flake.Evidence) == 0 {
		return usecase.FlakeEvidence{}, false
	}
	index := m.PanelSelected
	if index < 0 || index >= len(m.Flake.Evidence) {
		return usecase.FlakeEvidence{}, false
	}
	return m.Flake.Evidence[index], true
}

func (m Model) intent(kind IntentKind) Intent {
	return Intent{
		Kind:   kind,
		Repo:   m.Repo,
		RunID:  m.RunID,
		JobID:  m.Report.Job.ID,
		Offset: m.Offset,
	}
}

func excerptFrom(lines []logs.Line, offset, limit int) []logs.Line {
	out := make([]logs.Line, 0, limit)
	for _, line := range lines {
		if line.Number < offset {
			continue
		}
		out = append(out, line)
		if len(out) == limit {
			return out
		}
	}
	return out
}
