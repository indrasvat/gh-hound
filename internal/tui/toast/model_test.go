package toast

import (
	"testing"
	"time"

	"github.com/indrasvat/gh-hound/internal/usecase"
)

func TestStackDismissKeysAndPassthrough(t *testing.T) {
	model := New()
	model = model.Push(Toast{ID: "one", Severity: usecase.SeverityWarn, Title: "Runs unavailable", Timeout: 5 * time.Second})
	model = model.Push(Toast{ID: "two", Severity: usecase.SeverityError, Title: "GitHub API", Timeout: 5 * time.Second})

	var handled bool
	model, handled = model.Update(KeyMsg{Key: "j"})
	if handled {
		t.Fatalf("non-toast key should pass through")
	}
	if len(model.Toasts) != 2 {
		t.Fatalf("toasts changed on passthrough: %#v", model.Toasts)
	}

	model, handled = model.Update(KeyMsg{Key: "esc"})
	if !handled || len(model.Toasts) != 1 || model.Toasts[0].ID != "one" {
		t.Fatalf("esc dismiss = handled %v toasts %#v", handled, model.Toasts)
	}

	model = model.Push(Toast{ID: "three", Severity: usecase.SeverityOK, Title: "Re-run queued", Timeout: 5 * time.Second})
	model, handled = model.Update(KeyMsg{Key: "g"})
	if !handled || len(model.Toasts) != 0 {
		t.Fatalf("dismiss all = handled %v toasts %#v", handled, model.Toasts)
	}
}

func TestAutoDismissByTick(t *testing.T) {
	model := New()
	model = model.Push(Toast{ID: "one", Severity: usecase.SeverityWarn, Title: "Runs unavailable", Timeout: time.Second})

	model, handled := model.Update(TickMsg{Elapsed: 500 * time.Millisecond})
	if handled || len(model.Toasts) != 1 {
		t.Fatalf("early tick = handled %v toasts %#v", handled, model.Toasts)
	}

	model, handled = model.Update(TickMsg{Elapsed: 600 * time.Millisecond})
	if handled || len(model.Toasts) != 0 {
		t.Fatalf("timeout tick = handled %v toasts %#v", handled, model.Toasts)
	}
}

func TestToastFromResilienceKeepsRetryData(t *testing.T) {
	resilience := usecase.Resilience{
		Class:       usecase.ErrorClassRateLimit,
		Severity:    usecase.SeverityError,
		Title:       "GitHub API · 429",
		Message:     "Backing off",
		RetryAction: "auto_resume",
	}
	got := FromResilience("rate", resilience, 5*time.Second)
	if got.SourceClass != usecase.ErrorClassRateLimit || got.RetryAction != "auto_resume" || got.Timeout != 5*time.Second {
		t.Fatalf("toast = %#v", got)
	}
}
