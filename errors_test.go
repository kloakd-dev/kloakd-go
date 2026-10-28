package kloakd

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestRaiseForStatus_Success(t *testing.T) {
	err := raiseForStatus(200, map[string]interface{}{}, nil, nil)
	if err != nil {
		t.Fatalf("expected nil for 200, got %v", err)
	}
	err = raiseForStatus(201, map[string]interface{}{}, nil, nil)
	if err != nil {
		t.Fatalf("expected nil for 201, got %v", err)
	}
}

func TestRaiseForStatus_401(t *testing.T) {
	err := raiseForStatus(401, map[string]interface{}{"detail": "invalid key"}, nil, nil)
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
	err := raiseForStatus(403, map[string]interface{}{"detail": map[string]interface{}{
		"error":       "not_entitled",
		"message":     "not entitled",
		"module":      "webgrph",
		"upgrade_url": "https://kloakd.dev/upgrade",
	}}, nil, nil)
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
	}, nil, nil)
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
	err := raiseForStatus(502, map[string]interface{}{"detail": "upstream failed"}, nil, nil)
	var upErr *UpstreamError
	if !errors.As(err, &upErr) {
		t.Fatalf("expected UpstreamError, got %T: %v", err, err)
	}
}

func TestRaiseForStatus_500(t *testing.T) {
	err := raiseForStatus(500, map[string]interface{}{"detail": "server error"}, nil, nil)
	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected StatusCode 500, got %d", apiErr.StatusCode)
	}
}

func TestRaiseForStatus_FallbackMessage(t *testing.T) {
	err := raiseForStatus(404, map[string]interface{}{}, nil, nil)
	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if apiErr.Message == "" {
		t.Error("expected non-empty message fallback")
	}
}

func TestShouldRetry_ByMethodAndError(t *testing.T) {
	apiErr := func(code int) error { return &ApiError{KloakdError: KloakdError{StatusCode: code}} }
	for _, code := range []int{500, 502, 503, 504} {
		for _, m := range []string{"GET", "HEAD", "DELETE"} {
			if !shouldRetry(m, apiErr(code)) {
				t.Errorf("%s %d should be retried", m, code)
			}
		}
		for _, m := range []string{"POST", "PUT", "PATCH"} {
			if shouldRetry(m, apiErr(code)) {
				t.Errorf("%s %d must not be retried", m, code)
			}
		}
	}
	if shouldRetry("GET", &UpstreamError{}) == false || shouldRetry("POST", &UpstreamError{}) {
		t.Error("502 UpstreamError: retried for GET only")
	}
	for _, m := range []string{"GET", "POST", "PATCH"} {
		if !shouldRetry(m, &RateLimitError{}) {
			t.Errorf("%s rate_limited should be retried", m)
		}
		if shouldRetry(m, &RateLimitError{QuotaExhausted: true}) {
			t.Errorf("%s playground quota must not be retried", m)
		}
		if !shouldRetry(m, &BillingUnavailableError{}) {
			t.Errorf("%s billing_unavailable should be retried", m)
		}
		for _, e := range []error{&QuotaExceededError{}, &PaymentRequiredError{}, &AuthenticationError{},
			&NotEntitledError{}, &ForbiddenError{}, &ConcurrencyLimitError{}, apiErr(400), apiErr(404), apiErr(422)} {
			if shouldRetry(m, e) {
				t.Errorf("%s %T must not be retried", m, e)
			}
		}
	}
}

func TestRaiseForStatus_403ByCode(t *testing.T) {
	err := raiseForStatus(403, detail("tier_access_denied", "Upgrade to use this", map[string]interface{}{
		"current_tier": "playground", "required_tier": "pro", "feature": "fetchyr", "upgrade_url": "/pricing",
	}), nil, nil)
	var ne *NotEntitledError
	if !errors.As(err, &ne) || ne.Feature != "fetchyr" || ne.CurrentTier != "playground" || ne.RequiredTier != "pro" || ne.UpgradeURL != "/pricing" {
		t.Fatalf("expected NotEntitledError with the tier fields, got %T %+v", err, err)
	}
	err = raiseForStatus(403, map[string]interface{}{"detail": "Not your crawl"}, nil, nil)
	var fe *ForbiddenError
	if !errors.As(err, &fe) || fe.Message != "Not your crawl" {
		t.Fatalf("a 403 without an entitlement code is a ForbiddenError, got %T %v", err, err)
	}
	var ne2 *NotEntitledError
	if errors.As(err, &ne2) {
		t.Fatal("a plain 403 must not be a NotEntitledError")
	}
	err = raiseForStatus(403, detail("not_entitled", "no module", map[string]interface{}{"module": "nexus"}), nil, nil)
	if !errors.As(err, &fe) || fe.StatusCode != 403 || fe.Code != "not_entitled" {
		t.Fatalf("a NotEntitledError is also a ForbiddenError, got %v", err)
	}
}

