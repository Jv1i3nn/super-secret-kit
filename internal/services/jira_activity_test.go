package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ssk/internal/models"
	"ssk/internal/services/jira"
)

func activityPeriod(t *testing.T) *JiraPeriod {
	t.Helper()
	period, err := ParseJiraPeriod("2026-10-25T00:00:00Z", "2026-10-25T02:00:00Z", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return period
}

func TestActivityCandidateJQLAbsoluteBounds(t *testing.T) {
	first, err := ParseJiraPeriod("2026-10-25T02:30:00.123456789+02:00", "2026-10-25T03:00:00+01:00", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseJiraPeriod("2026-10-25T01:30:00.123456789+01:00", "2026-10-25T02:00:00Z", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	one, err := ActivityCandidateJQL(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := ActivityCandidateJQL(second)
	if err != nil {
		t.Fatal(err)
	}
	expected := fmt.Sprintf("updated >= %d ORDER BY updated DESC, key ASC", first.Since.UnixMilli())
	if one != two || one != expected || strings.ContainsAny(one, "\"<") {
		t.Fatalf("unsafe or timezone-dependent selection: %q %q", one, two)
	}
	if time.UnixMilli(first.Since.UnixMilli()).After(first.Since) {
		t.Fatal("candidate bound rounded up")
	}
}

type activityTestGate struct {
	done chan struct{}
	once sync.Once
}

type activityHTTPFixture struct {
	gate        atomic.Pointer[activityTestGate]
	active      atomic.Int32
	peak        atomic.Int32
	development atomic.Int32
}

func (f *activityHTTPFixture) serve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/search/jql" {
			if strings.Contains(r.URL.Query().Get("jql"), "<") || r.URL.Query().Get("fields") != "id,key,updated" {
				t.Errorf("unexpected candidate query: %s", r.URL)
			}
			if r.URL.Query().Get("nextPageToken") == "" {
				fmt.Fprint(w, `{"issues":[{"id":"2","key":"EX-2"},{"id":"1","key":"EX-1"}],"isLast":false,"nextPageToken":"next"}`)
			} else {
				fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"},{"id":"3","key":"OLD-3"}],"isLast":true}`)
			}
			return
		}
		active := f.active.Add(1)
		defer f.active.Add(-1)
		for old := f.peak.Load(); active > old; old = f.peak.Load() {
			if f.peak.CompareAndSwap(old, active) {
				break
			}
		}
		if strings.HasPrefix(r.URL.Path, "/rest/dev-status/") {
			f.development.Add(1)
			fmt.Fprint(w, `{"detail":[{"repositories":[{"name":"Example","url":"https://user:password@git.example/repo?token=fixture-secret#fragment","token":"fixture-secret","emailAddress":"private@example.com"}]}]}`)
			return
		}
		suffix := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
		parts := strings.Split(suffix, "/")
		id := parts[0]
		if len(parts) == 1 {
			if gate := f.gate.Load(); gate != nil {
				if active >= 2 {
					gate.once.Do(func() { close(gate.done) })
				}
				select {
				case <-gate.done:
				case <-r.Context().Done():
					return
				}
			}
			switch id {
			case "1":
				fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{"summary":"Reopened","description":"First 🦊\nSecond line","created":"2026-10-25T00:00:00Z","updated":"2026-10-29T10:00:00Z","status":{"id":"open","name":"Open","statusCategory":{"id":2,"key":"new","name":"To Do"}},"resolution":null,"parent":{"id":"9","key":"EX-9"},"subtasks":[],"issuelinks":[],"comment":{"comments":[{"id":"embedded","body":"Wrong subset"}]}}}`)
			case "2":
				fmt.Fprint(w, `{"id":"2","key":"EX-2","fields":{"summary":"Candidate only","created":"2020-01-01T00:00:00Z","updated":"2026-10-29T10:00:00Z","status":{"id":"done","name":"Cancelled","statusCategory":{"id":3,"key":"done","name":"Done"}},"resolution":{"id":"cancelled","name":"Cancelled"}}}`)
			case "3":
				fmt.Fprint(w, `{"id":"3","key":"EX-0","fields":{"summary":"Unknown dates"}}`)
			default:
				t.Errorf("unexpected immutable ID %q", id)
				w.WriteHeader(404)
			}
			return
		}
		switch parts[1] {
		case "changelog":
			if id != "1" && id != "EX-1" {
				fmt.Fprint(w, `{"startAt":0,"total":0,"values":[],"isLast":true}`)
				return
			}
			fmt.Fprint(w, `{"startAt":0,"total":5,"isLast":true,"values":[
    {"id":"1","created":"2026-10-25T00:10:00Z","items":[{"fieldId":"status","field":"status","from":"open","fromString":"Open","to":"done","toString":"Done"}]},
    {"id":"2","created":"2026-10-25T00:20:00Z","items":[{"fieldId":"status","field":"status","from":"done","to":"open"}]},
    {"id":"3","created":"2026-10-25T00:30:00Z","items":[{"fieldId":"status","field":"status","from":"open","to":"cancelled","toString":"Cancelled"},{"fieldId":"resolution","field":"resolution","from":null,"to":"cancelled"}]},
    {"id":"4","created":"2026-10-25T00:40:00Z","items":[{"fieldId":"status","field":"status","from":"cancelled","to":"open"},{"fieldId":"resolution","field":"resolution","from":"cancelled","to":null}]},
    {"id":"excluded","created":"2026-10-25T02:00:00Z","items":[{"fieldId":"status","field":"status","to":"done"}]}
   ]}`)
		case "comment":
			if id != "1" && id != "EX-1" {
				fmt.Fprint(w, `{"startAt":0,"total":0,"comments":[]}`)
				return
			}
			fmt.Fprint(w, `{"startAt":0,"total":4,"comments":[
    {"id":"old-edited","created":"2020-01-01T00:00:00Z","updated":"2026-10-25T01:00:00Z","body":"Old edited 🦊"},
    {"id":"both","created":"2026-10-25T00:15:00Z","updated":"2026-10-25T00:45:00Z","body":"New then edited"},
    {"id":"edited-later","created":"2026-10-25T00:16:00Z","updated":"2026-10-29T01:00:00Z","body":"Current version"},
    {"id":"at-until","created":"2026-10-25T02:00:00Z","updated":"2026-10-25T02:00:00Z"}
   ]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	}))
}

func TestActivityCountsBeforeLimitsAndDeterministicWorkers(t *testing.T) {
	fixture := &activityHTTPFixture{}
	server := fixture.serve(t)
	defer server.Close()
	client := jira.NewClient(server.URL, &models.Credentials{Username: "fixture@example.com", Password: "fixture-secret"})
	started := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	opts := JiraActivityOptions{Period: activityPeriod(t), Workers: 1, CommentsLimit: 1, CommentChars: 5, DescriptionLines: 1}
	first, err := NewJiraActivityService(client).Generate(context.Background(), "team", opts, started)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.peak.Load() != 1 {
		t.Fatalf("unbounded serial collection: %d", fixture.peak.Load())
	}
	fixture.peak.Store(0)
	fixture.gate.Store(&activityTestGate{done: make(chan struct{})})
	opts.Workers = 3
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	second, err := NewJiraActivityService(client).Generate(ctx, "team", opts, started)
	if err != nil {
		t.Fatal(err)
	}
	if peak := fixture.peak.Load(); peak < 2 || peak > 3 {
		t.Fatalf("unexpected concurrent requests: %d", peak)
	}
	second.CollectionEndedAt = first.CollectionEndedAt
	second.Activity.Workers = first.Activity.Workers
	one, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one, two) {
		t.Fatal("output depends on worker completion order")
	}
	if first.IssueCount != 3 || first.Issues[0].Key != "EX-1" || first.Issues[1].Key != "EX-2" || first.Issues[2].Key != "EX-0" {
		t.Fatal("wrong counts or issue ordering")
	}
	if first.Issues[0].UpdatedAt.Before(first.Activity.Period.Until) {
		t.Fatal("fixture must retain issue updated after until")
	}
	summary := first.Activity.Summary
	if summary.CreatedIssues.Count != 1 || summary.IssuesWithStatusChanges.Count != 1 || summary.StatusTransitionEvents.Count != 4 || summary.NewComments.Count != 2 || summary.EditedComments.Count != 2 || summary.UnknownCreationDates.Count != 1 || summary.IssuesWithoutDetailedEvidence.Count != 2 {
		t.Fatalf("bad activity counts %+v", summary)
	}
	if summary.StatusTransitionEvents.Unit != "events" || summary.CreatedIssues.Unit != "issues" || summary.NewComments.Unit != "comments" || len(summary.CurrentStatuses) != 3 {
		t.Fatal("units/current states lost")
	}
	issue := first.Issues[0]
	if !first.OutputTruncated || !first.RetrievalComplete || !issue.DescriptionTruncated || issue.Parent == nil || issue.Comments.ReturnedCount != 1 || *issue.Comments.MatchingCount != 3 || len(issue.Activity.Events) != 11 {
		t.Fatal("limits erased source evidence or metadata")
	}
	if issue.Activity.Events[0].Kind != "issue.created" || !issue.Activity.Events[0].At.Equal(opts.Period.Since) {
		t.Fatal("inclusive boundary lost")
	}
	for index, event := range issue.Activity.Events {
		if !event.At.Before(opts.Period.Until) {
			t.Fatal("exclusive boundary included")
		}
		if index > 0 {
			prev := issue.Activity.Events[index-1]
			if event.At.Before(prev.At) || (event.At.Equal(prev.At) && event.SourceID < prev.SourceID) {
				t.Fatal("unstable event timeline")
			}
		}
	}
	if first.Issues[1].Activity.HasDetailedEvidence || len(first.Issues[1].Activity.Events) != 0 || first.Issues[2].Activity.CreatedInPeriod != nil {
		t.Fatal("invented evidence or creation date")
	}
	if fixture.development.Load() != 0 {
		t.Fatal("development request despite disabled flag")
	}
	markdown, err := RenderJiraActivityMarkdown(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"# Jira Activity", "Status transitions: 4 events", "New comments: 2 comments", "EX-9", "Detailed period activity evidence: false", "Counts use all matching sources"} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("missing Markdown %q", expected)
		}
	}
	for _, forbidden := range []string{"fixture-secret", "fixture@example.com", "Wrong subset"} {
		if strings.Contains(string(one), forbidden) || strings.Contains(markdown, forbidden) {
			t.Fatalf("leaked %q", forbidden)
		}
	}
}

