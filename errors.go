// Package kloakd provides the Go SDK for the KLOAKD API.
// Error hierarchy mirrors the design document §6.
package kloakd

import (
	"fmt"
	"net/http"
	"strconv"
)

// KloakdError is the base error type for all SDK errors.
// Every error carries StatusCode and Message.
type KloakdError struct {
	StatusCode int
	Message    string
	// Code is the API's error word (detail.error), e.g. "gb_exhausted".
	Code string
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

// PaymentRequiredError is returned for HTTP 402: the organisation's paid
// plan has a failed payment; new billable work is refused until it is paid.
type PaymentRequiredError struct {
	KloakdError
	UpgradeURL string
}

func (e *PaymentRequiredError) Error() string {
	return fmt.Sprintf("PaymentRequiredError(%d): %s", e.StatusCode, e.Message)
}

// QuotaExceededError is returned for HTTP 429 "gb_exhausted": the plan's
// GB allocation and this period's top-ups are used up. Retrying does not
// help; buy a top-up or upgrade.
type QuotaExceededError struct {
	KloakdError
	UpgradeURL         string
	GBUsed             float64
	GBLimit            float64
	PurchasedGBBalance float64
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("QuotaExceededError(%d): %s", e.StatusCode, e.Message)
}

// BillingUnavailableError is returned for HTTP 503 "billing_unavailable":
// usage could not be checked, so the request was not started. Retry after
// RetryAfter seconds.
type BillingUnavailableError struct {
	KloakdError
	RetryAfter int
}

func (e *BillingUnavailableError) Error() string {
	return fmt.Sprintf("BillingUnavailableError(%d): %s (retry_after=%ds)", e.StatusCode, e.Message, e.RetryAfter)
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

// detailMap returns body["detail"] when the API sent a structured detail.
func detailMap(body map[string]interface{}) map[string]interface{} {
	if d, ok := body["detail"].(map[string]interface{}); ok {
		return d
	}
	return map[string]interface{}{}
}

func numField(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

// raiseForStatus maps an HTTP status code to the appropriate SDK error.
// Returns nil if the status is successful (2xx). The gateway's errors carry
// {"detail": {"error": <code>, "message": <text>, ...}}.
func raiseForStatus(statusCode int, body map[string]interface{}, headers http.Header) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	detail := detailMap(body)
	code, _ := detail["error"].(string)
	msg, _ := detail["message"].(string)
	if msg == "" {
		if d, ok := body["detail"].(string); ok {
			msg = d
		} else if m, ok := body["message"].(string); ok {
			msg = m
		} else if d, ok := body["detail"]; ok {
			msg = fmt.Sprintf("%v", d)
		}
	}
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", statusCode)
	}
	base := KloakdError{StatusCode: statusCode, Message: msg, Code: code}
	str := func(key string) string {
		if v, ok := detail[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
		if v, ok := body[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
	retryAfter := func(def int) int {
		if headers != nil {
			if n, err := strconv.Atoi(headers.Get("Retry-After")); err == nil && n > 0 {
				return n
			}
		}
		for _, m := range []map[string]interface{}{detail, body} {
			if n := numField(m, "retry_after"); n > 0 {
				return int(n)
			}
		}
		return def
	}

	switch {
	case statusCode == 401:
		return &AuthenticationError{KloakdError: base}
	case statusCode == 402:
		return &PaymentRequiredError{KloakdError: base, UpgradeURL: str("upgrade_url")}
	case statusCode == 403:
		return &NotEntitledError{KloakdError: base, Module: str("module"), UpgradeURL: str("upgrade_url")}
	case statusCode == 429 && code == "gb_exhausted":
		return &QuotaExceededError{
			KloakdError:        base,
			UpgradeURL:         str("upgrade_url"),
			GBUsed:             numField(detail, "residential_gb_used"),
			GBLimit:            numField(detail, "residential_gb_limit"),
			PurchasedGBBalance: numField(detail, "purchased_gb_balance"),
		}
	case statusCode == 429:
		return &RateLimitError{KloakdError: base, RetryAfter: retryAfter(60), ResetAt: str("reset_at")}
	case statusCode == 503 && code == "billing_unavailable":
		return &BillingUnavailableError{KloakdError: base, RetryAfter: retryAfter(30)}
	case statusCode == 502:
		return &UpstreamError{KloakdError: base}
	default:
		return &ApiError{KloakdError: base}
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

// isRetryableError is isRetryable, except that a used-up GB quota is not
// retried (it does not clear by waiting).
func isRetryableError(err error) bool {
	switch e := err.(type) {
	case *QuotaExceededError, *PaymentRequiredError:
		return false
	case *RateLimitError:
		return true
	case *BillingUnavailableError:
		return true
	case *UpstreamError:
		return isRetryable(e.StatusCode)
	case *ApiError:
		return isRetryable(e.StatusCode)
	}
	return false
}
