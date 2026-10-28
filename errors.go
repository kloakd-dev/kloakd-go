// Package kloakd provides the Go SDK for the KLOAKD API.
package kloakd

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

// NotEntitledError is returned for HTTP 403 with code "not_entitled" (the
// organisation's plan does not include the module) or "tier_access_denied"
// (the tier does not include the feature; Feature, CurrentTier and
// RequiredTier say which).
type NotEntitledError struct {
	KloakdError
	Module       string
	Feature      string
	CurrentTier  string
	RequiredTier string
	UpgradeURL   string
}

// As makes a NotEntitledError also a *ForbiddenError for errors.As: every
// NotEntitledError is a 403.
func (e *NotEntitledError) As(target interface{}) bool {
	if fe, ok := target.(**ForbiddenError); ok {
		*fe = &ForbiddenError{KloakdError: e.KloakdError}
		return true
	}
	return false
}

func (e *NotEntitledError) Error() string {
	what := e.Module
	if e.Feature != "" {
		what = e.Feature
	}
	return fmt.Sprintf("NotEntitledError(%d): %s (%s)", e.StatusCode, e.Message, what)
}

// ForbiddenError is returned for any other HTTP 403: the key may not do
// this (for example another organisation's resource). errors.As with a
// *ForbiddenError also matches a NotEntitledError.
type ForbiddenError struct {
	KloakdError
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("ForbiddenError(%d): %s", e.StatusCode, e.Message)
}

// ConcurrencyLimitError is returned for HTTP 409 "concurrent_limit": the
// organisation already runs as many crawls or jobs as its tier allows.
// Retry when one finishes.
type ConcurrencyLimitError struct {
	KloakdError
	Tier            string
	ConcurrentLimit int
	ActiveCrawls    int
	TestKeyID       string
}

func (e *ConcurrencyLimitError) Error() string {
	return fmt.Sprintf("ConcurrencyLimitError(%d): %s (limit=%d)", e.StatusCode, e.Message, e.ConcurrentLimit)
}

// RateLimitError is returned for HTTP 429 "rate_limited". For the tier's
// request rate, RetryAfter (seconds) and ResetAt say when to retry; the SDK
// retries it itself up to Config.MaxRetries. QuotaExhausted is true for the
// playground key's permanent quota (retry_after 0, no reset): waiting does
// not clear it, so it is not retried; QuotaUsed, QuotaLimit and UpgradeURL
// describe it.
type RateLimitError struct {
	KloakdError
	RetryAfter     int    // seconds
	ResetAt        string // ISO timestamp
	QuotaExhausted bool
	QuotaUsed      int
	QuotaLimit     int
	UpgradeURL     string
}

func (e *RateLimitError) Error() string {
	if e.QuotaExhausted {
		return fmt.Sprintf("RateLimitError(%d): %s (playground quota %d/%d)", e.StatusCode, e.Message, e.QuotaUsed, e.QuotaLimit)
	}
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
	Tier               string
	UpgradeURL         string
	GBUsed             float64
	GBLimit            float64
	PurchasedGBBalance float64
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("QuotaExceededError(%d): %s", e.StatusCode, e.Message)
}

// BillingUnavailableError is returned for HTTP 503 "billing_unavailable":
// usage could not be checked, so the request was not started. The SDK
// retries it itself (on any method) up to Config.MaxRetries.
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

// ApiError is returned for any other 4xx/5xx response. It is also the error
// of a handed-off job that failed or expired (Code "job_failed") or was still
// running at the poll timeout (Code "job_timeout", and "crawl_timeout" for
// Webgrph.Crawl); JobID names the job (or crawl), so the wait can be resumed
// with Client.WaitForHandoff (or Webgrph.GetCrawlStatus).
type ApiError struct {
	KloakdError
	JobID string
}

func (e *ApiError) Error() string {
	if e.JobID != "" {
		return fmt.Sprintf("ApiError(%d): %s (job=%s)", e.StatusCode, e.Message, e.JobID)
	}
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

// validationMessage joins a 422's detail[] ({loc, msg}) into one message.
func validationMessage(items []interface{}) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			parts = append(parts, fmt.Sprintf("%v", item))
			continue
		}
		var loc []string
		if l, ok := m["loc"].([]interface{}); ok {
			for _, p := range l {
				if s := fmt.Sprintf("%v", p); s != "body" {
					loc = append(loc, s)
				}
			}
		}
		msg, _ := m["msg"].(string)
		if len(loc) > 0 {
			msg = strings.Join(loc, ".") + ": " + msg
		}
		parts = append(parts, msg)
	}
	return strings.Join(parts, "; ")
}

