package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"ssk/internal/services/jira"
)

type JiraActivityOptions struct {
	Period             *JiraPeriod
	Workers            int
	DescriptionLines   int
	CommentsLimit      int
	CommentChars       int
	IncludeDevelopment bool
}

func (o JiraActivityOptions) validate() error {
	if o.Period == nil {
		return fmt.Errorf("activity requires a reporting period")
	}
	if err := o.Period.validate(); err != nil {
		return err
	}
	if o.Workers < 1 || o.Workers > 32 {
		return fmt.Errorf("workers must be between 1 and 32")
	}
	if o.DescriptionLines < 0 || o.CommentChars < 0 || o.CommentsLimit < 0 {
		return fmt.Errorf("activity output limits cannot be negative")
	}
	return nil
}

type JiraActivityCount struct {
	Count int    `json:"count"`
	Unit  string `json:"unit"`
}

type JiraActivityStatusCount struct {
	Status *JiraExportStatus `json:"status"`
	Count  int               `json:"count"`
	Unit   string            `json:"unit"`
}

type JiraActivitySummary struct {
	CurrentStatuses               []JiraActivityStatusCount `json:"current_status_counts"`
	CreatedIssues                 JiraActivityCount         `json:"created_issues"`
	IssuesWithStatusChanges       JiraActivityCount         `json:"issues_with_status_changes"`
	StatusTransitionEvents        JiraActivityCount         `json:"status_transition_events"`
	NewComments                   JiraActivityCount         `json:"new_comments"`
	EditedComments                JiraActivityCount         `json:"edited_comments"`
	UnknownCreationDates          JiraActivityCount         `json:"unknown_creation_dates"`
	IssuesWithoutDetailedEvidence JiraActivityCount         `json:"issues_without_detailed_evidence"`
}

type JiraExportActivity struct {
	Period                    *JiraPeriod         `json:"period"`
	CandidateLowerBoundMillis int64               `json:"candidate_lower_bound_millis"`
	DateEncoding              string              `json:"date_encoding"`
	SelectionSemantics        string              `json:"selection_semantics"`
	CompletenessScope         string              `json:"completeness_scope"`
	CurrentFields             string              `json:"current_fields"`
	EventOrdering             string              `json:"event_ordering"`
	Workers                   int                 `json:"workers"`
	DescriptionLines          int                 `json:"description_lines"`
	CommentsLimit             int                 `json:"comments_limit"`
	CommentChars              int                 `json:"comment_chars"`
	IncludeDevelopment        bool                `json:"include_development"`
	Summary                   JiraActivitySummary `json:"summary"`
}

type JiraActivityEvent struct {
	SourceID    string    `json:"source_id"`
	Kind        string    `json:"kind"`
	At          time.Time `json:"at"`
	ChangelogID *string   `json:"changelog_id"`
	ItemIndex   *int      `json:"item_index"`
	CommentID   *string   `json:"comment_id"`
}

type JiraIssueActivity struct {
	CreatedInPeriod        *bool               `json:"created_in_period"`
	HasDetailedEvidence    bool                `json:"has_detailed_evidence"`
	StatusTransitionEvents int                 `json:"status_transition_events"`
	NewComments            int                 `json:"new_comments"`
	EditedComments         int                 `json:"edited_comments"`
	Events                 []JiraActivityEvent `json:"events"`
}

// JiraActivityService orchestrates one workspace; workers only return private results.
type JiraActivityService struct {
	client  *jira.Client
	verbose bool
	output  io.Writer
}

func NewJiraActivityService(client *jira.Client) *JiraActivityService {
	return &JiraActivityService{client: client, output: io.Discard}
}

func (s *JiraActivityService) SetVerbose(enabled bool, output io.Writer) {
	s.verbose = enabled
	if output == nil {
		output = io.Discard
	}
	s.output = output
}

// ActivityCandidateJQL uses absolute milliseconds, floored to preserve fractional bounds.
// An upper update bound would lose issues edited after a qualifying historical event.
func ActivityCandidateJQL(period *JiraPeriod) (string, error) {
	if period == nil {
		return "", fmt.Errorf("activity requires a reporting period")
	}
	if err := period.validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("updated >= %d ORDER BY updated DESC, key ASC", period.Since.UnixMilli()), nil
}