func TestActivityDevelopmentIsOptionalAndSanitized(t *testing.T) {
	fixture := &activityHTTPFixture{}
	server := fixture.serve(t)
	defer server.Close()
	result, err := NewJiraActivityService(jira.NewClient(server.URL, &models.Credentials{})).Generate(context.Background(), "team", JiraActivityOptions{Period: activityPeriod(t), Workers: 2, IncludeDevelopment: true}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	development := result.Issues[0].Development
	if development == nil || development.RetrievalComplete || len(development.Repositories) != 1 || !strings.Contains(development.Repositories[0], "https://git.example/repo") {
		t.Fatal("missing best-effort links")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"fixture-secret", "password", "private@example.com", "#fragment"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("development leaked %q", private)
		}
	}
}

func TestActivityEmptyAndValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected request %s", r.URL)
		}
		fmt.Fprint(w, `{"issues":[],"isLast":true}`)
	}))
	defer server.Close()
	service := NewJiraActivityService(jira.NewClient(server.URL, &models.Credentials{}))
	opts := JiraActivityOptions{Period: activityPeriod(t), Workers: 6}
	result, err := service.Generate(context.Background(), "team", opts, time.Now())
	if err != nil || result.Issues == nil || result.Warnings == nil || result.IssueCount != 0 || result.Activity.Summary.CurrentStatuses == nil {
		t.Fatalf("bad empty export %+v %v", result, err)
	}
	for _, workers := range []int{0, 33} {
		opts.Workers = workers
		if result, err := service.Generate(context.Background(), "team", opts, time.Now()); err == nil || result != nil {
			t.Fatal("accepted invalid workers")
		}
	}
}

func TestActivityFailureCancelsOtherWorkers(t *testing.T) {
	blocked := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/search/jql":
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"},{"id":"2","key":"EX-2"}],"isLast":true}`)
		case "/rest/api/3/issue/1":
			select {
			case <-blocked:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(403)
		case "/rest/api/3/issue/2":
			close(blocked)
			<-r.Context().Done()
			close(canceled)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := NewJiraActivityService(jira.NewClient(server.URL, &models.Credentials{})).Generate(ctx, "team", JiraActivityOptions{Period: activityPeriod(t), Workers: 2}, time.Now())
	if err == nil || result != nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("outstanding request not canceled")
	}
}
