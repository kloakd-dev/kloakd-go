package kloakd

import (
	"errors"
	"testing"
)

func TestRaiseForStatus_Success(t *testing.T) {
	err := raiseForStatus(200, map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected nil for 200, got %v", err)
	}
	err = raiseForStatus(201, map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected nil for 201, got %v", err)
	}
}

func TestRaiseForStatus_401(t *testing.T) {
	err := raiseForStatus(401, map[string]interface{}{"detail": "invalid key"})
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
	if authErr.StatusCode != 401 {
		t.Errorf("expected StatusCode 401, got %d", authErr.StatusCode)
	}
	if authErr.Message != "invalid key" {
		t.Errorf("unexpected message: %s", authErr.Message)
	}
}

func TestRaiseForStatus_403(t *testing.T) {
	err := raiseForStatus(403, map[string]interface{}{
		"detail":      "not entitled",
		"module":      "webgrph",
		"upgrade_url": "https://kloakd.dev/upgrade",
	})
	var netErr *NotEntitledError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected NotEntitledError, got %T: %v", err, err)
	}
	if netErr.Module != "webgrph" {
		t.Errorf("expected module=webgrph, got %s", netErr.Module)
	}
}

func TestRaiseForStatus_429(t *testing.T) {
	err := raiseForStatus(429, map[string]interface{}{
		"detail":      "rate limited",
		"retry_after": float64(30),
		"reset_at":    "2026-04-10T00:00:00Z",
	})
	var rlErr *RateLimitError
	if !errors.As(err, &rlErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rlErr.RetryAfter != 30 {
		t.Errorf("expected RetryAfter=30, got %d", rlErr.RetryAfter)
	}
	if rlErr.ResetAt != "2026-04-10T00:00:00Z" {
		t.Errorf("unexpected ResetAt: %s", rlErr.ResetAt)
	}
}

func TestRaiseForStatus_502(t *testing.T) {
	err := raiseForStatus(502, map[string]interface{}{"detail": "upstream failed"})
	var upErr *UpstreamError
	if !errors.As(err, &upErr) {
		t.Fatalf("expected UpstreamError, got %T: %v", err, err)
	}
}

func TestRaiseForStatus_500(t *testing.T) {
	err := raiseForStatus(500, map[string]interface{}{"detail": "server error"})
	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected StatusCode 500, got %d", apiErr.StatusCode)
	}
}

func TestRaiseForStatus_FallbackMessage(t *testing.T) {
	err := raiseForStatus(404, map[string]interface{}{})
	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if apiErr.Message == "" {
		t.Error("expected non-empty message fallback")
	}
}

func TestIsRetryable(t *testing.T) {
	for _, code := range []int{429, 500, 502, 503, 504} {
		if !isRetryable(code) {
			t.Errorf("expected %d to be retryable", code)
		}
	}
	for _, code := range []int{200, 400, 401, 403, 404} {
		if isRetryable(code) {
			t.Errorf("expected %d to NOT be retryable", code)
		}
	}
}

func TestErrorMessages(t *testing.T) {
	authErr := &AuthenticationError{KloakdError: KloakdError{StatusCode: 401, Message: "bad key"}}
	if authErr.Error() == "" {
		t.Error("AuthenticationError.Error() returned empty string")
	}
	rlErr := &RateLimitError{KloakdError: KloakdError{StatusCode: 429, Message: "slow down"}, RetryAfter: 5}
	if rlErr.Error() == "" {
		t.Error("RateLimitError.Error() returned empty string")
	}
	netErr := &NotEntitledError{KloakdError: KloakdError{StatusCode: 403, Message: "no plan"}, Module: "evadr"}
	if netErr.Error() == "" {
		t.Error("NotEntitledError.Error() returned empty string")
	}
	upErr := &UpstreamError{KloakdError: KloakdError{StatusCode: 502, Message: "upstream down"}}
	if upErr.Error() == "" {
		t.Error("UpstreamError.Error() returned empty string")
	}
	apiErr := &ApiError{KloakdError: KloakdError{StatusCode: 422, Message: "unprocessable"}}
	if apiErr.Error() == "" {
		t.Error("ApiError.Error() returned empty string")
	}
}
