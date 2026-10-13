package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ssk/internal/services/jira"
)

type fakeBriefingJiraClient struct {
	candidates []jira.Ticket
	tickets    map[string]*jira.Ticket
	fetched    []string
}

func (f *fakeBriefingJiraClient) GetBriefingCandidateTickets() ([]jira.Ticket, error) {
	return f.candidates, nil
}

func (f *fakeBriefingJiraClient) GetBriefingTicketByKey(issueKey string) (*jira.Ticket, error) {
	f.fetched = append(f.fetched, issueKey)
	ticket, ok := f.tickets[issueKey]
	if !ok {
		return nil, errors.New("ticket not found")
	}
	return ticket, nil
}

func TestBriefingService_FetchesAllCandidatesInOrder(t *testing.T) {
	client := &fakeBriefingJiraClient{
		candidates: []jira.Ticket{
			{Key: "PROJ-2", ID: "1002", Fields: map[string]interface{}{"summary": "Second"}},
			{Key: "PROJ-1", ID: "1001", Fields: map[string]interface{}{"summary": "First"}},
		},
		tickets: map[string]*jira.Ticket{
			"PROJ-2": {Key: "PROJ-2", ID: "1002", Fields: map[string]interface{}{"summary": "Second full"}},
			"PROJ-1": {Key: "PROJ-1", ID: "1001", Fields: map[string]interface{}{"summary": "First full"}},
		},
	}

	service := NewBriefingService(client)
	context, err := service.GenerateBriefing(context.Background())
	if err != nil {
		t.Fatalf("GenerateBriefing() error = %v", err)
	}

	if got := strings.Join(client.fetched, ","); got != "PROJ-2,PROJ-1" {
		t.Fatalf("fetched tickets = %s, want PROJ-2,PROJ-1", got)
	}

	for _, expected := range []string{
		"# Jira Briefing Context",
		"intended for a code agent",
		"Ticket count: 2",
		"## Ticket 1: PROJ-2",
		"## Ticket 2: PROJ-1",
		"- Summary: Second full",
		"- Summary: First full",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing %q:\n%s", expected, context)
		}
	}
}

func TestBriefingService_LimitsCandidatesBeforeFullFetch(t *testing.T) {
	client := &fakeBriefingJiraClient{
		candidates: []jira.Ticket{
			{Key: "PROJ-3", ID: "1003", Fields: map[string]interface{}{"summary": "Third"}},
			{Key: "PROJ-2", ID: "1002", Fields: map[string]interface{}{"summary": "Second"}},
			{Key: "PROJ-1", ID: "1001", Fields: map[string]interface{}{"summary": "First"}},
		},
		tickets: map[string]*jira.Ticket{
			"PROJ-3": {Key: "PROJ-3", ID: "1003", Fields: map[string]interface{}{"summary": "Third full"}},
			"PROJ-2": {Key: "PROJ-2", ID: "1002", Fields: map[string]interface{}{"summary": "Second full"}},
			"PROJ-1": {Key: "PROJ-1", ID: "1001", Fields: map[string]interface{}{"summary": "First full"}},
		},
	}

	service := NewBriefingService(client)
	service.SetMaxTickets(2)

	output, err := service.GenerateBriefing(context.Background())
	if err != nil {
		t.Fatalf("GenerateBriefing() error = %v", err)
	}

	if got := strings.Join(client.fetched, ","); got != "PROJ-3,PROJ-2" {
		t.Fatalf("fetched tickets = %s, want PROJ-3,PROJ-2", got)
	}

	if strings.Contains(output, "PROJ-1") {
		t.Fatalf("context included ticket beyond max limit: %s", output)
	}
}

