// Package kloakd provides the Go SDK for the KLOAKD API.
// Error hierarchy mirrors the design document §6.
package kloakd

import "fmt"

// KloakdError is the base error type for all SDK errors.
// Every error carries StatusCode and Message.
type KloakdError struct {
	StatusCode int
	Message    string
}

func (e *KloakdError) Error() string {
	return fmt.Sprintf("KloakdError(%d): %s", e.StatusCode, e.Message)
}

// AuthenticationError is returned for HTTP 401 — invalid or expired API key.
type AuthenticationError struct {
	KloakdError
}

func (e *AuthenticationError) Error() string {
	return fmt.Sprintf("AuthenticationError(%d): %s", e.StatusCode, e.Message)
}

// NotEntitledError is returned for HTTP 403 — tenant not on required plan.
type NotEntitledError struct {
	KloakdError
	Module     string
	UpgradeURL string
}

func (e *NotEntitledError) Error() string {
	return fmt.Sprintf("NotEntitledError(%d): %s (module=%s)", e.StatusCode, e.Message, e.Module)
}

// RateLimitError is returned for HTTP 429 — quota exceeded.
type RateLimitError struct {
	KloakdError
	RetryAfter int    // seconds
	ResetAt    string // ISO timestamp
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("RateLimitError(%d): %s (retry_after=%ds)", e.StatusCode, e.Message, e.RetryAfter)
}

// UpstreamError is returned for HTTP 502 — upstream site fetch failed.
type UpstreamError struct {
	KloakdError
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("UpstreamError(%d): %s", e.StatusCode, e.Message)
}

// ApiError is returned for any other 4xx/5xx response.
type ApiError struct {
	KloakdError
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("ApiError(%d): %s", e.StatusCode, e.Message)
}

// raiseForStatus maps an HTTP status code to the appropriate SDK error.
// Returns nil if the status is successful (2xx).
func raiseForStatus(statusCode int, body map[string]interface{}) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	msg := ""
	if d, ok := body["detail"]; ok {
		msg = fmt.Sprintf("%v", d)
	} else if m, ok := body["message"]; ok {
		msg = fmt.Sprintf("%v", m)
	}
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", statusCode)
	}

	switch statusCode {
	case 401:
		return &AuthenticationError{KloakdError: KloakdError{StatusCode: statusCode, Message: msg}}
	case 403:
		module := ""
		upgradeURL := ""
		if v, ok := body["module"]; ok {
			module = fmt.Sprintf("%v", v)
		}
		if v, ok := body["upgrade_url"]; ok {
			upgradeURL = fmt.Sprintf("%v", v)
		}
		return &NotEntitledError{
			KloakdError: KloakdError{StatusCode: statusCode, Message: msg},
			Module:      module,
			UpgradeURL:  upgradeURL,
		}
	case 429:
		retryAfter := 60
		resetAt := ""
		if v, ok := body["retry_after"]; ok {
			switch n := v.(type) {
			case float64:
				retryAfter = int(n)
			case int:
				retryAfter = n
			}
		}
		if v, ok := body["reset_at"]; ok {
			resetAt = fmt.Sprintf("%v", v)
		}
		return &RateLimitError{
			KloakdError: KloakdError{StatusCode: statusCode, Message: msg},
			RetryAfter:  retryAfter,
			ResetAt:     resetAt,
		}
	case 502:
		return &UpstreamError{KloakdError: KloakdError{StatusCode: statusCode, Message: msg}}
	default:
		return &ApiError{KloakdError: KloakdError{StatusCode: statusCode, Message: msg}}
	}
}

// isRetryable reports whether the given status code is eligible for retry.
func isRetryable(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504:
		return true
	}
	return false
}
