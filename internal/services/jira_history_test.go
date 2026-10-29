package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ssk/internal/services/jira"
)

func testHistoryPeriod(t *testing.T) *JiraPeriod {
	t.Helper()
	period, err := ParseJiraPeriod("2026-10-25T02:00:00+02:00", "2026-10-25T03:00:00+01:00", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return period
}

func TestParseJiraPeriod(t *testing.T) {
	started := time.Date(2026, 10, 29, 10, 0, 0, 123, time.UTC)
	period, err := ParseJiraPeriod("2026-10-28T10:00:00Z", "", started)
	if err != nil || !period.Until.Equal(started) {
		t.Fatalf("period=%+v error=%v", period, err)
	}
	for _, tc := range []struct{ since, until string }{
		{"", "2026-10-29T10:00:00Z"}, {"2026-10-28T10:00:00", ""}, {"2026-10-28", ""}, {"invalid", ""},
		{"2026-10-28T10:00:00Z", "2026-10-29T10:00:00"}, {"2026-10-28T10:00:00Z", "2026-10-28T10:00:00Z"},
		{"2026-10-28T10:00:00Z", "2026-10-27T10:00:00Z"},
	} {
		if _, err := ParseJiraPeriod(tc.since, tc.until, started); err == nil {
			t.Fatalf("accepted %q %q", tc.since, tc.until)
		}
	}
	period = testHistoryPeriod(t)
	if period.Since.Format(time.RFC3339) != "2026-10-25T00:00:00Z" || period.Until.Format(time.RFC3339) != "2026-10-25T02:00:00Z" {
		t.Fatal("incorrect DST normalization")
	}
}

func TestChangelogsPreserveIntermediateTransitionsAndBounds(t *testing.T) {
	var entries []jira.Changelog
	raw := `[
 {"id":"at-until","created":"2026-10-25T02:00:00Z","items":[{"field":"status","toString":"Excluded"}]},
 {"id":"reopen","created":"2026-10-25T02:30:00+0100","author":{"displayName":"John Doe","emailAddress":"private@example.com"},"items":[{"fieldId":"status","field":"status","from":"done","fromString":"Done","to":"open","toString":"Open"},{"fieldId":"resolution","field":"resolution","from":"fixed","to":null}]},
 {"id":"done","created":"2026-10-25T02:30:00+0200","items":[{"fieldId":"status","field":"status","from":"open","fromString":"Open","to":"done","toString":"Done"}]},
 {"id":"at-since","created":"2026-10-25T00:00:00Z","items":[{"fieldId":"assignee","field":"assignee","from":null,"to":"person"}]},
 {"id":"before","created":"2026-10-24T23:59:59Z","items":[]}
 ]`
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatal(err)
	}
	result, err := normalizeChangelogs(entries, testHistoryPeriod(t))
	if err != nil {
		t.Fatal(err)
	}
	if !result.RetrievalComplete || result.RetrievedChangelogCount != 5 || result.ReturnedCount != 4 || len(result.Items) != 4 {
		t.Fatalf("bad metadata %+v", result)
	}
	if result.Items[0].ChangelogID != "at-since" || result.Items[1].ChangelogID != "done" || result.Items[2].ChangelogID != "reopen" || result.Items[3].ItemIndex != 1 {
		t.Fatal("transitions or ordering lost")
	}
	if *result.Items[2].FromText != "Done" || *result.Items[2].ToText != "Open" || result.Items[3].ToID != nil {
		t.Fatal("lost transition values/null")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private@example.com") {
		t.Fatal("email leak")
	}
}