func TestBriefingService_RejectsInvalidMaxTickets(t *testing.T) {
	client := &fakeBriefingJiraClient{
		candidates: []jira.Ticket{{Key: "PROJ-1", ID: "1001"}},
		tickets:    map[string]*jira.Ticket{},
	}
	service := NewBriefingService(client)
	service.SetMaxTickets(0)

	_, err := service.GenerateBriefing(context.Background())
	if err == nil {
		t.Fatal("GenerateBriefing() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "maximum Jira ticket count") {
		t.Fatalf("error = %q, want max ticket validation", err.Error())
	}
}

func TestBriefingService_NoCandidate(t *testing.T) {
	service := NewBriefingService(&fakeBriefingJiraClient{})

	_, err := service.GenerateBriefing(context.Background())
	if err == nil {
		t.Fatal("GenerateBriefing() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "no assigned non-done Jira ticket updated in the last 30 days") {
		t.Fatalf("error = %q, want no candidate message", err.Error())
	}
}

func TestBriefingService_GenerateTicketInspectionFetchesDirectRelationships(t *testing.T) {
	client := &fakeBriefingJiraClient{
		tickets: map[string]*jira.Ticket{
			"PROJ-1": {
				Key: "PROJ-1",
				ID:  "1001",
				Fields: map[string]interface{}{
					"summary": "Root ticket",
					"parent":  map[string]interface{}{"key": "PROJ-100"},
					"subtasks": []interface{}{
						map[string]interface{}{"key": "PROJ-2"},
						map[string]interface{}{"key": "PROJ-3"},
					},
					"issuelinks": []interface{}{
						map[string]interface{}{
							"type":         map[string]interface{}{"outward": "blocks"},
							"outwardIssue": map[string]interface{}{"key": "PROJ-4"},
						},
						map[string]interface{}{
							"type":        map[string]interface{}{"inward": "is blocked by"},
							"inwardIssue": map[string]interface{}{"key": "PROJ-5"},
						},
					},
				},
			},
			"PROJ-100": {Key: "PROJ-100", ID: "1100", Fields: map[string]interface{}{"summary": "Parent"}},
			"PROJ-2":   {Key: "PROJ-2", ID: "1002", Fields: map[string]interface{}{"summary": "First subtask"}},
			"PROJ-3":   {Key: "PROJ-3", ID: "1003", Fields: map[string]interface{}{"summary": "Second subtask"}},
			"PROJ-4":   {Key: "PROJ-4", ID: "1004", Fields: map[string]interface{}{"summary": "Blocked ticket"}},
			"PROJ-5":   {Key: "PROJ-5", ID: "1005", Fields: map[string]interface{}{"summary": "Blocking ticket"}},
		},
	}

	service := NewBriefingService(client)
	output, err := service.GenerateTicketInspection(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatalf("GenerateTicketInspection() error = %v", err)
	}

	if got := strings.Join(client.fetched, ","); got != "PROJ-1,PROJ-100,PROJ-2,PROJ-3,PROJ-4,PROJ-5" {
		t.Fatalf("fetched tickets = %s, want direct relationship order", got)
	}

	for _, expected := range []string{
		"# Jira Ticket Inspection Context",
		"Root ticket: PROJ-1",
		"Ticket count: 6",
		"## Ticket 1: PROJ-1",
		"## Ticket 2: PROJ-100",
		"## Ticket 6: PROJ-5",
		"- PROJ-100 parent of PROJ-1 (included)",
		"- PROJ-1 parent of PROJ-2 (included)",
		"- PROJ-1 parent of PROJ-3 (included)",
		"- PROJ-1 blocks PROJ-4 (included)",
		"- PROJ-1 is blocked by PROJ-5 (included)",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("inspection context missing %q:\n%s", expected, output)
		}
	}
}

func TestBriefingService_GenerateTicketInspectionUsesCanonicalRootKey(t *testing.T) {
	client := &fakeBriefingJiraClient{
		tickets: map[string]*jira.Ticket{
			"OLD-1": {Key: "NEW-1", ID: "1001", Fields: map[string]interface{}{"summary": "Moved ticket"}},
		},
	}

	service := NewBriefingService(client)
	output, err := service.GenerateTicketInspection(context.Background(), "OLD-1")
	if err != nil {
		t.Fatalf("GenerateTicketInspection() error = %v", err)
	}
	if !strings.Contains(output, "Root ticket: NEW-1") {
		t.Fatalf("inspection context does not use canonical root key:\n%s", output)
	}
}

func TestBriefingService_GenerateTicketInspectionDefaultsToDepthTwo(t *testing.T) {
	client := &fakeBriefingJiraClient{
		tickets: map[string]*jira.Ticket{
			"PROJ-1": testInspectionTicket("PROJ-1", "PROJ-2", "PROJ-3"),
			"PROJ-2": testInspectionTicket("PROJ-2", "PROJ-4"),
			"PROJ-3": testInspectionTicket("PROJ-3", "PROJ-5"),
			"PROJ-4": testInspectionTicket("PROJ-4", "PROJ-6"),
			"PROJ-5": testInspectionTicket("PROJ-5"),
			"PROJ-6": testInspectionTicket("PROJ-6"),
		},
	}

	service := NewBriefingService(client)
	output, err := service.GenerateTicketInspection(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatalf("GenerateTicketInspection() error = %v", err)
	}

	if got := strings.Join(client.fetched, ","); got != "PROJ-1,PROJ-2,PROJ-3,PROJ-4,PROJ-5" {
		t.Fatalf("fetched tickets = %s, want breadth-first depth 2 fetch", got)
	}

	if strings.Contains(output, "## Ticket 6: PROJ-6") {
		t.Fatalf("inspection context included depth 3 ticket:\n%s", output)
	}
	if !strings.Contains(output, "- PROJ-4 blocks PROJ-6 (external)") {
		t.Fatalf("inspection context should keep depth 3 relationship external:\n%s", output)
	}
}

func TestBriefingService_GenerateTicketInspectionRespectsMaxRelated(t *testing.T) {
	client := &fakeBriefingJiraClient{
		tickets: map[string]*jira.Ticket{
			"PROJ-1": testInspectionTicket("PROJ-1", "PROJ-2", "PROJ-3", "PROJ-4"),
			"PROJ-2": testInspectionTicket("PROJ-2", "PROJ-5"),
			"PROJ-3": testInspectionTicket("PROJ-3"),
			"PROJ-4": testInspectionTicket("PROJ-4"),
			"PROJ-5": testInspectionTicket("PROJ-5"),
		},
	}

	service := NewBriefingService(client)
	output, err := service.GenerateTicketInspectionWithOptions(context.Background(), "PROJ-1", TicketInspectionOptions{
		Depth:      2,
		MaxRelated: 2,
	})
	if err != nil {
		t.Fatalf("GenerateTicketInspectionWithOptions() error = %v", err)
	}

	if got := strings.Join(client.fetched, ","); got != "PROJ-1,PROJ-2,PROJ-3" {
		t.Fatalf("fetched tickets = %s, want root plus first 2 related tickets", got)
	}

	if !strings.Contains(output, "Ticket count: 3") {
		t.Fatalf("inspection context should include 3 tickets:\n%s", output)
	}
	if !strings.Contains(output, "- PROJ-1 blocks PROJ-4 (external)") {
		t.Fatalf("capped related ticket should be marked external:\n%s", output)
	}
}

func testInspectionTicket(key string, linkedKeys ...string) *jira.Ticket {
	links := make([]interface{}, 0, len(linkedKeys))
	for _, linkedKey := range linkedKeys {
		links = append(links, map[string]interface{}{
			"type":         map[string]interface{}{"outward": "blocks"},
			"outwardIssue": map[string]interface{}{"key": linkedKey},
		})
	}

	return &jira.Ticket{
		Key: key,
		ID:  key + "-id",
		Fields: map[string]interface{}{
			"summary":    key + " summary",
			"issuelinks": links,
		},
	}
}

func TestExtractRelatedTicketKeysDeduplicatesAndSkipsRoot(t *testing.T) {
	ticket := jira.Ticket{
		Key: "PROJ-1",
		Fields: map[string]interface{}{
			"parent": map[string]interface{}{"key": "PROJ-100"},
			"subtasks": []interface{}{
				map[string]interface{}{"key": "PROJ-2"},
				map[string]interface{}{"key": "PROJ-2"},
			},
			"issuelinks": []interface{}{
				map[string]interface{}{
					"outwardIssue": map[string]interface{}{"key": "PROJ-1"},
					"inwardIssue":  map[string]interface{}{"key": "PROJ-3"},
				},
			},
		},
	}

	keys := ExtractRelatedTicketKeys(ticket)
	if got := strings.Join(keys, ","); got != "PROJ-100,PROJ-2,PROJ-3" {
		t.Fatalf("related keys = %s, want PROJ-100,PROJ-2,PROJ-3", got)
	}
}

func TestBuildBriefingContextIncludesKeyFields(t *testing.T) {
	ticket := jira.Ticket{
		Key: "PROJ-123",
		ID:  "100123",
		Fields: map[string]interface{}{
			"summary":     "Add briefing command",
			"description": map[string]interface{}{"type": "doc", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "Description details"}}}}},
			"status":      map[string]interface{}{"name": "In Progress"},
			"priority":    map[string]interface{}{"name": "High"},
			"comment":     map[string]interface{}{"comments": []interface{}{map[string]interface{}{"author": map[string]interface{}{"displayName": "Jane Doe"}, "body": map[string]interface{}{"type": "doc", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "Please focus on permissions"}}}}}}}},
		},
	}

	context, err := BuildBriefingContext([]jira.Ticket{ticket})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	for _, expected := range []string{
		"- Key: PROJ-123",
		"- ID: 100123",
		"- Summary: Add briefing command",
		"Status:",
		"Priority:",
		"Description:",
		"Description details",
		"Comments:",
		"Jane Doe",
		"Please focus on permissions",
		"Do not invent facts",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing %q:\n%s", expected, context)
		}
	}

	if strings.Contains(context, "```json") {
		t.Fatalf("context should not include raw JSON fences:\n%s", context)
	}
}

