package services

import (
	"fmt"
	"sort"
	"time"

	"ssk/internal/models"
	"ssk/internal/services/jira"
	"ssk/internal/services/tempo"
)

// Ticket is an alias for jira.Ticket for convenience
type Ticket = jira.Ticket

// JiraTempoService combines Jira and Tempo operations
type JiraTempoService struct {
	jiraClient  *jira.Client
	tempoClient *tempo.Client
	accountID   string
}

const targetWorkdaySeconds = 8 * 3600

type AutoFillResult struct {
	From          time.Time
	To            time.Time
	DryRun        bool
	InspectedDays int
	SkippedDays   int
	FilledDays    int
	FailedDays    int
	Days          []AutoFillDayResult
}

type AutoFillDayResult struct {
	Date          time.Time
	Status        string
	Reason        string
	TicketKey     string
	TicketSummary string
	AddedSeconds  int
	AlreadyLogged int
	Error         string
}

// NewJiraTempoService creates a new Jira/Tempo service
func NewJiraTempoService(jiraURL, tempoURL string, jiraCreds, tempoCreds *models.Credentials) (*JiraTempoService, error) {
	jiraClient := jira.NewClient(jiraURL, jiraCreds)
	tempoClient := tempo.NewClient(tempoURL, tempoCreds)

	// Get account ID from Jira
	user, err := jiraClient.GetCurrentUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	return &JiraTempoService{
		jiraClient:  jiraClient,
		tempoClient: tempoClient,
		accountID:   user.AccountID,
	}, nil
}

// GetAssignedTickets retrieves tickets assigned to current user
func (s *JiraTempoService) GetAssignedTickets() ([]jira.Ticket, error) {
	return s.jiraClient.GetAssignedTickets()
}

// GetAutofillCandidateTickets retrieves recently updated assigned tickets for autofill.
func (s *JiraTempoService) GetAutofillCandidateTickets() ([]jira.Ticket, error) {
	return s.jiraClient.GetAutofillCandidateTickets()
}

// GetTicketByKey retrieves a ticket by its key
func (s *JiraTempoService) GetTicketByKey(issueKey string) (*jira.Ticket, error) {
	return s.jiraClient.GetTicketByKey(issueKey, []string{"id", "key", "summary"})
}

// GetWorklogs retrieves worklogs for a date range
func (s *JiraTempoService) GetWorklogs(from, to time.Time) ([]tempo.Worklog, error) {
	return s.tempoClient.GetWorklogs(s.accountID, from, to)
}

// GetDaysOffForCurrentYear retrieves French holidays for the current year
func (s *JiraTempoService) GetDaysOffForCurrentYear() ([]string, error) {
	return s.tempoClient.GetDaysOffForYear(time.Now().Year())
}

// GetDaysOffForYear retrieves holidays for a specific year.
func (s *JiraTempoService) GetDaysOffForYear(year int) ([]string, error) {
	return s.tempoClient.GetDaysOffForYear(year)
}

// GetTimeAlreadySpentByDay calculates time already logged per day
func (s *JiraTempoService) GetTimeAlreadySpentByDay(from, to time.Time) (map[string]int, error) {
	worklogs, err := s.GetWorklogs(from, to)
	if err != nil {
		return nil, err
	}

	timeByDay := make(map[string]int)
	for _, worklog := range worklogs {
		if worklog.StartDate != "" {
			timeByDay[worklog.StartDate] += worklog.TimeSpentSeconds
		}
	}

	return timeByDay, nil
}

