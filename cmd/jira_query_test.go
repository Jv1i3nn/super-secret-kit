package cmd

import (
	"strings"
	"testing"

	"ssk/internal/services/jira"
)

func ticketWithStatus(key, summary, status string) jira.Ticket {
	return jira.Ticket{
		Key: key,
		Fields: map[string]interface{}{
			"summary": summary,
			"status":  map[string]interface{}{"name": status},
		},
	}
}

func TestFormatQueryResults_GroupsAndSorts(t *testing.T) {
	out := formatQueryResults([]jira.Ticket{
		ticketWithStatus("SSK-2", "second", "Done"),
		ticketWithStatus("SSK-1", "first", "Done"),
		ticketWithStatus("SSK-3", "third", "In Progress"),
	})

	if !strings.Contains(out, "3 ticket(s)") {
		t.Fatalf("missing count header:\n%s", out)
	}
	// Statuses sorted alphabetically: Done before In Progress.
	if strings.Index(out, "Done (2)") > strings.Index(out, "In Progress (1)") {
		t.Fatalf("statuses not sorted:\n%s", out)
	}
	// Tickets within a status sorted by key: SSK-1 before SSK-2.
	if strings.Index(out, "SSK-1") > strings.Index(out, "SSK-2") {
		t.Fatalf("tickets not sorted by key:\n%s", out)
	}
}

func TestFormatQueryResults_Empty(t *testing.T) {
	if got := formatQueryResults(nil); got != "No tickets matched.\n" {
		t.Fatalf("unexpected empty output: %q", got)
	}
}

func TestFormatQueryResults_MissingStatus(t *testing.T) {
	out := formatQueryResults([]jira.Ticket{{Key: "SSK-9", Fields: map[string]interface{}{"summary": "x"}}})
	if !strings.Contains(out, "(no status)") {
		t.Fatalf("missing status fallback:\n%s", out)
	}
}
