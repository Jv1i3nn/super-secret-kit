package jira

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ssk/internal/models"
)

func TestReadRetriesTransientStatus(t *testing.T) {
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(status)
					fmt.Fprint(w, "private-description fixture-token")
					return
				}
				fmt.Fprint(w, `{"accountId":"person"}`)
			}))
			defer server.Close()
			user, err := NewClient(server.URL, &models.Credentials{}).GetCurrentUser()
			if err != nil || calls != 2 || user.AccountID != "person" {
				t.Fatalf("calls=%d user=%+v error=%v", calls, user, err)
			}
		})
	}
}

func TestReadRetryExhaustionAndNonRetryableStatus(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 501} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(status)
				fmt.Fprint(w, "private-description fixture-token")
			}))
			defer server.Close()
			_, err := NewClient(server.URL, &models.Credentials{}).GetCurrentUser()
			want := 1
			if status == 429 || status == 500 {
				want = 3
			}
			if err == nil || calls != want || strings.Contains(err.Error(), "private-description") || strings.Contains(err.Error(), "fixture-token") {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestCommentCreationNeverRetries(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" {
					t.Error("unexpected method")
				}
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(status)
			}))
			defer server.Close()
			_, err := NewClient(server.URL, &models.Credentials{}).AddComment("EX-1", "hello")
			if err == nil || calls != 1 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type failingReadBody struct{}

func (failingReadBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingReadBody) Close() error             { return nil }

func TestReadRetriesTransportAndBodyFailures(t *testing.T) {
	for _, bodyFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("body=%t", bodyFailure), func(t *testing.T) {
			client := NewClient("https://jira.example", &models.Credentials{})
			calls := 0
			client.httpClient.Transport = testRoundTripper(func(_ *http.Request) (*http.Response, error) {
				calls++
				if calls < 3 {
					if bodyFailure {
						return &http.Response{StatusCode: 200, Body: failingReadBody{}, Header: make(http.Header)}, nil
					}
					return nil, io.EOF
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"accountId":"person"}`)), Header: make(http.Header)}, nil
			})
			_, err := client.GetCurrentUser()
			if err != nil || calls != 3 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestRequestErrorsHideURLsAndDoNotRetryPermanentFailures(t *testing.T) {
	client := NewClient("https://jira.example", &models.Credentials{})
	calls := 0
	client.httpClient.Transport = testRoundTripper(func(_ *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("private-description fixture-token https://user:password@jira.example/?token=secret")
	})
	_, err := client.GetCurrentUser()
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
	for _, private := range []string{"private-description", "fixture-token", "password", "token=secret"} {
		if strings.Contains(err.Error(), private) {
			t.Fatalf("unsafe error %v", err)
		}
	}
	calls = 0
	client.httpClient.Transport = testRoundTripper(func(_ *http.Request) (*http.Response, error) { calls++; return nil, io.EOF })
	if _, err := client.AddComment("EX-1", "hello"); err == nil || calls != 1 {
		t.Fatalf("mutation transport retried: %d %v", calls, err)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header string
		want   time.Duration
		reject bool
	}{
		{"0", 0, false}, {"2", 2 * time.Second, false}, {"", 200 * time.Millisecond, false}, {"invalid", 200 * time.Millisecond, false},
		{now.Add(4 * time.Second).Format(http.TimeFormat), 4 * time.Second, false}, {now.Add(-time.Second).Format(http.TimeFormat), 0, false},
		{"31", 0, true}, {"999999999999999999999999999999", 0, true}, {now.Add(time.Minute).Format(http.TimeFormat), 0, true},
	} {
		delay, err := readRetryDelay(tc.header, 0, now)
		if (err != nil) != tc.reject || (!tc.reject && delay != tc.want) {
			t.Fatalf("header=%q delay=%v error=%v", tc.header, delay, err)
		}
	}
}

func TestRetryAfterRespectsBudgetAndCancellation(t *testing.T) {
	for _, header := range []string{"2", "31"} {
		t.Run(header, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Retry-After", header)
				w.WriteHeader(429)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := NewClient(server.URL, &models.Credentials{}).WithContext(ctx).GetCurrentUser()
			if err == nil || calls != 1 || !strings.Contains(err.Error(), "budget") {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		close(received)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewClient(server.URL, &models.Credentials{}).WithContext(ctx).GetCurrentUser()
		done <- err
	}()
	<-received
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestRequestTimeoutAndStreamingCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client := NewClient(server.URL, &models.Credentials{})
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatal("missing request timeout")
	}
	client.httpClient.Timeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := client.WithContext(ctx).GetCurrentUser()
	if err == nil {
		t.Fatal("slow response succeeded")
	}
}

func TestDevelopmentCanBeSkippedAndCancellationStopsProbes(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/dev-status/") {
			probes.Add(1)
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{}}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, &models.Credentials{})
	if _, err := client.GetInspectionTicketByKey("EX-1", false); err != nil || probes.Load() != 0 {
		t.Fatalf("unexpected development request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := client.WithContext(ctx).GetBriefingTicketByKey("EX-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("optional probes swallowed cancellation: %v", err)
	}
	if probes.Load() != 1 {
		t.Fatalf("probes=%d", probes.Load())
	}
}

func TestAttachmentStreamCancellationRemovesPartialFile(t *testing.T) {
	reached := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "partial bytes")
		w.(http.Flusher).Flush()
		close(reached)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	destination := filepath.Join(t.TempDir(), "attachment")
	done := make(chan error, 1)
	go func() {
		done <- NewClient(server.URL, &models.Credentials{}).WithContext(ctx).DownloadAttachment(server.URL+"/file", destination)
	}()
	<-reached
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled stream succeeded")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial file remains: %v", err)
	}
}

func TestAttachmentRetriesAcrossHeadersAndStreamWithoutAppending(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		case 2:
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "partial")
		default:
			fmt.Fprint(w, "complete")
		}
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "artifact")
	if err := NewClient(server.URL, &models.Credentials{}).DownloadAttachment(server.URL+"/file", destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "complete" || calls != 3 {
		t.Fatalf("data=%q calls=%d error=%v", data, calls, err)
	}
}

func TestAttachmentRetriesRemainBoundedAndCleanUp(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "partial")
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "artifact")
	if err := NewClient(server.URL, &models.Credentials{}).DownloadAttachment(server.URL+"/file", destination); err == nil || calls != 3 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("failed stream left a partial file")
	}
}

func TestAttachmentURLDiagnosticsHideSecrets(t *testing.T) {
	for _, tc := range []struct{ base, source string }{
		{"https://jira.example", "https://jira.example/file?token=fixture-secret-token\n"},
		{"https://jira.example/?token=fixture-secret-token\n", "https://jira.example/file"},
	} {
		err := NewClient(tc.base, &models.Credentials{}).DownloadAttachment(tc.source, filepath.Join(t.TempDir(), "artifact"))
		if err == nil || strings.Contains(err.Error(), "fixture-secret-token") {
			t.Fatalf("unsafe URL diagnostic: %v", err)
		}
	}
}
