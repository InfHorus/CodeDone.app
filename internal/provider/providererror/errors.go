package providererror

import (
	"fmt"
	"strings"
)

const (
	KindAuthentication = "authentication"
	KindRateLimit      = "rate_limit"
	KindQuota          = "quota"
	KindModel          = "model"
	KindBadRequest     = "bad_request"
	KindUnavailable    = "unavailable"
	KindNetwork        = "network"
	KindUnknown        = "unknown"
)

type Error struct {
	Provider   string
	Kind       string
	StatusCode int
	Message    string
	Detail     string
}

func (e *Error) Error() string {
	return e.UserMessage()
}

func (e *Error) UserMessage() string {
	provider := strings.TrimSpace(e.Provider)
	if provider == "" {
		provider = "Provider"
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = defaultMessage(e.Kind, provider)
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s (%s, HTTP %d)", msg, provider, e.StatusCode)
	}
	return fmt.Sprintf("%s (%s)", msg, provider)
}

func (e *Error) WithDetail(detail string) *Error {
	e.Detail = strings.TrimSpace(detail)
	return e
}

func New(provider, kind, message string) *Error {
	return &Error{
		Provider: strings.TrimSpace(provider),
		Kind:     strings.TrimSpace(kind),
		Message:  strings.TrimSpace(message),
	}
}

func FromHTTP(provider string, status int, detail string) *Error {
	kind := KindUnknown
	message := ""
	lower := strings.ToLower(detail)

	switch status {
	case 400:
		kind = KindBadRequest
		message = "The provider rejected the request. Check the selected model and token limit."
	case 401, 403:
		kind = KindAuthentication
		message = "The provider rejected the API key. Check or replace the key in Settings."
	case 404:
		kind = KindModel
		message = "The selected model was not found or is not available for this key."
	case 408:
		kind = KindNetwork
		message = "The provider request timed out."
	case 429:
		kind = KindRateLimit
		message = "The provider is rate-limiting requests. Wait a bit or lower concurrency."
	case 402:
		kind = KindQuota
		message = "The provider account has no available credits or quota."
	case 500, 502, 503, 504, 529:
		kind = KindUnavailable
		message = "The provider is temporarily unavailable."
	default:
		if strings.Contains(lower, "quota") || strings.Contains(lower, "credit") || strings.Contains(lower, "balance") {
			kind = KindQuota
			message = "The provider account has no available credits or quota."
		} else if strings.Contains(lower, "model") {
			kind = KindModel
			message = "The selected model was rejected by the provider."
		} else {
			message = "The provider request failed."
		}
	}

	return &Error{
		Provider:   strings.TrimSpace(provider),
		Kind:       kind,
		StatusCode: status,
		Message:    message,
		Detail:     strings.TrimSpace(detail),
	}
}

func defaultMessage(kind, provider string) string {
	switch kind {
	case KindAuthentication:
		return "The provider rejected the API key. Check or replace the key in Settings."
	case KindRateLimit:
		return "The provider is rate-limiting requests. Wait a bit or lower concurrency."
	case KindQuota:
		return "The provider account has no available credits or quota."
	case KindModel:
		return "The selected model was not found or is not available for this key."
	case KindNetwork:
		return "The provider request could not reach " + provider + "."
	case KindUnavailable:
		return "The provider is temporarily unavailable."
	default:
		return "The provider request failed."
	}
}
