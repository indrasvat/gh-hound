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
	ErrorClassAccessDenied     ErrorClass = "access_denied"
	ErrorClassNotFound         ErrorClass = "not_found"
	ErrorClassLogRender        ErrorClass = "log_render"
	ErrorClassMutationRejected ErrorClass = "mutation_rejected"
	ErrorClassSuccess          ErrorClass = "success"
)

type APIErrorKind string

const (
	APIErrorAuth       APIErrorKind = "auth"
	APIErrorPermission APIErrorKind = "permission"
	APIErrorNotFound   APIErrorKind = "not_found"
	APIErrorRateLimit  APIErrorKind = "rate_limit"
	APIErrorNetwork    APIErrorKind = "network"
	APIErrorUnknown    APIErrorKind = "unknown"
)

type APIError struct {
	Kind     APIErrorKind
	Method   string
	Resource string
	Status   int
	Message  string
}

func (e APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Status != 0 {
		return fmt.Sprintf("github api returned status %d", e.Status)
	}
	return string(e.Kind)
}

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
	var apiErr APIError
	if as(err, &apiErr) {
		return resilienceForAPIError(apiErr, context)
	}
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

func resilienceForAPIError(err APIError, context ErrorContext) Resilience {
	switch err.Kind {
	case APIErrorRateLimit:
		return Resilience{
			Class:          ErrorClassRateLimit,
			Severity:       SeverityError,
			Title:          fmt.Sprintf("GitHub API · %d", err.Status),
			Message:        firstNonEmpty(err.Message, "GitHub rate limit reached."),
			RetryAction:    "auto_resume",
			KeepCachedView: true,
		}
	case APIErrorAuth, APIErrorPermission:
		return Resilience{
			Class:          ErrorClassAccessDenied,
			Severity:       SeverityError,
			Title:          "GitHub access denied",
			Message:        firstNonEmpty(err.Message, "Check gh auth status, token scopes, SSO, and repository Actions permissions."),
			RetryAction:    "reauth",
			KeepCachedView: true,
		}
	case APIErrorNotFound:
		return Resilience{
			Class:          ErrorClassNotFound,
			Severity:       SeverityError,
			Title:          "GitHub resource not found",
			Message:        firstNonEmpty(err.Message, "Repository, run, job, or workflow was not found."),
			RetryAction:    "check_repo",
			KeepCachedView: true,
		}
	default:
		message := firstNonEmpty(err.Message, "Showing cached view.")
		if context.CachedAge > 0 {
			message = fmt.Sprintf("%s Showing cached view, %s old.", message, context.CachedAge.Round(time.Second))
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
