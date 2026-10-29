package services

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ssk/internal/services/jira"
)

func exportFixture(t *testing.T, raw string) jira.Ticket {
	t.Helper()
	var ticket jira.Ticket
	if err := json.Unmarshal([]byte(raw), &ticket); err != nil {
		t.Fatal(err)
	}
	return ticket
}

func TestJiraExportNormalizesAndAllowlistsFields(t *testing.T) {
	ticket := exportFixture(t, `{
 "id":"1001","key":"EX-1","fields":{
 "summary":"Unicode café 🦊","description":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Details é"}]}]},
 "project":{"id":"10","key":"EX","name":"Example"},
 "status":{"id":"1","name":"Open","statusCategory":{"id":2,"key":"new","name":"To Do"}},
 "resolution":null,"created":"2026-10-25T02:30:00.000+0200","updated":"2026-10-25T02:30:00+01:00",
 "assignee":{"accountId":"person-1","displayName":"John Doe","emailAddress":"private@example.com"},
 "parent":{"id":"1000","key":"EX-0","fields":{"summary":"Parent"}},
 "subtasks":[],"issuelinks":[{"id":"link-1","type":{"id":"blocks","name":"Blocks","outward":"blocks","inward":"is blocked by"},"outwardIssue":{"id":"1002","key":"EX-2"}}],
 "components":[],"labels":[],"fixVersions":[{"id":"v1","name":"1.0"}],
 "comment":{"total":50,"comments":[{"id":"c1","author":{"displayName":"John Doe","emailAddress":"private@example.com"},"created":"2026-10-25T01:00:00Z","body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"A comment"}]}]}}]},
 "custom_secret":"api-token","attachment":[{"content":"https://jira.example/file?token=secret"}],"_developmentInfo":[{"token":"secret"}]}}
 `)
	started := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	jql := "project = EX"
	result, err := BuildJiraExport("jira.query", "other", "https://jira.example/team/", &jql, started, []jira.Ticket{ticket, ticket})
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 1 || result.IssueCount != 1 || !result.RetrievalComplete || result.OutputTruncated {
		t.Fatalf("bad metadata: %+v", result)
	}
	issue := result.Issues[0]
	if issue.Workspace != "other" || issue.URL != "https://jira.example/team/browse/EX-1" || *issue.Description != "Details é" {
		t.Fatalf("bad issue identity/body: %+v", issue)
	}
	if issue.Status.Category == nil || *issue.Status.Category.ID != "2" {
		t.Fatal("lost native status category ID")
	}
	if issue.Parent == nil || *issue.Parent.Key != "EX-0" || issue.Parent.Workspace != "other" || len(issue.IssueLinks) != 1 || *issue.IssueLinks[0].OutwardIssue.ID != "1002" {
		t.Fatal("lost relationships")
	}
	if issue.CreatedAt.Format(time.RFC3339) != "2026-10-25T00:30:00Z" || issue.UpdatedAt.Format(time.RFC3339) != "2026-10-25T01:30:00Z" {
		t.Fatal("incorrect DST timestamp normalization")
	}
	if issue.Comments == nil || issue.Comments.RetrievalComplete || issue.Comments.ReturnedCount != 1 || *issue.Comments.Items[0].Body != "A comment" || len(result.Warnings) != 1 {
		t.Fatal("embedded comments masquerade as complete")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"emailAddress", "private@example.com", "api-token", "custom_secret", "token=secret", "_developmentInfo"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("export leaks %q", forbidden)
		}
	}
	for _, expected := range []string{`"subtasks":[]`, `"labels":[]`, `"resolution":null`, `"resolved_at":null`, `"description":"Details é"`} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("missing %s: %s", expected, data)
		}
	}
}

