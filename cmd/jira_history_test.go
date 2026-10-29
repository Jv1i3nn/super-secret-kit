package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"ssk/internal/services"
)

func historyTestCommand(t *testing.T, format string, flags ...string) *cobra.Command {
	t.Helper()
	command, _ := outputTestCommand(format)
	addJiraHistoryFlags(command)
	if err := command.Flags().Parse(flags); err != nil {
		t.Fatal(err)
	}
	return command
}

func zeroInspectionTraversal(t *testing.T) {
	t.Helper()
	depth, maxRelated, directory := jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir
	jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir = 0, 0, ""
	t.Cleanup(func() { jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir = depth, maxRelated, directory })
}

func TestJiraHistoryFlagValidation(t *testing.T) {
	for _, flags := range [][]string{
		{"--changes-until", "2026-10-28T10:00:00Z"},
		{"--comments-until", "2026-10-28T10:00:00Z"},
		{"--changes-since", "2026-10-28T10:00:00"},
		{"--comments-since", "2026-10-28"},
		{"--comments-since", "2026-10-28T10:00:00Z", "--comments-until", "2026-10-28T10:00:00Z"},
		{"--changes-since", "2026-10-28T10:00:00Z", "--changes-until", "2026-10-27T10:00:00Z"},
		{"--comments-limit", "-1"},
		{"--comments-since="},
		{"--changes-since", "2026-10-28T10:00:00Z", "--changes-until="},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			command := historyTestCommand(t, "json", flags...)
			// No store/configuration setup: malformed options must fail before access.
			err := runJiraInspect(command, []string{"EX-1"})
			if err == nil || !strings.Contains(err.Error(), "--") {
				t.Fatalf("expected flag error before setup: %v", err)
			}
		})
	}
	started := time.Date(2026, 10, 29, 10, 0, 0, 123, time.UTC)
	command := historyTestCommand(t, "json", "--changes-since", "2026-10-28T10:00:00Z", "--comments-since", "2026-10-28T10:00:00Z", "--comments-limit", "0")
	opts, err := jiraHistoryOptions(command, started)
	if err != nil || !opts.ChangesPeriod.Until.Equal(started) || !opts.CommentsPeriod.Until.Equal(started) || opts.CommentsLimit == nil || *opts.CommentsLimit != 0 {
		t.Fatalf("wrong captured bounds/options: %+v %v", opts, err)
	}
}

func TestJiraInspectPaginatedHistory(t *testing.T) {
	changesCalls, commentsCalls := 0, 0
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/dev-status/"):
			fmt.Fprint(w, `{"detail":[]}`)
		case strings.HasSuffix(r.URL.Path, "/changelog"):
			changesCalls++
			if r.URL.Query().Get("startAt") == "0" {
				fmt.Fprint(w, `{"startAt":0,"total":2,"isLast":false,"values":[{"id":"done","created":"2026-10-25T00:00:00Z","items":[{"fieldId":"status","field":"status","fromString":"Open","toString":"Done"}]}]}`)
			} else {
				fmt.Fprint(w, `{"startAt":1,"total":2,"isLast":true,"values":[{"id":"reopen","created":"2026-10-25T01:00:00Z","items":[{"fieldId":"status","field":"status","fromString":"Done","toString":"Open"}]}]}`)
			}
		case strings.HasSuffix(r.URL.Path, "/comment"):
			commentsCalls++
			if r.URL.Query().Get("startAt") == "0" {
				fmt.Fprint(w, `{"startAt":0,"total":2,"comments":[{"id":"new","created":"2026-10-25T00:00:00Z","updated":"2026-10-25T00:00:00Z","body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"New discussion"}]}]}}]}`)
			} else {
				fmt.Fprint(w, `{"startAt":1,"total":2,"comments":[{"id":"old-edited","created":"2020-01-01T00:00:00Z","updated":"2026-10-25T01:00:00Z","author":{"accountId":"person","displayName":"John Doe","emailAddress":"private@example.com"},"body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Old comment edited 🦊"}]}]}}]}`)
			}
		case r.URL.Path == "/rest/api/3/issue/EX-1":
			fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{"summary":"Current ticket","updated":"2026-10-29T10:00:00Z","status":{"name":"Open"},"comment":{"comments":[{"id":"embedded","body":"Incomplete subset"}]}}}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	})
	zeroInspectionTraversal(t)
	flags := []string{"--changes-since", "2026-10-25T00:00:00Z", "--changes-until", "2026-10-25T02:00:00Z", "--comments-since", "2026-10-25T00:00:00Z", "--comments-until", "2026-10-25T02:00:00Z", "--comments-limit", "1"}
	for _, format := range []string{"json", "markdown"} {
		command := historyTestCommand(t, format, flags...)
		var output strings.Builder
		command.SetOut(&output)
		if err := runJiraInspect(command, []string{"EX-1"}); err != nil {
			t.Fatal(err)
		}
		if format == "json" {
			var result services.JiraExport
			if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
				t.Fatal(err)
			}
			issue := result.Issues[0]
			if issue.Changes.ReturnedCount != 2 || issue.Comments.ReturnedCount != 1 || *issue.Comments.MatchingCount != 2 || !issue.Comments.RetrievalComplete || !result.OutputTruncated || *issue.Comments.Items[0].ID != "old-edited" || !*issue.Comments.Items[0].EditedInPeriod {
				t.Fatalf("bad history export %+v", issue)
			}
			if len(result.Warnings) != 1 || result.Warnings[0].Code != "comments_output_limited" {
				t.Fatal("stale embedded comments warning")
			}
		} else if !strings.Contains(output.String(), "Old comment edited 🦊") || !strings.Contains(output.String(), "reopen") || strings.Contains(output.String(), "Incomplete subset") {
			t.Fatalf("bad filtered Markdown: %s", output.String())
		}
		for _, forbidden := range []string{"private@example.com", "fixture-secret-token", "fixture@example.com"} {
			if strings.Contains(output.String(), forbidden) {
				t.Fatalf("leaked %q", forbidden)
			}
		}
	}
	if changesCalls != 4 || commentsCalls != 4 {
		t.Fatalf("missing pages: changes=%d comments=%d", changesCalls, commentsCalls)
	}
}

