package jira

import (
	"fmt"
	"time"
)

// Ticket represents a Jira issue
type Ticket struct {
	ID     string                 `json:"id"`
	Key    string                 `json:"key"`
	Fields map[string]interface{} `json:"fields"`
}

const (
	FieldDevelopmentInfo = "_developmentInfo"
)

// TicketSearchResponse represents the response from Jira search API
type TicketSearchResponse struct {
	Issues     []Ticket `json:"issues"`
	Total      int      `json:"total"`
	MaxResults int      `json:"maxResults"`
}

// User represents a Jira user
type User struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

type Comment struct {
	ID      string                 `json:"id"`
	Self    string                 `json:"self"`
	Created string                 `json:"created"`
	Body    map[string]interface{} `json:"body"`
}

func (t Ticket) Summary() string {
	if t.Fields == nil {
		return ""
	}

	if summary, ok := t.Fields["summary"].(string); ok {
		return summary
	}

	return ""
}

func (t Ticket) Status() string {
	if t.Fields == nil {
		return ""
	}
	if status, ok := t.Fields["status"].(map[string]interface{}); ok {
		if name, ok := status["name"].(string); ok {
			return name
		}
	}
	return ""
}

func (t Ticket) CreatedAt() (time.Time, bool) {
	return jiraTimeField(t.Fields, "created")
}

func (t Ticket) UpdatedAt() (time.Time, bool) {
	return jiraTimeField(t.Fields, "updated")
}

func (t Ticket) ResolutionDate() (time.Time, bool) {
	return jiraTimeField(t.Fields, "resolutiondate")
}

func jiraTimeField(fields map[string]interface{}, key string) (time.Time, bool) {
	if fields == nil {
		return time.Time{}, false
	}

	value, ok := fields[key].(string)
	if !ok || value == "" {
		return time.Time{}, false
	}

	t, err := parseJiraTime(value)
	if err != nil {
		return time.Time{}, false
	}

	return t, true
}

func parseJiraTime(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05.000-0700",
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported Jira time format: %s", value)
}
