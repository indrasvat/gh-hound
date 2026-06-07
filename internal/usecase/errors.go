package usecase

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type Severity string

const (
	SeverityError Severity = "err"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
	SeverityOK    Severity = "ok"
)

type ErrorClass string

const (
	ErrorClassRateLimit        ErrorClass = "rate_limit"
	ErrorClassNetwork          ErrorClass = "network"
	ErrorClassLogRender        ErrorClass = "log_render"
	ErrorClassMutationRejected ErrorClass = "mutation_rejected"
	ErrorClassSuccess          ErrorClass = "success"
)

type ErrorContext struct {
	CachedAge time.Duration
}

type Resilience struct {
	Class          ErrorClass
	Severity       Severity
	Title          string
	Message        string
	RetryAction    string
	KeepCachedView bool
}

type LogRenderError struct {
	Message string
}

func (e LogRenderError) Error() string {
	return e.Message
}

func ResilienceFor(err error, context ErrorContext) Resilience {
	if actionErr, ok := AsActionError(err); ok {
		if actionErr.Kind == ActionErrorRateLimit {
			return Resilience{
				Class:          ErrorClassRateLimit,
				Severity:       SeverityError,
				Title:          fmt.Sprintf("GitHub API · %d", actionErr.Status),
				Message:        actionErr.Message,
				RetryAction:    "auto_resume",
				KeepCachedView: true,
			}
		}
		return Resilience{
			Class:          ErrorClassMutationRejected,
			Severity:       SeverityError,
			Title:          "Mutation rejected",
			Message:        actionErr.Message,
			RetryAction:    "retry",
			KeepCachedView: true,
		}
	}
	var logErr LogRenderError
	if as(err, &logErr) {
		return Resilience{
			Class:          ErrorClassLogRender,
			Severity:       SeverityError,
			Title:          "Log render failed",
			Message:        logErr.Message,
			RetryAction:    "refetch_log",
			KeepCachedView: true,
		}
	}
	var dnsErr *net.DNSError
	_ = as(err, &dnsErr)
	message := "Showing cached view."
	if context.CachedAge > 0 {
		message = fmt.Sprintf("Showing cached view, %s old.", context.CachedAge.Round(time.Second))
	}
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error() + ". " + message
	}
	return Resilience{
		Class:          ErrorClassNetwork,
		Severity:       SeverityWarn,
		Title:          "Runs unavailable",
		Message:        message,
		RetryAction:    "retry",
		KeepCachedView: true,
	}
}

func ResilienceForSuccess(result ActionResult) Resilience {
	message := result.Message
	if result.RunID != 0 {
		message = fmt.Sprintf("CI #%d · %s", result.RunID, result.Action)
	}
	return Resilience{
		Class:          ErrorClassSuccess,
		Severity:       SeverityOK,
		Title:          firstNonEmpty(result.Message, "Action accepted"),
		Message:        message,
		RetryAction:    "refresh",
		KeepCachedView: true,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