func (s *JiraActivityService) Generate(ctx context.Context, workspace string, opts JiraActivityOptions, started time.Time) (*JiraExport, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client := s.client.WithContext(ctx)
	jql, err := ActivityCandidateJQL(opts.Period)
	if err != nil {
		return nil, err
	}
	s.debug("searching activity candidates")
	candidates, err := client.SearchActivityCandidates(jql)
	if err != nil {
		return nil, err
	}
	s.debug("retrieved %d distinct candidate issues", len(candidates))
	result, err := BuildJiraExport("jira.activity", workspace, client.BaseURL(), &jql, started, nil)
	if err != nil {
		return nil, err
	}
	result.Ordering = "current update timestamp descending (unknown last), key ascending, issue ID ascending"
	result.Activity = &JiraExportActivity{
		Period: opts.Period, CandidateLowerBoundMillis: opts.Period.Since.UnixMilli(), DateEncoding: "unquoted Unix epoch milliseconds; lower bound rounded down to a millisecond",
		SelectionSemantics: "All visible issues returned by the lower-bound updated search are retained, with no upper updated bound. Creation, changelog and comment events are filtered by since <= timestamp < until.",
		CompletenessScope:  "Complete candidate pages and visible changelog/comment APIs within operational page limits; deleted or restricted data and intermediate comment edits cannot be recovered.",
		CurrentFields:      "collection time; not a historical or transactional snapshot",
		EventOrdering:      "timestamp ascending, source ID ascending; comment details select newest qualifying activity before output limits",
		Workers:            opts.Workers, DescriptionLines: opts.DescriptionLines, CommentsLimit: opts.CommentsLimit, CommentChars: opts.CommentChars, IncludeDevelopment: opts.IncludeDevelopment,
	}
	results, err := s.collect(ctx, client, workspace, candidates, opts, started)
	if err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool { return activityIssueLess(results[i].Issues[0], results[j].Issues[0]) })
	for _, item := range results {
		result.Issues = append(result.Issues, item.Issues[0])
		result.Warnings = append(result.Warnings, item.Warnings...)
		result.OutputTruncated = result.OutputTruncated || item.OutputTruncated
	}
	result.IssueCount = len(result.Issues)
	result.Activity.Summary = summarizeActivity(result.Issues)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	result.CollectionEndedAt = time.Now().UTC()
	s.debug("collected %d issues", result.IssueCount)
	return result, nil
}

func (s *JiraActivityService) collect(ctx context.Context, client *jira.Client, workspace string, candidates []jira.Ticket, opts JiraActivityOptions, started time.Time) ([]*JiraExport, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client = client.WithContext(ctx)
	results := make([]*JiraExport, len(candidates))
	jobs := make(chan int)
	var workers sync.WaitGroup
	var once sync.Once
	var failure error
	for worker := 0; worker < opts.Workers && worker < len(candidates); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				item, err := collectActivityIssue(ctx, client, workspace, candidates[index], opts, started)
				if err != nil {
					once.Do(func() { failure = err; cancel() })
					return
				}
				results[index] = item
			}
		}()
	}
feed:
	for index := range candidates {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- index:
		}
	}
	close(jobs)
	workers.Wait()
	if failure != nil {
		return nil, failure
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	return results, nil
}

func collectActivityIssue(ctx context.Context, client *jira.Client, workspace string, candidate jira.Ticket, opts JiraActivityOptions, started time.Time) (*JiraExport, error) {
	// Fetch by immutable Jira ID so a key rename during collection cannot change identity.
	ticket, err := client.GetInspectionTicketByKey(candidate.ID, opts.IncludeDevelopment)
	if err != nil {
		return nil, fmt.Errorf("issue %s: %w", candidate.ID, err)
	}
	if ticket.ID != candidate.ID {
		return nil, fmt.Errorf("issue %s changed identity during collection", candidate.ID)
	}
	result, err := BuildJiraExport("jira.activity", workspace, client.BaseURL(), nil, started, []jira.Ticket{*ticket})
	if err != nil {
		return nil, err
	}
	service := NewBriefingService(client)
	fullOptions := TicketInspectionOptions{ChangesPeriod: opts.Period, CommentsPeriod: opts.Period}
	histories, err := service.inspectionHistory(ctx, []jira.Ticket{*ticket}, fullOptions)
	if err != nil {
		return nil, err
	}
	history := histories[ticket.Key]
	issue := &result.Issues[0]
	issue.Activity = activityEvidence(*issue, history, opts.Period)
	limitExportComments(history.Comments, &opts.CommentsLimit)
	compactComments(history.Comments, opts.CommentChars)
	applyInspectionHistory(result, histories)
	applyInspectionCompact(result, TicketInspectionOptions{DescriptionLines: opts.DescriptionLines})
	if opts.IncludeDevelopment {
		issue.Development = normalizeActivityDevelopment(ticket.Fields[jira.FieldDevelopmentInfo])
	}
	if !issue.Activity.HasDetailedEvidence {
		result.Warnings = append(result.Warnings, JiraExportWarning{Code: "no_dated_activity_evidence", IssueKey: &issue.Key, Message: "Candidate selection matched, but the visible sources expose no dated activity evidence in the requested period."})
	}
	return result, nil
}