func TestRaiseForStatus_409ConcurrentLimit(t *testing.T) {
	err := raiseForStatus(409, detail("concurrent_limit", "Concurrent crawl limit reached", map[string]interface{}{
		"tier": "pro", "concurrent_limit": float64(10), "active_crawls": float64(10),
	}), nil, nil)
	var ce *ConcurrencyLimitError
	if !errors.As(err, &ce) || ce.Tier != "pro" || ce.ConcurrentLimit != 10 || ce.ActiveCrawls != 10 {
		t.Fatalf("expected ConcurrencyLimitError, got %T %+v", err, err)
	}
	var ae *ApiError
	if !errors.As(raiseForStatus(409, map[string]interface{}{"detail": "exists"}, nil, nil), &ae) {
		t.Fatal("another 409 stays an ApiError")
	}
}

func TestRaiseForStatus_429QuotaExhausted(t *testing.T) {
	err := raiseForStatus(429, detail("rate_limited", "Rate limit exceeded. Retry after 0s.", map[string]interface{}{
		"retry_after": float64(0), "reset_at": "", "quota_used": float64(50), "quota_limit": float64(50),
		"upgrade_url": "https://app.kloakd.dev/pricing",
	}), http.Header{"Retry-After": []string{"0"}}, nil)
	var rl *RateLimitError
	if !errors.As(err, &rl) || !rl.QuotaExhausted || rl.QuotaUsed != 50 || rl.QuotaLimit != 50 || rl.RetryAfter != 0 || rl.UpgradeURL == "" {
		t.Fatalf("expected the playground quota, got %T %+v", err, err)
	}
	err = raiseForStatus(429, detail("rate_limited", "slow down", map[string]interface{}{
		"retry_after": float64(12), "reset_at": "2026-10-28T12:00:00Z",
	}), http.Header{"Retry-After": []string{"12"}}, nil)
	if !errors.As(err, &rl) || rl.QuotaExhausted || rl.RetryAfter != 12 || rl.ResetAt != "2026-10-28T12:00:00Z" {
		t.Fatalf("expected the tier rate limit, got %+v", err)
	}
}

func TestRaiseForStatus_QuotaExceededTier(t *testing.T) {
	err := raiseForStatus(429, detail("gb_exhausted", "limit reached", map[string]interface{}{"tier": "developer"}), nil, nil)
	var q *QuotaExceededError
	if !errors.As(err, &q) || q.Tier != "developer" {
		t.Fatalf("expected QuotaExceededError with Tier, got %+v", err)
	}
}

func TestRaiseForStatus_NonJSONBody(t *testing.T) {
	raw := []byte("<html>502 Bad Gateway</html>")
	err := raiseForStatus(502, decodeBody(raw), nil, raw)
	var up *UpstreamError
	if !errors.As(err, &up) || up.Message != "<html>502 Bad Gateway</html>" {
		t.Fatalf("expected the body text as the message, got %T %v", err, err)
	}
	err = raiseForStatus(500, decodeBody(nil), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("an empty body falls back to the status, got %v", err)
	}
}

func TestRaiseForStatus_422KeepsTheValidationMessages(t *testing.T) {
	body := map[string]interface{}{"detail": []interface{}{
		map[string]interface{}{"loc": []interface{}{"body", "query_type"}, "msg": "Field required", "type": "missing"},
		map[string]interface{}{"loc": []interface{}{"body", "parameters", float64(0)}, "msg": "Input should be a valid string"},
	}}
	err := raiseForStatus(422, body, nil, nil)
	want := "query_type: Field required; parameters.0: Input should be a valid string"
	var ae *ApiError
	if !errors.As(err, &ae) || ae.Message != want {
		t.Fatalf("expected %q, got %v", want, err)
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