func TestJiraExportUnknownAndEmptyStayDistinct(t *testing.T) {
	ticket := jira.Ticket{ID: "1", Key: "EX-1"}
	result, err := BuildJiraExport("jira.query", "default", "https://jira.example", nil, time.Now(), []jira.Ticket{ticket})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result.Issues[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"summary", "description", "comments", "subtasks", "issue_links", "components", "labels", "fix_versions", "created_at"} {
		if !strings.Contains(string(data), `"`+field+`":null`) {
			t.Fatalf("unknown %s is not null: %s", field, data)
		}
	}
	empty, err := BuildJiraExport("jira.query", "default", "https://jira.example", nil, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"issues":[]`) || !strings.Contains(string(data), `"warnings":[]`) || empty.IssueCount != 0 {
		t.Fatalf("bad empty export: %s", data)
	}
}

func TestInspectionExportRetainsUnfetchedRelationships(t *testing.T) {
	root := exportFixture(t, `{"id":"1","key":"EX-1","fields":{"summary":"Root","parent":{"id":"2","key":"EX-2"},"subtasks":[],"issuelinks":[]}}`)
	client := &fakeBriefingJiraClient{tickets: map[string]*jira.Ticket{"EX-1": &root}}
	result, err := NewBriefingService(client).GenerateTicketInspectionExport(context.Background(), "team", "https://jira.example", "EX-1", TicketInspectionOptions{}, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(client.fetched) != 1 || result.IssueCount != 1 || !result.Inspection.RelatedOmitted || result.Issues[0].Parent == nil || len(result.Warnings) != 1 {
		t.Fatalf("wrong bounded inspection: %+v", result)
	}
	if !result.RetrievalComplete || result.OutputTruncated {
		t.Fatal("selection limits confused with output truncation")
	}
}

func TestJiraExportRejectsInvalidIdentityAndSite(t *testing.T) {
	for _, ticket := range []jira.Ticket{{ID: "1"}, {Key: "EX-1"}} {
		if _, err := BuildJiraExport("jira.query", "team", "https://jira.example", nil, time.Now(), []jira.Ticket{ticket}); err == nil {
			t.Fatal("accepted missing identity")
		}
	}
	for _, site := range []string{"not-a-url", "https://user:token@jira.example", "https://jira.example?token=secret", "file:///tmp"} {
		if _, err := BuildJiraExport("jira.query", "team", site, nil, time.Now(), nil); err == nil {
			t.Fatal("accepted unsafe site")
		}
	}
}

func TestJiraExportInvalidDateIsUnknownWithWarning(t *testing.T) {
	result, err := BuildJiraExport("jira.query", "team", "https://jira.example", nil, time.Now(), []jira.Ticket{{ID: "1", Key: "EX-1", Fields: map[string]interface{}{"created": "yesterday"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Issues[0].CreatedAt != nil || len(result.Warnings) != 1 || result.Warnings[0].Code != "invalid_timestamp" {
		t.Fatal("invalid timestamp was not disclosed")
	}
}

type exportAttachmentClient struct {
	*fakeBriefingJiraClient
	fakeAttachmentDownloader
}

func TestInspectionExportAttachmentsPreservePartialFailure(t *testing.T) {
	root := exportFixture(t, `{"id":"1","key":"EX-1","fields":{"attachment":[{"id":"ok","filename":"ok.txt","content":"https://jira.example/ok"},{"id":"bad","filename":"bad.txt","content":"https://jira.example/bad?token=secret"}]}}`)
	client := &exportAttachmentClient{
		fakeBriefingJiraClient:   &fakeBriefingJiraClient{tickets: map[string]*jira.Ticket{"EX-1": &root}},
		fakeAttachmentDownloader: fakeAttachmentDownloader{failFor: "https://jira.example/bad?token=secret", failWithURL: true},
	}
	result, err := NewBriefingService(client).GenerateTicketInspectionExport(context.Background(), "team", "https://jira.example", "EX-1", TicketInspectionOptions{}, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	attachments := result.Issues[0].Attachments
	if len(attachments) != 2 || attachments[0].LocalPath == nil || attachments[1].DownloadError == nil || attachments[1].LocalPath != nil || len(result.Warnings) != 1 {
		t.Fatalf("wrong attachment results: %+v", attachments)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "token=secret") {
		t.Fatal("download error leaked query credentials")
	}
}

func TestJiraExportStableOrderingAndWorkspaceIdentity(t *testing.T) {
	tickets := []jira.Ticket{{ID: "2", Key: "EX-2"}, {ID: "1", Key: "EX-1"}}
	started := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	first, err := BuildJiraExport("jira.query", "first", "https://first.example", nil, started, tickets)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildJiraExport("jira.query", "first", "https://first.example", nil, started, tickets)
	if err != nil {
		t.Fatal(err)
	}
	second.CollectionEndedAt = first.CollectionEndedAt
	one, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one, two) || first.Issues[0].ID != "2" || first.Issues[1].ID != "1" {
		t.Fatal("unstable export order")
	}
	other, err := BuildJiraExport("jira.query", "other", "https://other.example", nil, started, tickets)
	if err != nil {
		t.Fatal(err)
	}
	if other.Issues[0].Workspace == first.Issues[0].Workspace || other.Issues[0].URL == first.Issues[0].URL {
		t.Fatal("workspace identities collide")
	}
}
