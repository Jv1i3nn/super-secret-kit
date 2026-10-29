package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"ssk/internal/services"
)

func TestJiraCompactFlagsAndRelationships(t *testing.T) {
	var development atomic.Int32
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/dev-status/"):
			development.Add(1)
			fmt.Fprint(w, `{"detail":[]}`)
		case strings.HasSuffix(r.URL.Path, "/comment"):
			fmt.Fprint(w, `{"startAt":0,"total":2,"comments":[{"id":"older","created":"2020-01-01T00:00:00Z","updated":"2020-01-01T00:00:00Z","body":"Older body"},{"id":"newer","created":"2020-01-02T00:00:00Z","updated":"2020-01-02T00:00:00Z","body":"🦊éabc remainder"}]}`)
		default:
			fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{"summary":"Current","description":"🦊 First line\né Second line\nThird line omitted","parent":{"id":"2","key":"EX-2"},"issuelinks":[{"id":"link","outwardIssue":{"id":"3","key":"EX-3"}}]}}`)
		}
	})
	zeroInspectionTraversal(t)
	for _, format := range []string{"json", "markdown"} {
		command := historyTestCommand(t, format, "--description-lines", "2", "--comments-limit", "1", "--comment-chars", "5", "--include-development=false")
		var output strings.Builder
		command.SetOut(&output)
		if err := runJiraInspect(command, []string{"EX-1"}); err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(output.String()) || strings.Contains(output.String(), "Third line omitted") || strings.Contains(output.String(), "remainder") {
			t.Fatalf("invalid excerpt: %s", output.String())
		}
		if format == "json" {
			var result services.JiraExport
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatal(err)
			}
			issue := result.Issues[0]
			if !result.OutputTruncated || !issue.DescriptionTruncated || issue.Parent == nil || len(issue.IssueLinks) != 1 || result.Inspection.IncludeDevelopment {
				t.Fatal("lost relationships or compact metadata")
			}
			comments := issue.Comments
			if !comments.RetrievalComplete || *comments.MatchingCount != 2 || comments.ReturnedCount != 1 || !comments.SelectionTruncated || !comments.BodyOutputTruncated || !comments.Items[0].BodyTruncated || *comments.Items[0].Body != "🦊éabc" {
				t.Fatalf("bad compact comments %+v", comments)
			}
		} else if !strings.Contains(output.String(), "EX-2") || !strings.Contains(output.String(), "EX-3") || !strings.Contains(output.String(), "Description output truncated") || !strings.Contains(output.String(), "Body truncated: true") {
			t.Fatal("Markdown lost metadata")
		}
	}
	if development.Load() != 0 {
		t.Fatalf("development requests=%d", development.Load())
	}
	command := historyTestCommand(t, "json", "--include-development=false", "--description-lines", "0", "--comment-chars", "0")
	var output strings.Builder
	command.SetOut(&output)
	if err := runJiraInspect(command, []string{"EX-1"}); err != nil {
		t.Fatal(err)
	}
	var result services.JiraExport
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if result.OutputTruncated || result.Issues[0].DescriptionTruncated || result.Issues[0].Comments.BodyOutputTruncated || result.Issues[0].Comments.ReturnedCount != 2 || !strings.Contains(*result.Issues[0].Description, "Third line omitted") {
		t.Fatal("zero limits clipped output")
	}
}

func TestJiraCompactInvalidOptionsBeforeSetup(t *testing.T) {
	for _, flags := range [][]string{{"--description-lines", "-1"}, {"--comment-chars", "-1"}} {
		command := historyTestCommand(t, "json", flags...)
		if err := runJiraInspect(command, []string{"EX-1"}); err == nil || !strings.Contains(err.Error(), "cannot be negative") {
			t.Fatalf("error=%v", err)
		}
	}
	for _, duration := range []time.Duration{0, -time.Second} {
		command, _ := outputTestCommand("json")
		command.Flags().Duration("timeout", duration, "")
		if err := runJiraQuery(command, []string{"project = EX"}); err == nil || !strings.Contains(err.Error(), "--timeout") {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestJiraDeadlineLeavesNoOutput(t *testing.T) {
	for _, format := range []string{"json", "markdown"} {
		t.Run(format, func(t *testing.T) {
			setupJiraOutputTest(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
			zeroInspectionTraversal(t)
			command := historyTestCommand(t, format, "--include-development=false")
			command.Flags().Duration("timeout", 30*time.Millisecond, "")
			var output strings.Builder
			command.SetOut(&output)
			if err := runJiraInspect(command, []string{"EX-1"}); !errors.Is(err, context.DeadlineExceeded) || output.Len() != 0 {
				t.Fatalf("error=%v stdout=%q", err, output.String())
			}
		})
	}
}

func TestJiraQueryCancellationOnLaterPage(t *testing.T) {
	reached := make(chan struct{})
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("nextPageToken") == "" {
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"}],"nextPageToken":"next","isLast":false}`)
			return
		}
		close(reached)
		<-r.Context().Done()
	})
	command, output := outputTestCommand("json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- runJiraQuery(command, []string{"project = EX"}) }()
	<-reached
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || output.Len() != 0 {
		t.Fatalf("error=%v stdout=%q", err, output.String())
	}
}

func TestJiraReadRetriesDoNotPolluteJSON(t *testing.T) {
	calls := 0
	setupJiraOutputTest(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			fmt.Fprint(w, "private-description fixture-secret-token")
			return
		}
		fmt.Fprint(w, `{"issues":[],"isLast":true}`)
	})
	command, output := outputTestCommand("json")
	if err := runJiraQuery(command, []string{"project = EX"}); err != nil {
		t.Fatal(err)
	}
	var result services.JiraExport
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.IssueCount != 0 || strings.Contains(output.String(), "fixture-secret-token") {
		t.Fatal("unsafe or incomplete retried JSON")
	}
}

func TestJiraAttachmentCancellationFailsInspection(t *testing.T) {
	reached := make(chan struct{})
	sourceURL := ""
	server := setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file" {
			fmt.Fprint(w, "partial bytes")
			w.(http.Flusher).Flush()
			close(reached)
			<-r.Context().Done()
			return
		}
		fmt.Fprintf(w, `{"id":"1","key":"EX-1","fields":{"attachment":[{"id":"file","filename":"test.txt","content":%q}]}}`, sourceURL)
	})
	sourceURL = server.URL + "/file"
	zeroInspectionTraversal(t)
	jiraInspectDownloadDir = t.TempDir()
	command := historyTestCommand(t, "json", "--include-development=false")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command.SetContext(ctx)
	var output strings.Builder
	command.SetOut(&output)
	done := make(chan error, 1)
	go func() { done <- runJiraInspect(command, []string{"EX-1"}) }()
	<-reached
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || output.Len() != 0 {
		t.Fatalf("error=%v stdout=%q", err, output.String())
	}
}
