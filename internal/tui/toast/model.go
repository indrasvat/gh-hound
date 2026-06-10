package toast

import (
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

type Toast struct {
	ID          string
	Severity    usecase.Severity
	Title       string
	Message     string
	RetryAction string
	RetryAfter  time.Duration
	ResetAt     time.Time
	Timeout     time.Duration
	SourceClass usecase.ErrorClass

	elapsed time.Duration
}

type Model struct {
	Toasts []Toast
}

type KeyMsg struct {
	Key string
}

type TickMsg struct {
	Elapsed time.Duration
}

func New() Model {
	return Model{}
}

func FromResilience(id string, resilience usecase.Resilience, timeout time.Duration) Toast {
	return Toast{
		ID:          id,
		Severity:    resilience.Severity,
		Title:       resilience.Title,
		Message:     resilience.Message,
		RetryAction: resilience.RetryAction,
		RetryAfter:  resilience.RetryAfter,
		ResetAt:     resilience.ResetAt,
		Timeout:     timeout,
		SourceClass: resilience.Class,
	}
}

func (m Model) Push(toast Toast) Model {
	m.Toasts = append(m.Toasts, toast)
	return m
}

// Dismiss drops every toast with the given id, e.g. a progress toast
// superseded by its completion toast.
func (m Model) Dismiss(id string) Model {
	next := make([]Toast, 0, len(m.Toasts))
	for _, toast := range m.Toasts {
		if toast.ID != id {
			next = append(next, toast)
		}
	}
	m.Toasts = next
	return m
}

func (m Model) Update(msg any) (Model, bool) {
	switch typed := msg.(type) {
	case KeyMsg:
		switch typed.Key {
		case "esc", "⎋":
			if len(m.Toasts) == 0 {
				return m, false
			}
			m.Toasts = m.Toasts[:len(m.Toasts)-1]
			return m, true
		case "g":
			if len(m.Toasts) == 0 {
				return m, false
			}
			m.Toasts = nil
			return m, true
		default:
			return m, false
		}
	case TickMsg:
		next := m.Toasts[:0]
		for _, toast := range m.Toasts {
			toast.elapsed += typed.Elapsed
			if toast.Timeout <= 0 || toast.elapsed < toast.Timeout {
				next = append(next, toast)
			}
		}
		m.Toasts = next
		return m, false
	default:
		return m, false
	}
}