func TestBuildBriefingContextIncludesDevelopmentInfo(t *testing.T) {
	ticket := jira.Ticket{
		Key: "PROJ-123",
		ID:  "100123",
		Fields: map[string]interface{}{
			"summary": "Ticket with GitHub links",
			jira.FieldDevelopmentInfo: []interface{}{
				map[string]interface{}{
					"dataType":        "pullrequest",
					"applicationType": "GitHub",
					"detail": []interface{}{
						map[string]interface{}{
							"repositories": []interface{}{
								map[string]interface{}{
									"name": "example-repo",
									"url":  "https://github.com/acme/example-repo",
									"pullRequests": []interface{}{
										map[string]interface{}{
											"title":       "Add Jira inspection context",
											"url":         "https://github.com/acme/example-repo/pull/42",
											"status":      "OPEN",
											"source":      map[string]interface{}{"name": "feature/jira-inspect"},
											"destination": map[string]interface{}{"name": "main"},
										},
									},
									"branches": []interface{}{
										map[string]interface{}{
											"name":       "feature/jira-inspect",
											"url":        "https://github.com/acme/example-repo/tree/feature/jira-inspect",
											"repository": map[string]interface{}{"name": "example-repo"},
										},
									},
									"commits": []interface{}{
										map[string]interface{}{
											"displayId":       "abc1234",
											"message":         "Add Jira inspection context",
											"url":             "https://github.com/acme/example-repo/commit/abc1234",
											"author":          map[string]interface{}{"name": "Jane Doe"},
											"authorTimestamp": "2026-05-29T08:00:00.000+0000",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	context, err := BuildBriefingContext([]jira.Ticket{ticket})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	for _, expected := range []string{
		"Development Info:",
		"Repositories:",
		"example-repo (https://github.com/acme/example-repo)",
		"Pull Requests:",
		"Add Jira inspection context (OPEN, source=feature/jira-inspect, target=main, https://github.com/acme/example-repo/pull/42)",
		"Branches:",
		"feature/jira-inspect (repository=example-repo, https://github.com/acme/example-repo/tree/feature/jira-inspect)",
		"Commits:",
		"abc1234 - Add Jira inspection context (author=Jane Doe, 2026-05-29T08:00:00.000+0000, https://github.com/acme/example-repo/commit/abc1234)",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing development info %q:\n%s", expected, context)
		}
	}
}

func TestBuildBriefingContextPreservesAtlassianDocLinks(t *testing.T) {
	ticket := jira.Ticket{
		Key: "PROJ-123",
		ID:  "100123",
		Fields: map[string]interface{}{
			"summary": "Ticket with links",
			"description": map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Confluence page",
								"marks": []interface{}{
									map[string]interface{}{
										"type":  "link",
										"attrs": map[string]interface{}{"href": "https://example.atlassian.net/wiki/spaces/ABC/pages/123"},
									},
								},
							},
						},
					},
					map[string]interface{}{
						"type":  "inlineCard",
						"attrs": map[string]interface{}{"url": "https://example.atlassian.net/browse/PROJ-456"},
					},
				},
			},
		},
	}

	context, err := BuildBriefingContext([]jira.Ticket{ticket})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	for _, expected := range []string{
		"Confluence page (https://example.atlassian.net/wiki/spaces/ABC/pages/123)",
		"https://example.atlassian.net/browse/PROJ-456",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing link %q:\n%s", expected, context)
		}
	}
}

func TestBuildBriefingContextRendersAgentUsefulAtlassianDocNodes(t *testing.T) {
	ticket := jira.Ticket{
		Key: "PROJ-123",
		ID:  "100123",
		Fields: map[string]interface{}{
			"summary": "Ticket with rich description",
			"description": map[string]interface{}{
				"type": "doc",
				"content": []interface{}{
					map[string]interface{}{
						"type": "paragraph",
						"content": []interface{}{
							map[string]interface{}{"type": "mention", "attrs": map[string]interface{}{"text": "@Jane Doe"}},
							map[string]interface{}{"type": "text", "text": " set status to "},
							map[string]interface{}{"type": "status", "attrs": map[string]interface{}{"text": "READY"}},
							map[string]interface{}{"type": "text", "text": " "},
							map[string]interface{}{"type": "emoji", "attrs": map[string]interface{}{"shortName": ":rocket:"}},
							map[string]interface{}{"type": "text", "text": " on "},
							map[string]interface{}{"type": "date", "attrs": map[string]interface{}{"timestamp": "1782864000000"}},
						},
					},
					map[string]interface{}{
						"type": "bulletList",
						"content": []interface{}{
							map[string]interface{}{"type": "listItem", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "first item"}}}}},
							map[string]interface{}{"type": "listItem", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "second item"}}}}},
						},
					},
					map[string]interface{}{
						"type": "orderedList",
						"content": []interface{}{
							map[string]interface{}{"type": "listItem", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "ordered item"}}}}},
						},
					},
					map[string]interface{}{
						"type":  "codeBlock",
						"attrs": map[string]interface{}{"language": "go"},
						"content": []interface{}{
							map[string]interface{}{"type": "text", "text": "fmt.Println(\"hello\")"},
						},
					},
					map[string]interface{}{
						"type": "blockquote",
						"content": []interface{}{
							map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "quoted context"}}},
						},
					},
					map[string]interface{}{
						"type":  "panel",
						"attrs": map[string]interface{}{"panelType": "warning"},
						"content": []interface{}{
							map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "panel context"}}},
						},
					},
					map[string]interface{}{
						"type":  "expand",
						"attrs": map[string]interface{}{"title": "More details"},
						"content": []interface{}{
							map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "expanded context"}}},
						},
					},
					map[string]interface{}{
						"type": "taskList",
						"content": []interface{}{
							map[string]interface{}{"type": "taskItem", "attrs": map[string]interface{}{"state": "DONE"}, "content": []interface{}{map[string]interface{}{"type": "text", "text": "done task"}}},
						},
					},
					map[string]interface{}{
						"type": "decisionList",
						"content": []interface{}{
							map[string]interface{}{"type": "decisionItem", "content": []interface{}{map[string]interface{}{"type": "text", "text": "ship it"}}},
						},
					},
					map[string]interface{}{
						"type": "table",
						"content": []interface{}{
							map[string]interface{}{"type": "tableRow", "content": []interface{}{
								map[string]interface{}{"type": "tableHeader", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "Name"}}}}},
								map[string]interface{}{"type": "tableHeader", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "Value"}}}}},
							}},
							map[string]interface{}{"type": "tableRow", "content": []interface{}{
								map[string]interface{}{"type": "tableCell", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "API"}}}}},
								map[string]interface{}{"type": "tableCell", "content": []interface{}{map[string]interface{}{"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "GET /files"}}}}},
							}},
						},
					},
					map[string]interface{}{
						"type": "mediaSingle",
						"content": []interface{}{
							map[string]interface{}{"type": "media", "attrs": map[string]interface{}{"alt": "architecture diagram", "id": "media-1", "type": "file"}},
						},
					},
				},
			},
		},
	}

	context, err := BuildBriefingContext([]jira.Ticket{ticket})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	for _, expected := range []string{
		"@Jane Doe set status to READY :rocket: on 1782864000000",
		"- first item",
		"- second item",
		"1. ordered item",
		"```go\nfmt.Println(\"hello\")\n```",
		"> quoted context",
		"Panel (warning):\npanel context",
		"More details:\nexpanded context",
		"- [x] done task",
		"- Decision: ship it",
		"| Name | Value |",
		"| API | GET /files |",
		"Media: alt=architecture diagram, type=file, id=media-1",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing rich ADF node %q:\n%s", expected, context)
		}
	}
}