func activityEvidence(issue JiraExportIssue, history jiraIssueHistory, period *JiraPeriod) *JiraIssueActivity {
	evidence := &JiraIssueActivity{Events: make([]JiraActivityEvent, 0)}
	if issue.CreatedAt != nil {
		created := period.contains(*issue.CreatedAt)
		evidence.CreatedInPeriod = &created
		if created {
			evidence.Events = append(evidence.Events, JiraActivityEvent{SourceID: issue.ID + "/created", Kind: "issue.created", At: *issue.CreatedAt})
		}
	}
	for _, change := range history.Changes.Items {
		index := change.ItemIndex
		id := change.ChangelogID
		evidence.Events = append(evidence.Events, JiraActivityEvent{SourceID: fmt.Sprintf("%s/changelog/%s/%d", issue.ID, id, index), Kind: "issue.change", At: change.CreatedAt, ChangelogID: &id, ItemIndex: &index})
		if statusChange(change) {
			evidence.StatusTransitionEvents++
		}
	}
	for _, comment := range history.Comments.Items {
		id := *comment.ID
		if *comment.CreatedInPeriod {
			evidence.NewComments++
			evidence.Events = append(evidence.Events, JiraActivityEvent{SourceID: issue.ID + "/comment/" + id + "/created", Kind: "comment.created", At: *comment.CreatedAt, CommentID: &id})
		}
		if *comment.EditedInPeriod {
			evidence.EditedComments++
			evidence.Events = append(evidence.Events, JiraActivityEvent{SourceID: issue.ID + "/comment/" + id + "/edited", Kind: "comment.edited", At: *comment.UpdatedAt, CommentID: &id})
		}
	}
	sort.Slice(evidence.Events, func(i, j int) bool {
		left, right := evidence.Events[i], evidence.Events[j]
		if !left.At.Equal(right.At) {
			return left.At.Before(right.At)
		}
		return left.SourceID < right.SourceID
	})
	evidence.HasDetailedEvidence = len(evidence.Events) > 0
	return evidence
}

func statusChange(change JiraExportChange) bool {
	if change.FieldID != nil {
		return *change.FieldID == "status"
	}
	return change.Field != nil && strings.EqualFold(*change.Field, "status")
}

func activityIssueLess(left, right JiraExportIssue) bool {
	if left.UpdatedAt != nil && right.UpdatedAt != nil && !left.UpdatedAt.Equal(*right.UpdatedAt) {
		return left.UpdatedAt.After(*right.UpdatedAt)
	}
	if (left.UpdatedAt == nil) != (right.UpdatedAt == nil) {
		return left.UpdatedAt != nil
	}
	if left.Key != right.Key {
		return left.Key < right.Key
	}
	return left.ID < right.ID
}

func summarizeActivity(issues []JiraExportIssue) JiraActivitySummary {
	summary := JiraActivitySummary{
		CurrentStatuses: make([]JiraActivityStatusCount, 0), CreatedIssues: JiraActivityCount{Unit: "issues"}, IssuesWithStatusChanges: JiraActivityCount{Unit: "issues"},
		StatusTransitionEvents: JiraActivityCount{Unit: "events"}, NewComments: JiraActivityCount{Unit: "comments"}, EditedComments: JiraActivityCount{Unit: "comments"},
		UnknownCreationDates: JiraActivityCount{Unit: "issues"}, IssuesWithoutDetailedEvidence: JiraActivityCount{Unit: "issues"},
	}
	statuses := make(map[string]JiraActivityStatusCount)
	for _, issue := range issues {
		activity := issue.Activity
		if activity.CreatedInPeriod == nil {
			summary.UnknownCreationDates.Count++
		} else if *activity.CreatedInPeriod {
			summary.CreatedIssues.Count++
		}
		if activity.StatusTransitionEvents > 0 {
			summary.IssuesWithStatusChanges.Count++
		}
		summary.StatusTransitionEvents.Count += activity.StatusTransitionEvents
		summary.NewComments.Count += activity.NewComments
		summary.EditedComments.Count += activity.EditedComments
		if !activity.HasDetailedEvidence {
			summary.IssuesWithoutDetailedEvidence.Count++
		}
		// Group exact observed native status values, including unknown status as null.
		encoded, _ := json.Marshal(issue.Status)
		key := string(encoded)
		count := statuses[key]
		count.Status, count.Unit, count.Count = issue.Status, "issues", count.Count+1
		statuses[key] = count
	}
	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		summary.CurrentStatuses = append(summary.CurrentStatuses, statuses[key])
	}
	return summary
}

func (s *JiraActivityService) debug(format string, args ...interface{}) {
	if s.verbose {
		_, _ = fmt.Fprintf(s.output, "activity: "+format+"\n", args...)
	}
}
