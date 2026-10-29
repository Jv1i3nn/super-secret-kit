package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/services"
)

func activityTestCommand(t *testing.T, flags ...string) *cobra.Command {
	t.Helper()
	command := newJiraActivityCommand()
	if err := command.Flags().Parse(flags); err != nil {
		t.Fatal(err)
	}
	return command
}

func TestJiraActivityValidationBeforeSetup(t *testing.T) {
	for _, flags := range [][]string{
		{}, {"--since="}, {"--since", "2026-10-25"}, {"--since", "2026-10-25T00:00:00"},
		{"--since", "2026-10-25T00:00:00Z", "--until="},
		{"--since", "2026-10-25T00:00:00Z", "--until", "2026-10-25T00:00:00Z"},
		{"--since", "2026-10-25T00:00:00Z", "--until", "2026-10-24T00:00:00Z"},
		{"--since", "2020-01-01T00:00:00Z", "--workers", "0"}, {"--since", "2020-01-01T00:00:00Z", "--workers", "33"},
		{"--since", "2020-01-01T00:00:00Z", "--comments-limit", "-1"},
		{"--since", "2020-01-01T00:00:00Z", "--comment-chars", "-1"},
		{"--since", "2020-01-01T00:00:00Z", "--description-lines", "-1"},
		{"--since", "2020-01-01T00:00:00Z", "--format", "yaml"},
	} {
		command := activityTestCommand(t, flags...)
		var output strings.Builder
		command.SetOut(&output)
		if err := runJiraActivity(command, nil); err == nil || !strings.Contains(err.Error(), "--") || output.Len() != 0 {
			t.Fatalf("flags=%v error=%v stdout=%q", flags, err, output.String())
		}
	}
	for _, flag := range []string{"--changes-since", "--changes-until", "--comments-since", "--comments-until"} {
		if err := newJiraActivityCommand().Flags().Parse([]string{flag, "2026-10-25T00:00:00Z"}); err == nil {
			t.Fatalf("accepted conflicting per-source period %s", flag)
		}
	}
	started := time.Date(2026, 10, 29, 10, 0, 0, 123, time.UTC)
	command := activityTestCommand(t, "--since", "2026-10-25T00:00:00Z")
	opts, err := jiraActivityOptions(command, started)
	if err != nil || opts.Workers != 6 || !opts.IncludeDevelopment || !opts.Period.Until.Equal(started) {
		t.Fatalf("bad defaults %+v %v", opts, err)
	}
}