func TestBuildBriefingContextTruncatesLargeFields(t *testing.T) {
	largeDescription := strings.Repeat("description ", maxBriefingDescriptionChars)
	ticket := jira.Ticket{
		Key: "PROJ-123",
		ID:  "100123",
		Fields: map[string]interface{}{
			"summary":     "Large ticket",
			"description": map[string]interface{}{"content": largeDescription},
			"comment":     map[string]interface{}{"comments": []interface{}{map[string]interface{}{"body": largeDescription}}},
		},
	}

	context, err := BuildBriefingContext([]jira.Ticket{ticket})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	if !strings.Contains(context, "truncated") {
		t.Fatalf("context did not mention truncation")
	}
}

func TestBuildBriefingContextHandlesIssueObjectsWithNilFields(t *testing.T) {
	ticket := jira.Ticket{
		Key: "PROJ-123",
		ID:  "100123",
		Fields: map[string]interface{}{
			"parent": map[string]interface{}{
				"key":    "PROJ-100",
				"fields": nil,
			},
			"issuelinks": []interface{}{
				map[string]interface{}{
					"type": map[string]interface{}{
						"outward": "blocks",
					},
					"outwardIssue": map[string]interface{}{
						"key":    "PROJ-200",
						"fields": nil,
					},
				},
			},
		},
	}

	context, err := BuildBriefingContext([]jira.Ticket{ticket})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	for _, expected := range []string{"Parent: PROJ-100", "Issue Links: blocks PROJ-200"} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing %q:\n%s", expected, context)
		}
	}
}