// IsWeekend checks if a date is Saturday or Sunday
func IsWeekend(date time.Time) bool {
	weekday := date.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

// IsDayOff checks if a date is in the list of days off
func IsDayOff(date time.Time, daysOff []string) bool {
	dateStr := date.Format("2006-01-02")
	for _, dayOff := range daysOff {
		if dayOff == dateStr {
			return true
		}
	}
	return false
}

// AutoFillTimeGaps fills missing worklog time for today and the previous 5 days.
func (s *JiraTempoService) AutoFillTimeGaps(now time.Time, dryRun bool) (*AutoFillResult, error) {
	dates := BuildAutoFillDateWindow(now)
	if len(dates) == 0 {
		return &AutoFillResult{DryRun: dryRun}, nil
	}

	daysOff, err := s.getDaysOffForDates(dates)
	if err != nil {
		return nil, fmt.Errorf("failed to get holidays: %w", err)
	}

	timeSpent, err := s.GetTimeAlreadySpentByDay(dates[0], dates[len(dates)-1])
	if err != nil {
		return nil, fmt.Errorf("failed to get existing worklogs: %w", err)
	}

	tickets, err := s.GetAutofillCandidateTickets()
	if err != nil {
		return nil, fmt.Errorf("failed to get autofill candidate tickets: %w", err)
	}

	results := make([]AutoFillDayResult, 0, len(dates))
	result := &AutoFillResult{
		From:          dates[0],
		To:            dates[len(dates)-1],
		DryRun:        dryRun,
		InspectedDays: len(dates),
		Days:          results,
	}

	for _, day := range dates {
		dayResult := s.autoFillDay(day, daysOff, timeSpent, tickets, dryRun)
		result.Days = append(result.Days, dayResult)

		switch dayResult.Status {
		case "filled":
			result.FilledDays++
		case "failed":
			result.FailedDays++
		default:
			result.SkippedDays++
		}
	}

	return result, nil
}

// LogWorkTime creates a worklog entry in Tempo
func (s *JiraTempoService) LogWorkTime(issueID int, date time.Time, timeSpentSeconds int) (*tempo.Worklog, error) {
	worklog := tempo.Worklog{
		IssueID:          issueID,
		TimeSpentSeconds: timeSpentSeconds,
		BillableSeconds:  timeSpentSeconds,
		StartDate:        date.Format("2006-01-02"),
		StartTime:        "08:00:00",
		AuthorAccountID:  s.accountID,
	}

	return s.tempoClient.CreateWorklog(worklog)
}

func (s *JiraTempoService) autoFillDay(day time.Time, daysOff []string, timeSpent map[string]int, tickets []jira.Ticket, dryRun bool) AutoFillDayResult {
	alreadyLogged := timeSpent[day.Format("2006-01-02")]
	result := AutoFillDayResult{
		Date:          day,
		AlreadyLogged: alreadyLogged,
	}

	if IsWeekend(day) {
		result.Status = "skipped"
		result.Reason = "weekend"
		return result
	}

	if IsDayOff(day, daysOff) {
		result.Status = "skipped"
		result.Reason = "day off"
		return result
	}

	if alreadyLogged >= targetWorkdaySeconds {
		result.Status = "skipped"
		if alreadyLogged > targetWorkdaySeconds {
			result.Reason = "already above target"
		} else {
			result.Reason = "already logged 8h"
		}
		return result
	}

	selectedTicket, ok := SelectMostRecentlyUpdatedEligibleTicket(tickets, day, day.Location())
	if !ok {
		result.Status = "skipped"
		result.Reason = "no eligible ticket found"
		return result
	}

	result.TicketKey = selectedTicket.Key
	result.TicketSummary = selectedTicket.Summary()
	result.AddedSeconds = targetWorkdaySeconds - alreadyLogged

	if dryRun {
		result.Status = "filled"
		result.Reason = "dry run"
		return result
	}

	issueID, err := parseIssueID(selectedTicket.ID)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}

	if _, err := s.LogWorkTime(issueID, day, result.AddedSeconds); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}

	result.Status = "filled"
	return result
}

func (s *JiraTempoService) getDaysOffForDates(dates []time.Time) ([]string, error) {
	if len(dates) == 0 {
		return nil, nil
	}

	seenYears := map[int]struct{}{}
	var years []int
	for _, date := range dates {
		year := date.Year()
		if _, ok := seenYears[year]; ok {
			continue
		}
		seenYears[year] = struct{}{}
		years = append(years, year)
	}

	sort.Ints(years)

	var daysOff []string
	for _, year := range years {
		holidays, err := s.GetDaysOffForYear(year)
		if err != nil {
			return nil, err
		}
		daysOff = append(daysOff, holidays...)
	}

	return daysOff, nil
}

func BuildAutoFillDateWindow(now time.Time) []time.Time {
	base := normalizeToDay(now)
	dates := make([]time.Time, 0, 6)

	for offset := 5; offset >= 0; offset-- {
		dates = append(dates, base.AddDate(0, 0, -offset))
	}

	return dates
}

func SelectMostRecentlyUpdatedEligibleTicket(tickets []jira.Ticket, day time.Time, loc *time.Location) (jira.Ticket, bool) {
	var (
		selected jira.Ticket
		found    bool
		bestTime time.Time
	)

	for _, ticket := range tickets {
		if !IsTicketEligibleForDay(ticket, day, loc) {
			continue
		}

		updatedAt, ok := ticket.UpdatedAt()
		if !ok {
			updatedAt = time.Time{}
		}

		if !found || updatedAt.After(bestTime) {
			selected = ticket
			bestTime = updatedAt
			found = true
		}
	}

	return selected, found
}

func IsTicketEligibleForDay(ticket jira.Ticket, day time.Time, loc *time.Location) bool {
	targetDay := normalizeToLocationDay(day, loc)

	createdAt, ok := ticket.CreatedAt()
	if !ok {
		return false
	}

	if normalizeToLocationDay(createdAt, loc).After(targetDay) {
		return false
	}

	if resolutionDate, ok := ticket.ResolutionDate(); ok {
		if !normalizeToLocationDay(resolutionDate, loc).After(targetDay) {
			return false
		}
	}

	return true
}

func normalizeToDay(t time.Time) time.Time {
	return normalizeToLocationDay(t, t.Location())
}

func normalizeToLocationDay(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}

	localTime := t.In(loc)
	return time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, loc)
}

func parseIssueID(value string) (int, error) {
	var issueID int
	if _, err := fmt.Sscanf(value, "%d", &issueID); err != nil {
		return 0, fmt.Errorf("invalid ticket ID format: %s", value)
	}
	if issueID == 0 {
		return 0, fmt.Errorf("invalid ticket ID: got empty or zero value")
	}
	return issueID, nil
}
