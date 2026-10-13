package services

import (
	"testing"
	"time"

	"ssk/internal/services/jira"
)

func TestBuildAutoFillDateWindow(t *testing.T) {
	now := time.Date(2026, 4, 2, 15, 30, 0, 0, time.UTC)

	dates := BuildAutoFillDateWindow(now)

	if len(dates) != 6 {
		t.Fatalf("BuildAutoFillDateWindow() len = %d, want 6", len(dates))
	}

	if got := dates[0].Format("2006-01-02"); got != "2026-03-28" {
		t.Fatalf("first date = %s, want 2026-03-28", got)
	}

	if got := dates[5].Format("2006-01-02"); got != "2026-04-02" {
		t.Fatalf("last date = %s, want 2026-04-02", got)
	}
}

func TestIsTicketEligibleForDay_RequiresCreationOnOrBeforeTargetDay(t *testing.T) {
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	ticket := testTicket("PROJ-1", "1001", "2026-04-02T09:00:00.000+0000", "2026-04-02T10:00:00.000+0000", "")

	if IsTicketEligibleForDay(ticket, day, time.UTC) {
		t.Fatal("ticket should not be eligible before its creation date")
	}
}

func TestIsTicketEligibleForDay_RejectsSameDayCompletion(t *testing.T) {
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	ticket := testTicket("PROJ-1", "1001", "2026-03-20T09:00:00.000+0000", "2026-04-01T10:00:00.000+0000", "2026-04-01T17:30:00.000+0000")

	if IsTicketEligibleForDay(ticket, day, time.UTC) {
		t.Fatal("ticket should not be eligible on its resolution day")
	}
}

func TestIsTicketEligibleForDay_AllowsLaterCompletion(t *testing.T) {
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	ticket := testTicket("PROJ-1", "1001", "2026-03-20T09:00:00.000+0000", "2026-04-02T10:00:00.000+0000", "2026-04-02T17:30:00.000+0000")

	if !IsTicketEligibleForDay(ticket, day, time.UTC) {
		t.Fatal("ticket should be eligible before its resolution day")
	}
}

func TestSelectMostRecentlyUpdatedEligibleTicket(t *testing.T) {
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	tickets := []jira.Ticket{
		testTicket("PROJ-1", "1001", "2026-03-20T09:00:00.000+0000", "2026-04-01T09:00:00.000+0000", ""),
		testTicket("PROJ-2", "1002", "2026-03-21T09:00:00.000+0000", "2026-04-01T11:00:00.000+0000", ""),
		testTicket("PROJ-3", "1003", "2026-04-02T09:00:00.000+0000", "2026-04-01T12:00:00.000+0000", ""),
	}

	selected, ok := SelectMostRecentlyUpdatedEligibleTicket(tickets, day, time.UTC)
	if !ok {
		t.Fatal("expected an eligible ticket")
	}

	if selected.Key != "PROJ-2" {
		t.Fatalf("selected key = %s, want PROJ-2", selected.Key)
	}
}

func TestAutoFillDay_DryRun(t *testing.T) {
	service := &JiraTempoService{}
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	tickets := []jira.Ticket{
		testTicket("PROJ-1", "1001", "2026-03-20T09:00:00.000+0000", "2026-04-01T11:00:00.000+0000", ""),
	}
	timeSpent := map[string]int{
		"2026-04-01": 3 * 3600,
	}

	result := service.autoFillDay(day, nil, timeSpent, tickets, true)

	if result.Status != "filled" {
		t.Fatalf("status = %s, want filled", result.Status)
	}

	if result.TicketKey != "PROJ-1" {
		t.Fatalf("ticket = %s, want PROJ-1", result.TicketKey)
	}

	if result.AddedSeconds != 5*3600 {
		t.Fatalf("added seconds = %d, want %d", result.AddedSeconds, 5*3600)
	}
}

func TestAutoFillDay_SkipsAlreadyAboveTarget(t *testing.T) {
	service := &JiraTempoService{}
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	timeSpent := map[string]int{
		"2026-04-01": 9 * 3600,
	}

	result := service.autoFillDay(day, nil, timeSpent, nil, true)

	if result.Status != "skipped" {
		t.Fatalf("status = %s, want skipped", result.Status)
	}

	if result.Reason != "already above target" {
		t.Fatalf("reason = %s, want already above target", result.Reason)
	}
}

func testTicket(key, id, created, updated, resolution string) jira.Ticket {
	fields := map[string]interface{}{
		"summary": "Test summary",
		"created": created,
		"updated": updated,
	}
	if resolution != "" {
		fields["resolutiondate"] = resolution
	}

	return jira.Ticket{
		ID:     id,
		Key:    key,
		Fields: fields,
	}
}
