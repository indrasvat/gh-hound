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
	Kind       APIErrorKind
	Method     string
	Resource   string
	Status     int
	Message    string
	RetryAfter time.Duration
	ResetAt    time.Time
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
	RetryAfter     time.Duration
	ResetAt        time.Time
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
				Message:        rateLimitMessage(actionErr.Message, actionErr.RetryAfter, actionErr.ResetAt),
				RetryAction:    "auto_resume",
				RetryAfter:     actionErr.RetryAfter,
				ResetAt:        actionErr.ResetAt,
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
	var expiredErr ArtifactExpiredError
	if as(err, &expiredErr) {
		return Resilience{
			Class:          ErrorClassMutationRejected,
			Severity:       SeverityWarn,
			Title:          "Artifact expired",
			Message:        fmt.Sprintf("%q is past its retention window and can no longer be downloaded", expiredErr.Name),
			KeepCachedView: true,
		}
	}
	var destErr DestinationExistsError
	if as(err, &destErr) {
		return Resilience{
			Class:          ErrorClassMutationRejected,
			Severity:       SeverityWarn,
			Title:          "Download blocked",
			Message:        fmt.Sprintf("%s already exists; remove it before downloading again", destErr.Path),
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
			Message:        rateLimitMessage(firstNonEmpty(err.Message, "GitHub rate limit reached."), err.RetryAfter, err.ResetAt),
			RetryAction:    "auto_resume",
			RetryAfter:     err.RetryAfter,
			ResetAt:        err.ResetAt,
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

func rateLimitMessage(base string, retryAfter time.Duration, resetAt time.Time) string {
	parts := []string{strings.TrimSpace(firstNonEmpty(base, "GitHub rate limit reached."))}
	if retryAfter > 0 {
		parts = append(parts, fmt.Sprintf("auto-resume in %s", retryAfter.Round(time.Second)))
	}
	if !resetAt.IsZero() {
		parts = append(parts, fmt.Sprintf("reset %s", resetAt.UTC().Format("15:04 UTC")))
	}
	return strings.Join(parts, " · ")
}

func ResilienceForSuccess(result ActionResult) Resilience {
	title := firstNonEmpty(result.Message, "Action accepted")
	message := result.Message
	if result.RunID != 0 {
		message = fmt.Sprintf("CI #%d · %s", result.RunID, result.Action)
	} else if message == title {
		// Run-less actions (workflow toggles) would otherwise echo the
		// title as the body — "back on duty. · back on duty." stutters.
		message = ""
	}
	return Resilience{
		Class:          ErrorClassSuccess,
		Severity:       SeverityOK,
		Title:          title,
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