func TestJiraActivityJSONAndDefaultMarkdown(t *testing.T) {
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/search/jql":
			if strings.Contains(r.URL.Query().Get("jql"), "<") {
				t.Error("upper update bound loses historical events")
			}
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"},{"id":"1","key":"EX-1"}],"isLast":true}`)
		case "/rest/api/3/issue/1":
			fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{"summary":"Current ticket","description":"First 🦊\nSecond line","created":"2020-01-01T00:00:00Z","updated":"2026-10-29T10:00:00Z","status":{"id":"open","name":"Open","statusCategory":{"key":"new","name":"To Do"}},"parent":{"id":"9","key":"EX-9"},"assignee":{"displayName":"John Doe","emailAddress":"private@example.com"}}}`)
		case "/rest/api/3/issue/EX-1/changelog":
			if r.URL.Query().Get("startAt") == "0" {
				fmt.Fprint(w, `{"startAt":0,"total":2,"isLast":false,"values":[{"id":"c1","created":"2026-10-25T10:00:00Z","items":[{"fieldId":"status","from":"open","to":"done"}]}]}`)
			} else {
				fmt.Fprint(w, `{"startAt":1,"total":2,"isLast":true,"values":[{"id":"c2","created":"2026-10-25T11:00:00Z","items":[{"fieldId":"status","from":"done","to":"open"}]}]}`)
			}
		case "/rest/api/3/issue/EX-1/comment":
			if r.URL.Query().Get("startAt") == "0" {
				fmt.Fprint(w, `{"startAt":0,"total":2,"comments":[{"id":"new","created":"2026-10-25T10:00:00Z","updated":"2026-10-25T10:00:00Z","body":"New discussion"}]}`)
			} else {
				fmt.Fprint(w, `{"startAt":1,"total":2,"comments":[{"id":"old","created":"2020-01-01T00:00:00Z","updated":"2026-10-25T12:00:00Z","body":"🦊éabc older comment edited","author":{"displayName":"John Doe","emailAddress":"private@example.com"}}]}`)
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	})
	base := []string{"--since", "2026-10-25T00:00:00Z", "--until", "2026-10-26T00:00:00Z", "--workers", "2", "--include-development=false", "--comments-limit", "1", "--comment-chars", "5", "--description-lines", "1"}
	for _, format := range []string{"json", "markdown"} {
		flags := append([]string{}, base...)
		if format == "json" {
			flags = append(flags, "--format", "json")
		}
		command := activityTestCommand(t, flags...)
		var output strings.Builder
		command.SetOut(&output)
		if err := runJiraActivity(command, nil); err != nil {
			t.Fatal(err)
		}
		if format == "json" {
			decoder := json.NewDecoder(strings.NewReader(output.String()))
			var result services.JiraExport
			if err := decoder.Decode(&result); err != nil {
				t.Fatal(err)
			}
			var extra interface{}
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatalf("stdout contains extra content: %v", err)
			}
			summary := result.Activity.Summary
			if result.Kind != "jira.activity" || result.SchemaVersion != 1 || result.IssueCount != 1 || result.Inspection != nil || summary.StatusTransitionEvents.Count != 2 || summary.IssuesWithStatusChanges.Count != 1 || summary.NewComments.Count != 1 || summary.EditedComments.Count != 1 {
				t.Fatalf("bad export %+v", result)
			}
			issue := result.Issues[0]
			if !result.OutputTruncated || !issue.DescriptionTruncated || issue.Parent == nil || *issue.Comments.MatchingCount != 2 || issue.Comments.ReturnedCount != 1 || *issue.Comments.Items[0].Body != "🦊éabc" {
				t.Fatal("limits lost metadata or relationships")
			}
		} else {
			for _, expected := range []string{"# Jira Activity", "Status transitions: 2 events", "With status changes: 1 issues", "New comments: 1 comments", "EX-9", "Detailed period activity evidence: true"} {
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("missing Markdown %q", expected)
				}
			}
		}
		for _, private := range []string{"private@example.com", "fixture-secret-token", "fixture@example.com", "Second line", "older comment edited"} {
			if strings.Contains(output.String(), private) {
				t.Fatalf("leaked %q", private)
			}
		}
	}
}

func TestJiraActivityEmptyDefaultUntilAndWorkspace(t *testing.T) {
	server := setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected request %s", r.URL)
		}
		fmt.Fprint(w, `{"issues":[],"isLast":true}`)
	})
	viper.Set("jira_workspaces", map[string]string{"other": server.URL})
	parent := &cobra.Command{Use: "jira"}
	parent.PersistentFlags().StringVar(&jiraWorkspace, "workspace", "default", "")
	parent.PersistentFlags().Duration("timeout", time.Minute, "")
	parent.AddCommand(newJiraActivityCommand())
	parent.SetArgs([]string{"activity", "--since", "2020-01-01T00:00:00Z", "--format", "json", "--workspace", "other", "--timeout", "1s"})
	var output strings.Builder
	parent.SetOut(&output)
	if err := parent.Execute(); err != nil {
		t.Fatal(err)
	}
	var result services.JiraExport
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if result.Workspace != "other" || result.SiteURL != server.URL || result.Issues == nil || result.IssueCount != 0 || !result.Activity.Period.Until.Equal(result.CollectionStartedAt) {
		t.Fatalf("bad empty/default export %+v", result)
	}
}

func TestJiraActivityFailureHasNoPartialOutput(t *testing.T) {
	for _, status := range []int{403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/api/3/search/jql":
					fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"}],"isLast":true}`)
				case "/rest/api/3/issue/1":
					fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{}}`)
				case "/rest/api/3/issue/EX-1/changelog":
					if r.URL.Query().Get("startAt") == "0" {
						fmt.Fprint(w, `{"startAt":0,"total":2,"values":[{"id":"c1","created":"2026-10-25T00:00:00Z","items":[]}]}`)
					} else {
						w.Header().Set("Retry-After", "0")
						w.WriteHeader(status)
						fmt.Fprint(w, "fixture-secret-token")
					}
				default:
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(500)
				}
			})
			for _, format := range []string{"json", "markdown"} {
				command := activityTestCommand(t, "--since", "2026-10-25T00:00:00Z", "--until", "2026-10-26T00:00:00Z", "--format", format, "--include-development=false")
				var output strings.Builder
				command.SetOut(&output)
				err := runJiraActivity(command, nil)
				if err == nil || output.Len() != 0 || strings.Contains(err.Error(), "fixture-secret-token") {
					t.Fatalf("error=%v stdout=%q", err, output.String())
				}
			}
		})
	}
}

func TestJiraActivityDeadlineCancelsWorkers(t *testing.T) {
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/search/jql" {
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"},{"id":"2","key":"EX-2"}],"isLast":true}`)
			return
		}
		<-r.Context().Done()
	})
	command := newJiraActivityCommand()
	command.Flags().Duration("timeout", 30*time.Millisecond, "")
	if err := command.Flags().Parse([]string{"--since", "2020-01-01T00:00:00Z", "--format", "json", "--include-development=false", "--workers", "2"}); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	command.SetOut(&output)
	if err := runJiraActivity(command, nil); !errors.Is(err, context.DeadlineExceeded) || output.Len() != 0 {
		t.Fatalf("error=%v stdout=%q", err, output.String())
	}
}
