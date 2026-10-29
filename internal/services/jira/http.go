package jira

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	requestTimeout  = 30 * time.Second
	maxReadAttempts = 3
	maxRetryWait    = 30 * time.Second
)

// WithContext returns an independent client view sharing the HTTP transport.
func (c *Client) WithContext(ctx context.Context) *Client {
	copyClient := *c
	if ctx == nil {
		ctx = context.Background()
	}
	copyClient.ctx = ctx
	return &copyClient
}

// Context is the cancellation/deadline scope for requests made through this client.
func (c *Client) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

type apiStatusError struct {
	status     int
	retryAfter string
}

func (e *apiStatusError) Error() string { return fmt.Sprintf("API returned status %d", e.status) }

type requestFailure struct{ cause error }

func (e *requestFailure) Error() string {
	var status *apiStatusError
	if errors.As(e.cause, &status) {
		return status.Error()
	}
	if errors.Is(e.cause, context.Canceled) {
		return "Jira request canceled"
	}
	var timeout net.Error
	if errors.Is(e.cause, context.DeadlineExceeded) || (errors.As(e.cause, &timeout) && timeout.Timeout()) {
		return "Jira request timed out"
	}
	return "Jira request failed"
}
func (e *requestFailure) Unwrap() error { return e.cause }

func (c *Client) request(ctx context.Context, method, endpoint string, body []byte, accept string, buffered bool) (*http.Response, error) {
	attempts := 1
	if method == http.MethodGet && buffered {
		attempts = maxReadAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("invalid Jira request")
		}
		req.SetBasicAuth(c.credentials.Username, c.credentials.Password)
		req.Header.Set("Accept", accept)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, requestErr := c.httpClient.Do(req)
		var failure error
		retryAfter := ""
		retryable := false
		switch {
		case requestErr != nil:
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			failure, retryable = &requestFailure{cause: requestErr}, transientReadError(requestErr)
		case response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated:
			retryAfter = response.Header.Get("Retry-After")
			_ = response.Body.Close()
			failure = &apiStatusError{status: response.StatusCode, retryAfter: retryAfter}
			retryable = retryableStatus(response.StatusCode)
		case buffered:
			data, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil {
				response.Body = io.NopCloser(bytes.NewReader(data))
				return response, nil
			}
			failure, retryable = &requestFailure{cause: readErr}, transientReadError(readErr)
		default:
			return response, nil
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if !retryable || attempt+1 == attempts {
			return nil, failure
		}
		if waitErr := waitReadRetry(ctx, failure, retryAfter, attempt); waitErr != nil {
			return nil, waitErr
		}
	}
	return nil, fmt.Errorf("jira read retries exhausted")
}

func waitReadRetry(ctx context.Context, failure error, header string, attempt int) error {
	delay, err := readRetryDelay(header, attempt, time.Now())
	if err != nil {
		return fmt.Errorf("%w; retry delay exceeds the 30s wait budget", failure)
	}
	if deadline, ok := ctx.Deadline(); ok && delay >= time.Until(deadline) {
		return fmt.Errorf("%w; retry delay exceeds the remaining command budget", failure)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryableFailure(failure error) (string, bool) {
	var status *apiStatusError
	if errors.As(failure, &status) {
		return status.retryAfter, retryableStatus(status.status)
	}
	return "", transientReadError(failure)
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func transientReadError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsTemporary {
		return true
	}
	for _, transient := range []error{io.EOF, io.ErrUnexpectedEOF, syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.EPIPE} {
		if errors.Is(err, transient) {
			return true
		}
	}
	return false
}

func readRetryDelay(header string, attempt int, now time.Time) (time.Duration, error) {
	delay := 200 * time.Millisecond * time.Duration(1<<attempt)
	header = strings.TrimSpace(header)
	seconds, parseErr := strconv.ParseInt(header, 10, 64)
	if errors.Is(parseErr, strconv.ErrRange) {
		return 0, fmt.Errorf("retry wait too long")
	}
	if parseErr == nil && seconds >= 0 {
		if seconds > int64(maxRetryWait/time.Second) {
			return 0, fmt.Errorf("retry wait too long")
		}
		delay = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(header); err == nil {
		delay = date.Sub(now)
		if delay < 0 {
			delay = 0
		}
	}
	if delay > maxRetryWait {
		return 0, fmt.Errorf("retry wait too long")
	}
	return delay, nil
}
