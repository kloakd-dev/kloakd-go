package kloakd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Defaults for following a request that the API hands off as a job.
const (
	DefaultPollInterval      = 3 * time.Second
	DefaultPollTimeout       = 15 * time.Minute
	maxPollInterval          = 15 * time.Second
	pollBackoffFactor        = 1.5
	DefaultFetchPollInterval = DefaultPollInterval
	DefaultFetchPollTimeout  = DefaultPollTimeout
)

// PollOptions bounds the following of a handed-off request. Zero values
// mean the defaults (3 s first interval, 15 min overall).
type PollOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

func (p PollOptions) or(fallback PollOptions) PollOptions {
	if p.Interval <= 0 {
		p.Interval = fallback.Interval
	}
	if p.Timeout <= 0 {
		p.Timeout = fallback.Timeout
	}
	return p
}

func (p PollOptions) effective() PollOptions {
	return p.or(PollOptions{Interval: DefaultPollInterval, Timeout: DefaultPollTimeout})
}

// postFollowing posts to a route that may hand off to a job: past the
// synchronous limit (about 85 s; Cloudflare closes requests at 100 s) the API
// answers 202 with {job_id, poll_url}. The job is then polled until it ends
// and its result (the body a direct 200 would have had) is returned. A failed
// job is an *ApiError with Code "job_failed"; a job still running at the
// poll timeout is Code "job_timeout"; an expired job (poll 404) is "job_failed".
func (t *transport) postFollowing(ctx context.Context, path string, body map[string]interface{}, poll *PollOptions) (map[string]interface{}, error) {
	status, raw, err := t.do(ctx, http.MethodPost, path, body, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted {
		return raw, nil
	}
	jobID, _ := raw["job_id"].(string)
	pollURL, _ := raw["poll_url"].(string)
	if jobID == "" || pollURL == "" {
		return nil, &ApiError{KloakdError: KloakdError{StatusCode: status, Code: "unexpected_handoff",
			Message: fmt.Sprintf("POST %s answered 202 without job_id and poll_url", path)}, JobID: jobID}
	}
	opts := t.poll
	if poll != nil {
		opts = poll.or(t.poll)
	}
	return t.followJob(ctx, jobID, pollURL, opts.effective())
}

// resolvePollURL resolves poll_url against the base URL; it must stay on the
// API's host (the request carries the API key).
func (t *transport) resolvePollURL(pollURL string) (string, error) {
	base, err := url.Parse(t.baseURL + "/")
	if err != nil {
		return "", fmt.Errorf("kloakd: parse base URL: %w", err)
	}
	ref, err := url.Parse(pollURL)
	if err != nil {
		return "", fmt.Errorf("kloakd: parse poll_url %q: %w", pollURL, err)
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return "", fmt.Errorf("kloakd: poll_url %q is not on the API host", pollURL)
	}
	return resolved.String(), nil
}

// followJob polls pollURL with backoff until the job completes, fails,
// expires or the timeout passes.
func (t *transport) followJob(ctx context.Context, jobID, pollURL string, opts PollOptions) (map[string]interface{}, error) {
	reqURL, err := t.resolvePollURL(pollURL)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(opts.Timeout)
	interval := opts.Interval
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		_, job, err := t.doURL(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			var apiErr *ApiError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
				return nil, jobError("job_failed", http.StatusNotFound,
					fmt.Sprintf("job %s not found or expired: %s", jobID, apiErr.Message), jobID)
			}
			return nil, err
		}
		switch strField(job, "status") {
		case "completed":
			return mapField(job, "result"), nil
		case "failed":
			msg := strField(job, "error")
			if msg == "" {
				msg = fmt.Sprintf("job %s failed", jobID)
			}
			return nil, jobError("job_failed", http.StatusOK, msg, jobID)
		}
		if time.Now().After(deadline) {
			return nil, jobError("job_timeout", 0,
				fmt.Sprintf("job %s still running after %s", jobID, opts.Timeout), jobID)
		}
		interval = nextPollInterval(interval)
	}
}

// nextPollInterval is the wait after d: 1.5 times longer, at most 15 s.
func nextPollInterval(d time.Duration) time.Duration {
	next := time.Duration(float64(d) * pollBackoffFactor)
	if next > maxPollInterval {
		return maxPollInterval
	}
	return next
}

func jobError(code string, status int, msg, jobID string) error {
	return &ApiError{KloakdError: KloakdError{StatusCode: status, Message: msg, Code: code}, JobID: jobID}
}