func TestCommentsFilterBeforeLimitWithOldEdits(t *testing.T) {
	var comments []jira.Comment
	raw := `[
 {"id":"excluded","created":"2026-10-25T02:00:00Z","updated":"2026-10-25T02:00:00Z"},
 {"id":"old-edited","created":"2020-01-01T00:00:00Z","updated":"2026-10-25T02:30:00+0100","body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Edited café 🦊"}]}]}},
 {"id":"created-then-edited-later","created":"2026-10-25T00:30:00Z","updated":"2026-10-29T10:00:00Z"},
 {"id":"at-since","created":"2026-10-25T00:00:00Z","updated":"2026-10-25T00:00:00Z"},
 {"id":"outside","created":"2020-01-01T00:00:00Z","updated":"2026-10-29T10:00:00Z"}
 ]`
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		t.Fatal(err)
	}
	limit := 1
	result, err := normalizeComments(comments, testHistoryPeriod(t), &limit)
	if err != nil {
		t.Fatal(err)
	}
	if *result.RetrievedCount != 5 || *result.MatchingCount != 3 || result.ReturnedCount != 1 || !result.OutputTruncated || !result.RetrievalComplete {
		t.Fatalf("bad metadata %+v", result)
	}
	item := result.Items[0]
	if *item.ID != "old-edited" || *item.CreatedInPeriod || !*item.EditedInPeriod || *item.Body != "Edited café 🦊" {
		t.Fatalf("bad edited comment %+v", item)
	}
	limit = 0
	result, err = normalizeComments(comments, testHistoryPeriod(t), &limit)
	if err != nil || result.ReturnedCount != 3 || result.OutputTruncated {
		t.Fatalf("zero limit: %+v, %v", result, err)
	}
	if result.Items[1].ActivityAt.Format(time.RFC3339) != "2026-10-25T00:30:00Z" || !*result.Items[1].CreatedInPeriod || *result.Items[1].EditedInPeriod {
		t.Fatal("later edit erased qualifying creation")
	}
}

