// Package errors defines sentinel error types with educational messages.
// Every error includes a Hint for developer guidance and a DocsURL for reference.
package errors

import (
	"fmt"

	"github.com/raeseoklee/a2a-sentinel/internal/i18n"
)

// SentinelError is the base error type for all sentinel errors.
// It includes educational Hint and DocsURL fields for developer guidance.
type SentinelError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	DocsURL string `json:"docs_url,omitempty"`
	msgKey  string // i18n key for message
	hintKey string // i18n key for hint
}

// Localize returns a copy of the error with message and hint translated.
// If l is nil, the original error is returned unchanged.
func (e *SentinelError) Localize(l *i18n.Localizer) *SentinelError {
	if l == nil {
		return e
	}
	return &SentinelError{
		Code:    e.Code,
		Message: l.T(e.msgKey),
		Hint:    l.T(e.hintKey),
		DocsURL: e.DocsURL,
		msgKey:  e.msgKey,
		hintKey: e.hintKey,
	}
}

// Error implements the error interface.
func (e *SentinelError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("[%d] %s (hint: %s)", e.Code, e.Message, e.Hint)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Predefined errors — each includes an educational hint and documentation URL.
var (
	ErrAuthRequired        = &SentinelError{Code: 401, Message: "Authentication required", Hint: "Set Authorization header: 'Bearer <token>'", DocsURL: "https://a2a-sentinel.dev/docs/auth", msgKey: i18n.SentinelErrAuthRequiredMsg, hintKey: i18n.SentinelErrAuthRequiredHint}
	ErrAuthInvalid         = &SentinelError{Code: 401, Message: "Invalid authentication token", Hint: "Check token expiry and issuer", DocsURL: "https://a2a-sentinel.dev/docs/auth", msgKey: i18n.SentinelErrAuthInvalidMsg, hintKey: i18n.SentinelErrAuthInvalidHint}
	ErrForbidden           = &SentinelError{Code: 403, Message: "Access denied", Hint: "Check agent permissions and scope configuration", DocsURL: "https://a2a-sentinel.dev/docs/security", msgKey: i18n.SentinelErrForbiddenMsg, hintKey: i18n.SentinelErrForbiddenHint}
	ErrRateLimited         = &SentinelError{Code: 429, Message: "Rate limit exceeded", Hint: "Wait before retrying. Configure security.rate_limit in sentinel.yaml", DocsURL: "https://a2a-sentinel.dev/docs/rate-limit", msgKey: i18n.SentinelErrRateLimitedMsg, hintKey: i18n.SentinelErrRateLimitedHint}
	ErrAgentUnavailable    = &SentinelError{Code: 503, Message: "Target agent unavailable", Hint: "Check agent health with GET /readyz", DocsURL: "https://a2a-sentinel.dev/docs/agents", msgKey: i18n.SentinelErrAgentUnavailableMsg, hintKey: i18n.SentinelErrAgentUnavailableHint}
	ErrStreamLimitExceeded = &SentinelError{Code: 429, Message: "Too many concurrent streams", Hint: "Max streams per agent reached. Configure agents[].max_streams", DocsURL: "https://a2a-sentinel.dev/docs/streaming", msgKey: i18n.SentinelErrStreamLimitMsg, hintKey: i18n.SentinelErrStreamLimitHint}
	ErrReplayDetected      = &SentinelError{Code: 429, Message: "Replay attack detected", Hint: "Request ID already seen within replay window. Use unique IDs for each request.", DocsURL: "https://a2a-sentinel.dev/docs/replay-protection", msgKey: i18n.SentinelErrReplayMsg, hintKey: i18n.SentinelErrReplayHint}
	ErrSSRFBlocked         = &SentinelError{Code: 403, Message: "Push notification URL blocked", Hint: "URL resolves to private network. Use public URLs or configure security.push.allowed_domains", DocsURL: "https://a2a-sentinel.dev/docs/ssrf", msgKey: i18n.SentinelErrSSRFMsg, hintKey: i18n.SentinelErrSSRFHint}
	ErrInvalidRequest      = &SentinelError{Code: 400, Message: "Invalid request format", Hint: "Check A2A protocol specification for correct message format", DocsURL: "https://a2a-sentinel.dev/docs/protocol", msgKey: i18n.SentinelErrInvalidRequestMsg, hintKey: i18n.SentinelErrInvalidRequestHint}
	ErrNoRoute             = &SentinelError{Code: 404, Message: "No matching agent found", Hint: "Check routing path or set a default agent", DocsURL: "https://a2a-sentinel.dev/docs/routing", msgKey: i18n.SentinelErrNoRouteMsg, hintKey: i18n.SentinelErrNoRouteHint}
	ErrGlobalLimitReached  = &SentinelError{Code: 503, Message: "Gateway capacity reached", Hint: "Gateway is at maximum connections. Try again shortly", DocsURL: "https://a2a-sentinel.dev/docs/limits", msgKey: i18n.SentinelErrGlobalLimitMsg, hintKey: i18n.SentinelErrGlobalLimitHint}
)
