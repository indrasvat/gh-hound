package failure

import (
	"github.com/indrasvat/gh-hound/internal/logs"
	"github.com/indrasvat/gh-hound/internal/usecase"
)

type KeyMsg struct {
	Key string
}

type IntentKind string

const (
	IntentNone        IntentKind = ""
	IntentFullLog     IntentKind = "full_log"
	IntentCopyExcerpt IntentKind = "copy_excerpt"
	IntentBrowser     IntentKind = "browser"
	IntentRerunJob    IntentKind = "rerun_job"
	IntentRerunFailed IntentKind = "rerun_failed"
	IntentBack        IntentKind = "back"
)

type Intent struct {
	Kind   IntentKind
	Repo   string
	RunID  int64
	JobID  int64
	Offset int
}

type Model struct {
	Repo    string
	RunID   int64
	Report  usecase.FailureReport
	Offset  int
	Excerpt []logs.Line
	Intent  Intent
}

func NewModel(repo string, runID int64, report usecase.FailureReport) Model {
	offset := report.Log.Failure.AnchorLine
	if offset > 1 {
		offset--
	}
	excerpt := excerptFrom(report.Log.Failure.Lines, offset, 5)
	return Model{
		Repo:    repo,
		RunID:   runID,
		Report:  report,
		Offset:  offset,
		Excerpt: excerpt,
	}
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
	case "esc":
		m.Intent = m.intent(IntentBack)
	}
	return m
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