func TestCommentsBothActivityKindsAndStableTies(t *testing.T) {
	comments := []jira.Comment{
		{ID: "b", Created: "2026-10-25T00:10:00Z", Updated: "2026-10-25T01:00:00Z"},
		{ID: "a", Created: "2026-10-25T00:10:00Z", Updated: "2026-10-25T01:00:00Z"},
	}
	result, err := normalizeComments(comments, testHistoryPeriod(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if *result.Items[0].ID != "a" || !*result.Items[0].CreatedInPeriod || !*result.Items[0].EditedInPeriod {
		t.Fatal("unstable ties or lost edit/creation flags")
	}
	all, err := normalizeComments(comments, nil, nil)
	if err != nil || all.Period != nil || all.Items[0].CreatedInPeriod != nil || all.ReturnedCount != 2 {
		t.Fatalf("all comments: %+v %v", all, err)
	}
}

func TestHistoryEmptyAndUnavailableDates(t *testing.T) {
	empty, err := normalizeChangelogs(nil, testHistoryPeriod(t))
	if err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal("empty changes not represented as known empty")
	}
	comments, err := normalizeComments(nil, testHistoryPeriod(t), nil)
	if err != nil || comments.Items == nil || *comments.MatchingCount != 0 || !comments.RetrievalComplete {
		t.Fatal("empty comments not complete")
	}
	for _, date := range []string{"", "yesterday"} {
		if _, err := normalizeChangelogs([]jira.Changelog{{ID: "1", Created: date}}, testHistoryPeriod(t)); err == nil {
			t.Fatal("accepted undated history")
		}
		if _, err := normalizeComments([]jira.Comment{{ID: "1", Created: date, Updated: "2026-10-25T00:00:00Z"}}, testHistoryPeriod(t), nil); err == nil {
			t.Fatal("accepted undated comment")
		}
	}
}

type fakeHistoryClient struct {
	*fakeBriefingJiraClient
	changes      []jira.Changelog
	comments     []jira.Comment
	commentCalls int
	changeCalls  int
}

func (f *fakeHistoryClient) BaseURL() string { return "https://jira.example" }
func (f *fakeHistoryClient) GetChangelogs(context.Context, string) ([]jira.Changelog, error) {
	f.changeCalls++
	return f.changes, nil
}
func (f *fakeHistoryClient) GetComments(context.Context, string) ([]jira.Comment, error) {
	f.commentCalls++
	return f.comments, nil
}

func TestInspectionHistoryExportAndMarkdown(t *testing.T) {
	root := exportFixture(t, `{"id":"1","key":"EX-1","fields":{"summary":"Current state","updated":"2026-10-29T10:00:00Z","comment":{"comments":[{"id":"embedded-only","body":"Do not render this subset"}]}}}`)
	client := &fakeHistoryClient{fakeBriefingJiraClient: &fakeBriefingJiraClient{tickets: map[string]*jira.Ticket{"EX-1": &root}}, comments: []jira.Comment{
		{ID: "1", Created: "2020-01-01T00:00:00Z", Updated: "2026-10-25T00:30:00Z", Body: "Updated discussion"},
		{ID: "2", Created: "2026-10-25T00:10:00Z", Updated: "2026-10-25T00:10:00Z", Body: "Less recent"},
	}}
	limit := 1
	opts := TicketInspectionOptions{CommentsPeriod: testHistoryPeriod(t), ChangesPeriod: testHistoryPeriod(t), CommentsLimit: &limit}
	result, err := NewBriefingService(client).GenerateTicketInspectionExport(context.Background(), "default", client.BaseURL(), "EX-1", opts, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !result.OutputTruncated || result.Issues[0].Changes == nil || !result.Issues[0].Comments.RetrievalComplete || client.commentCalls != 1 || client.changeCalls != 1 {
		t.Fatal("history not applied")
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "comments_output_limited" {
		t.Fatalf("stale warnings %+v", result.Warnings)
	}
	if !strings.Contains(*result.Issues[0].Comments.Items[0].URL, "/browse/EX-1?focusedCommentId=1#comment-1") {
		t.Fatal("missing comment browser link")
	}
	text, err := NewBriefingService(client).GenerateTicketInspectionWithOptions(context.Background(), "EX-1", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Updated discussion") || strings.Contains(text, "embedded-only") || strings.Contains(text, "Less recent") || !strings.Contains(text, "matching comments: 2") || !strings.Contains(text, "selection truncated: true") {
		t.Fatalf("bad filtered Markdown: %s", text)
	}
}

func TestHistoryMarkdownTruncationPreservesUnicode(t *testing.T) {
	comments := []jira.Comment{{ID: "1", Created: "2026-10-25T00:00:00Z", Updated: "2026-10-25T00:00:00Z", Body: strings.Repeat("🦊", maxBriefingCommentsChars+1)}}
	normalized, err := normalizeComments(comments, testHistoryPeriod(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	appendHistoryComments(&text, normalized)
	if !strings.Contains(text.String(), "body output truncated: true") || strings.ContainsRune(text.String(), '\uFFFD') {
		t.Fatal("invalid or undisclosed Unicode truncation")
	}
	if normalized.OutputTruncated {
		t.Fatal("Markdown budget changed the JSON selection metadata")
	}
}

func TestInspectionHistoryEnrichesSelectedRelatedIssues(t *testing.T) {
	root := exportFixture(t, `{"id":"1","key":"EX-1","fields":{"parent":{"id":"2","key":"EX-2"}}}`)
	parent := exportFixture(t, `{"id":"2","key":"EX-2","fields":{}}`)
	client := &fakeHistoryClient{fakeBriefingJiraClient: &fakeBriefingJiraClient{tickets: map[string]*jira.Ticket{"EX-1": &root, "EX-2": &parent}}}
	result, err := NewBriefingService(client).GenerateTicketInspectionExport(context.Background(), "team", client.BaseURL(), "EX-1", TicketInspectionOptions{Depth: 1, MaxRelated: 1, ChangesPeriod: testHistoryPeriod(t), CommentsPeriod: testHistoryPeriod(t)}, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.IssueCount != 2 || client.commentCalls != 2 || client.changeCalls != 2 {
		t.Fatal("related issue history missing")
	}
	for _, issue := range result.Issues {
		if issue.Changes.Items == nil || issue.Comments.Items == nil || !issue.Comments.RetrievalComplete {
			t.Fatal("empty history not distinguished from unavailable")
		}
	}
}