// errorMessage reads the message of an error answer: detail.message, a
// string detail, a 422's detail[], a top-level message, or the body's text
// when it is not JSON.
func errorMessage(statusCode int, body map[string]interface{}, raw []byte) string {
	if msg, _ := detailMap(body)["message"].(string); msg != "" {
		return msg
	}
	switch d := body["detail"].(type) {
	case string:
		if d != "" {
			return d
		}
	case []interface{}:
		if msg := validationMessage(d); msg != "" {
			return msg
		}
	}
	if m, ok := body["message"].(string); ok && m != "" {
		return m
	}
	if len(body) == 0 {
		if text := strings.TrimSpace(string(raw)); text != "" {
			const maxLen = 500
			if len(text) > maxLen {
				text = text[:maxLen] + "…"
			}
			return text
		}
	}
	return fmt.Sprintf("HTTP %d", statusCode)
}

// raiseForStatus maps an HTTP status code to the appropriate SDK error.
// Returns nil if the status is successful (2xx). The gateway's errors carry
// {"detail": {"error": <code>, "message": <text>, ...}}; raw is the body as
// received (its text is the message when it is not JSON).
func raiseForStatus(statusCode int, body map[string]interface{}, headers http.Header, raw []byte) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	detail := detailMap(body)
	code, _ := detail["error"].(string)
	base := KloakdError{StatusCode: statusCode, Message: errorMessage(statusCode, body, raw), Code: code}
	str := func(key string) string {
		for _, m := range []map[string]interface{}{detail, body} {
			if v, ok := m[key]; ok && v != nil {
				return fmt.Sprintf("%v", v)
			}
		}
		return ""
	}
	num := func(key string) float64 {
		if n := numField(detail, key); n != 0 {
			return n
		}
		return numField(body, key)
	}
	retryAfter := func(def int) int {
		if headers != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(headers.Get("Retry-After"))); err == nil && n > 0 {
				return n
			}
		}
		if n := num("retry_after"); n > 0 {
			return int(n)
		}
		return def
	}

	switch {
	case statusCode == 401:
		return &AuthenticationError{KloakdError: base}
	case statusCode == 402:
		return &PaymentRequiredError{KloakdError: base, UpgradeURL: str("upgrade_url")}
	case statusCode == 403 && (code == "not_entitled" || code == "tier_access_denied"):
		return &NotEntitledError{
			KloakdError:  base,
			Module:       str("module"),
			Feature:      str("feature"),
			CurrentTier:  str("current_tier"),
			RequiredTier: str("required_tier"),
			UpgradeURL:   str("upgrade_url"),
		}
	case statusCode == 403:
		return &ForbiddenError{KloakdError: base}
	case statusCode == 409 && code == "concurrent_limit":
		return &ConcurrencyLimitError{
			KloakdError:     base,
			Tier:            str("tier"),
			ConcurrentLimit: int(num("concurrent_limit")),
			ActiveCrawls:    int(num("active_crawls")),
			TestKeyID:       str("test_key_id"),
		}
	case statusCode == 429 && code == "gb_exhausted":
		return &QuotaExceededError{
			KloakdError:        base,
			Tier:               str("tier"),
			UpgradeURL:         str("upgrade_url"),
			GBUsed:             numField(detail, "residential_gb_used"),
			GBLimit:            numField(detail, "residential_gb_limit"),
			PurchasedGBBalance: numField(detail, "purchased_gb_balance"),
		}
	case statusCode == 429:
		// The playground quota is a rate_limited answer with retry_after 0
		// and no reset (kernel/gateway/rate_limiter.py): it does not clear.
		_, hasRetryAfter := detail["retry_after"]
		if hasRetryAfter && num("retry_after") == 0 && str("reset_at") == "" {
			return &RateLimitError{
				KloakdError:    base,
				QuotaExhausted: true,
				QuotaUsed:      int(num("quota_used")),
				QuotaLimit:     int(num("quota_limit")),
				UpgradeURL:     str("upgrade_url"),
			}
		}
		return &RateLimitError{KloakdError: base, RetryAfter: retryAfter(60), ResetAt: str("reset_at"),
			QuotaUsed: int(num("quota_used")), QuotaLimit: int(num("quota_limit")), UpgradeURL: str("upgrade_url")}
	case statusCode == 503 && code == "billing_unavailable":
		return &BillingUnavailableError{KloakdError: base, RetryAfter: retryAfter(30)}
	case statusCode == 502:
		return &UpstreamError{KloakdError: base}
	default:
		return &ApiError{KloakdError: base}
	}
}
