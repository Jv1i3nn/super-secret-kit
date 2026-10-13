package tempo

// Worklog represents a Tempo worklog entry
type Worklog struct {
	IssueID          int    `json:"issueId"`
	TimeSpentSeconds int    `json:"timeSpentSeconds"`
	BillableSeconds  int    `json:"billableSeconds"`
	StartDate        string `json:"startDate"`
	StartTime        string `json:"startTime"`
	AuthorAccountID  string `json:"authorAccountId"`
}

// WorklogResponse represents the response from Tempo worklog API
type WorklogResponse struct {
	Results []Worklog `json:"results"`
}

// Holiday represents a day off from the holiday calendar
type Holiday struct {
	Date        string `json:"date"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HolidayResponse represents the response from Tempo holiday API
type HolidayResponse struct {
	Results []Holiday `json:"results"`
}