func TestJiraHistoryLaterFailureHasNoOutput(t *testing.T) {
	for _, source := range []string{"comment", "changelog"} {
		t.Run(source, func(t *testing.T) {
			setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/rest/dev-status/"):
					fmt.Fprint(w, `{"detail":[]}`)
				case strings.HasSuffix(r.URL.Path, "/"+source):
					if r.URL.Query().Get("startAt") == "0" {
						field := "values"
						if source == "comment" {
							field = "comments"
						}
						fmt.Fprintf(w, `{"startAt":0,"total":2,%q:[{"id":"1","created":"2026-10-25T00:00:00Z","updated":"2026-10-25T00:00:00Z","items":[]}]}`, field)
					} else {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, "fixture-secret-token")
					}
				default:
					fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{}}`)
				}
			})
			zeroInspectionTraversal(t)
			prefix := "changes"
			if source == "comment" {
				prefix = "comments"
			}
			for _, format := range []string{"json", "markdown"} {
				command := historyTestCommand(t, format, "--"+prefix+"-since", "2026-10-25T00:00:00Z", "--"+prefix+"-until", "2026-10-26T00:00:00Z")
				var output strings.Builder
				command.SetOut(&output)
				err := runJiraInspect(command, []string{"EX-1"})
				if err == nil || output.Len() != 0 || strings.Contains(err.Error(), "fixture-secret-token") {
					t.Fatalf("error=%v stdout=%s", err, output.String())
				}
			}
		})
	}
}

func TestJiraCommentsExplicitZeroAndDefaultUpperBound(t *testing.T) {
	commentsCalls := 0
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/dev-status/"):
			fmt.Fprint(w, `{"detail":[]}`)
		case strings.HasSuffix(r.URL.Path, "/comment"):
			commentsCalls++
			fmt.Fprint(w, `{"startAt":0,"total":1,"comments":[{"id":"1","created":"2020-01-02T00:00:00Z","updated":"2020-01-02T00:00:00Z"}]}`)
		case strings.HasSuffix(r.URL.Path, "/changelog"):
			t.Error("unexpected changelog retrieval")
			w.WriteHeader(500)
		default:
			fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{}}`)
		}
	})
	zeroInspectionTraversal(t)
	for _, flags := range [][]string{{"--comments-limit", "0"}, {"--comments-since", "2020-01-01T00:00:00Z"}} {
		command := historyTestCommand(t, "json", flags...)
		var output strings.Builder
		command.SetOut(&output)
		if err := runJiraInspect(command, []string{"EX-1"}); err != nil {
			t.Fatal(err)
		}
		var result services.JiraExport
		if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
			t.Fatal(err)
		}
		comments := result.Issues[0].Comments
		if comments.ReturnedCount != 1 || !comments.RetrievalComplete || comments.OutputTruncated || result.Issues[0].Changes != nil {
			t.Fatal("explicit comment retrieval incomplete")
		}
		if flags[0] == "--comments-since" && !comments.Period.Until.Equal(result.CollectionStartedAt) {
			t.Fatal("upper bound differs from command start")
		}
		if flags[0] == "--comments-limit" && (comments.Period != nil || comments.Limit == nil || *comments.Limit != 0) {
			t.Fatal("zero limit invented a period or lost explicit unlimited selection")
		}
	}
	if commentsCalls != 2 {
		t.Fatal("missing explicit retrieval")
	}
}