func TestBuildBriefingContextIncludesTicketRelationships(t *testing.T) {
	parent := jira.Ticket{
		Key:    "PROJ-100",
		ID:     "100100",
		Fields: map[string]interface{}{"summary": "Parent ticket"},
	}
	child := jira.Ticket{
		Key: "PROJ-101",
		ID:  "100101",
		Fields: map[string]interface{}{
			"summary": "Child ticket",
			"parent": map[string]interface{}{
				"key": "PROJ-100",
				"fields": map[string]interface{}{
					"summary": "Parent ticket",
				},
			},
			"issuelinks": []interface{}{
				map[string]interface{}{
					"type": map[string]interface{}{
						"outward": "blocks",
					},
					"outwardIssue": map[string]interface{}{
						"key": "PROJ-102",
						"fields": map[string]interface{}{
							"summary": "Blocked ticket",
						},
					},
				},
			},
		},
	}
	blocked := jira.Ticket{
		Key:    "PROJ-102",
		ID:     "100102",
		Fields: map[string]interface{}{"summary": "Blocked ticket"},
	}

	context, err := BuildBriefingContext([]jira.Ticket{child, parent, blocked})
	if err != nil {
		t.Fatalf("BuildBriefingContext() error = %v", err)
	}

	for _, expected := range []string{
		"## Ticket Relationships",
		"- PROJ-100 parent of PROJ-101 (included)",
		"- PROJ-101 blocks PROJ-102 (included)",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("context missing relationship %q:\n%s", expected, context)
		}
	}
}
