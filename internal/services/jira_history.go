package services

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"ssk/internal/services/jira"
)

// JiraPeriod uses an inclusive lower bound and an exclusive upper bound.
type JiraPeriod struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

// ParseJiraPeriod captures a missing upper bound from the command's start time.
func ParseJiraPeriod(since, until string, started time.Time) (*JiraPeriod, error) {
	if since == "" {
		if until != "" {
			return nil, fmt.Errorf("an upper bound requires a lower bound")
		}
		return nil, nil
	}
	lower, err := time.Parse(time.RFC3339Nano, since)
	if err != nil {
		return nil, fmt.Errorf("lower bound must be RFC 3339 with an explicit timezone")
	}
	upper := started
	if until != "" {
		upper, err = time.Parse(time.RFC3339Nano, until)
		if err != nil {
			return nil, fmt.Errorf("upper bound must be RFC 3339 with an explicit timezone")
		}
	}
	period := &JiraPeriod{Since: lower.UTC(), Until: upper.UTC()}
	if err := period.validate(); err != nil {
		return nil, err
	}
	return period, nil
}

func (p *JiraPeriod) validate() error {
	if p != nil && !p.Since.Before(p.Until) {
		return fmt.Errorf("lower bound must precede upper bound")
	}
	return nil
}

func (p *JiraPeriod) contains(value time.Time) bool {
	return p == nil || (!value.Before(p.Since) && value.Before(p.Until))
}

type JiraExportChange struct {
	ChangelogID string          `json:"changelog_id"`
	ItemIndex   int             `json:"item_index"`
	CreatedAt   time.Time       `json:"created_at"`
	Author      *JiraExportUser `json:"author"`
	FieldID     *string         `json:"field_id"`
	Field       *string         `json:"field"`
	FromID      *string         `json:"from_id"`
	FromText    *string         `json:"from_text"`
	ToID        *string         `json:"to_id"`
	ToText      *string         `json:"to_text"`
}

type JiraExportChanges struct {
	Period                  *JiraPeriod        `json:"period"`
	RetrievalComplete       bool               `json:"retrieval_complete"`
	OutputTruncated         bool               `json:"output_truncated"`
	RetrievedChangelogCount int                `json:"retrieved_changelog_count"`
	MatchingCount           int                `json:"matching_count"`
	ReturnedCount           int                `json:"returned_count"`
	Ordering                string             `json:"ordering"`
	Items                   []JiraExportChange `json:"items"`
}

type jiraIssueHistory struct {
	Changes  *JiraExportChanges
	Comments *JiraExportComments
}

type jiraHistoryClient interface {
	BaseURL() string
	GetChangelogs(context.Context, string) ([]jira.Changelog, error)
	GetComments(context.Context, string) ([]jira.Comment, error)
}

func (o TicketInspectionOptions) validateHistory() error {
	if err := o.ChangesPeriod.validate(); err != nil {
		return fmt.Errorf("changes period: %w", err)
	}
	if err := o.CommentsPeriod.validate(); err != nil {
		return fmt.Errorf("comments period: %w", err)
	}
	if o.CommentsLimit != nil && *o.CommentsLimit < 0 {
		return fmt.Errorf("comments limit cannot be negative")
	}
	return nil
}

func (s *BriefingService) inspectionHistory(ctx context.Context, tickets []jira.Ticket, opts TicketInspectionOptions) (map[string]jiraIssueHistory, error) {
	if opts.ChangesPeriod == nil && opts.CommentsPeriod == nil && opts.CommentsLimit == nil {
		return nil, nil
	}
	client, ok := s.jiraClient.(jiraHistoryClient)
	if !ok {
		return nil, fmt.Errorf("jira client does not support history retrieval")
	}
	result := make(map[string]jiraIssueHistory, len(tickets))
	for _, ticket := range tickets {
		var history jiraIssueHistory
		if opts.ChangesPeriod != nil {
			s.debug("fetching changelogs for %s", ticket.Key)
			entries, err := client.GetChangelogs(ctx, ticket.Key)
			if err != nil {
				return nil, fmt.Errorf("changelogs for %s: %w", ticket.Key, err)
			}
			history.Changes, err = normalizeChangelogs(entries, opts.ChangesPeriod)
			if err != nil {
				return nil, fmt.Errorf("changelogs for %s: %w", ticket.Key, err)
			}
		}
		if opts.CommentsPeriod != nil || opts.CommentsLimit != nil {
			s.debug("fetching comments for %s", ticket.Key)
			comments, err := client.GetComments(ctx, ticket.Key)
			if err != nil {
				return nil, fmt.Errorf("comments for %s: %w", ticket.Key, err)
			}
			history.Comments, err = normalizeComments(comments, opts.CommentsPeriod, opts.CommentsLimit)
			if err != nil {
				return nil, fmt.Errorf("comments for %s: %w", ticket.Key, err)
			}
		}
		if history.Comments != nil {
			for index := range history.Comments.Items {
				comment := &history.Comments.Items[index]
				link := issueBrowserURL(strings.TrimRight(client.BaseURL(), "/"), ticket.Key) + "?focusedCommentId=" + url.QueryEscape(*comment.ID) + "#comment-" + url.PathEscape(*comment.ID)
				comment.URL = &link
			}
		}
		result[ticket.Key] = history
	}
	return result, nil
}

func normalizeChangelogs(entries []jira.Changelog, period *JiraPeriod) (*JiraExportChanges, error) {
	result := &JiraExportChanges{Period: period, RetrievalComplete: true, RetrievedChangelogCount: len(entries), Ordering: "timestamp ascending, changelog ID ascending, item index ascending", Items: make([]JiraExportChange, 0)}
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			return nil, fmt.Errorf("changelog has no source ID")
		}
		timestamp := exportTime(entry.Created)
		if timestamp == nil {
			return nil, fmt.Errorf("changelog %s has no valid timestamp; cannot establish period completeness", entry.ID)
		}
		if !period.contains(*timestamp) {
			continue
		}
		if entry.Items == nil {
			return nil, fmt.Errorf("changelog %s has unavailable field changes", entry.ID)
		}
		for index, item := range entry.Items {
			result.Items = append(result.Items, JiraExportChange{
				ChangelogID: entry.ID, ItemIndex: index, CreatedAt: *timestamp, Author: normalizeHistoryUser(entry.Author),
				FieldID: item.FieldID, Field: item.Field, FromID: item.From, FromText: item.FromString, ToID: item.To, ToText: item.ToString,
			})
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		left, right := result.Items[i], result.Items[j]
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		if left.ChangelogID != right.ChangelogID {
			return left.ChangelogID < right.ChangelogID
		}
		return left.ItemIndex < right.ItemIndex
	})
	result.MatchingCount, result.ReturnedCount = len(result.Items), len(result.Items)
	return result, nil
}

func normalizeComments(comments []jira.Comment, period *JiraPeriod, limit *int) (*JiraExportComments, error) {
	count := len(comments)
	ordering := "qualifying activity timestamp descending, comment ID ascending"
	result := &JiraExportComments{Source: "issue_comments", RetrievalComplete: true, Period: period, RetrievedCount: &count, Limit: limit, Ordering: &ordering, Items: make([]JiraExportComment, 0)}
	for _, comment := range comments {
		if strings.TrimSpace(comment.ID) == "" {
			return nil, fmt.Errorf("comment has no source ID")
		}
		created, updated := exportTime(comment.Created), exportTime(comment.Updated)
		if created == nil || updated == nil || updated.Before(*created) {
			return nil, fmt.Errorf("comment %s has missing or invalid dates; cannot establish activity order and period completeness", comment.ID)
		}
		createdInPeriod := period.contains(*created)
		editedInPeriod := updated.After(*created) && period.contains(*updated)
		if !createdInPeriod && !editedInPeriod {
			continue
		}
		activity := *created
		if editedInPeriod {
			activity = *updated
		}
		item := JiraExportComment{ID: nonemptyExportString(comment.ID), Author: normalizeHistoryUser(comment.Author), CreatedAt: created, UpdatedAt: updated, ActivityAt: &activity, Body: exportDoc(comment.Body)}
		if period != nil {
			item.CreatedInPeriod, item.EditedInPeriod = &createdInPeriod, &editedInPeriod
		}
		result.Items = append(result.Items, item)
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		left, right := result.Items[i], result.Items[j]
		if !left.ActivityAt.Equal(*right.ActivityAt) {
			return left.ActivityAt.After(*right.ActivityAt)
		}
		return *left.ID < *right.ID
	})
	matching := len(result.Items)
	result.MatchingCount = &matching
	if limit != nil && *limit > 0 && len(result.Items) > *limit {
		result.Items = result.Items[:*limit]
		result.OutputTruncated = true
	}
	result.ReturnedCount = len(result.Items)
	return result, nil
}

func normalizeHistoryUser(user *jira.User) *JiraExportUser {
	if user == nil {
		return nil
	}
	return &JiraExportUser{AccountID: nonemptyExportString(user.AccountID), DisplayName: nonemptyExportString(user.DisplayName)}
}

func applyInspectionHistory(result *JiraExport, histories map[string]jiraIssueHistory) {
	warnings := result.Warnings[:0]
	for _, warning := range result.Warnings {
		if warning.Code == "embedded_comments_incomplete" && warning.IssueKey != nil && histories[*warning.IssueKey].Comments != nil {
			continue
		}
		warnings = append(warnings, warning)
	}
	result.Warnings = warnings
	for index := range result.Issues {
		issue := &result.Issues[index]
		history := histories[issue.Key]
		issue.Changes = history.Changes
		if history.Comments != nil {
			issue.Comments = history.Comments
			if history.Comments.OutputTruncated {
				result.OutputTruncated = true
				result.Warnings = append(result.Warnings, JiraExportWarning{Code: "comments_output_limited", IssueKey: &issue.Key, Message: "Comment output was limited after complete retrieval and period filtering."})
			}
		}
	}
}

// renderInspectionHistory preserves legacy rendering when history options are absent.
func renderInspectionHistory(rootKey string, tickets []jira.Ticket, attachments map[string][]jiraAttachment, histories map[string]jiraIssueHistory) (string, error) {
	if histories == nil {
		return buildInspectionContext(rootKey, tickets, attachments)
	}
	rendered := make([]jira.Ticket, len(tickets))
	copy(rendered, tickets)
	for index, ticket := range tickets {
		if histories[ticket.Key].Comments == nil {
			continue
		}
		fields := make(map[string]interface{}, len(ticket.Fields))
		for key, value := range ticket.Fields {
			if key != "comment" {
				fields[key] = value
			}
		}
		rendered[index].Fields = fields
	}
	text, err := buildInspectionContext(rootKey, rendered, attachments)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	output.WriteString(text)
	output.WriteString("\n# Dated Jira Activity\n\nCurrent fields above reflect collection time. Comment bodies are their current versions.\n")
	for _, ticket := range tickets {
		history := histories[ticket.Key]
		fmt.Fprintf(&output, "\n## %s\n", ticket.Key)
		if history.Changes != nil {
			appendHistoryChanges(&output, history.Changes)
		}
		if history.Comments != nil {
			appendHistoryComments(&output, history.Comments)
		}
	}
	return output.String(), nil
}

func appendHistoryPeriod(output *strings.Builder, period *JiraPeriod) {
	if period == nil {
		output.WriteString("Period: all visible comments\n")
		return
	}
	fmt.Fprintf(output, "Period: %s <= timestamp < %s\n", period.Since.Format(time.RFC3339Nano), period.Until.Format(time.RFC3339Nano))
}

func appendHistoryChanges(output *strings.Builder, changes *JiraExportChanges) {
	output.WriteString("\n### Changes\n\n")
	appendHistoryPeriod(output, changes.Period)
	fmt.Fprintf(output, "Retrieval complete: true; returned field changes: %d\n", changes.ReturnedCount)
	for _, item := range changes.Items {
		fmt.Fprintf(output, "\n- %s [changelog %s, item %d] %s (%s): %s (%s) → %s (%s)\n", item.CreatedAt.Format(time.RFC3339Nano), item.ChangelogID, item.ItemIndex, historyText(item.Field), historyText(item.FieldID), historyText(item.FromText), historyText(item.FromID), historyText(item.ToText), historyText(item.ToID))
		if item.Author != nil {
			fmt.Fprintf(output, "  Author: %s\n", historyText(item.Author.DisplayName))
		}
	}
}

func appendHistoryComments(output *strings.Builder, comments *JiraExportComments) {
	output.WriteString("\n### Comments\n\n")
	appendHistoryPeriod(output, comments.Period)
	var body strings.Builder
	for _, comment := range comments.Items {
		fmt.Fprintf(&body, "\n- Comment %s; created %s; updated %s; qualifying activity %s\n", historyText(comment.ID), comment.CreatedAt.Format(time.RFC3339Nano), comment.UpdatedAt.Format(time.RFC3339Nano), comment.ActivityAt.Format(time.RFC3339Nano))
		if comment.CreatedInPeriod != nil {
			fmt.Fprintf(&body, "  Created in period: %t; edited in period: %t\n", *comment.CreatedInPeriod, *comment.EditedInPeriod)
		}
		if comment.URL != nil {
			fmt.Fprintf(&body, "  Link: %s\n", *comment.URL)
		}
		if comment.Author != nil {
			fmt.Fprintf(&body, "  Author: %s\n", historyText(comment.Author.DisplayName))
		}
		if comment.Body == nil {
			body.WriteString("  Body: unavailable\n")
		} else {
			fmt.Fprintf(&body, "\n%s\n", *comment.Body)
		}
	}
	// Retain the existing Markdown comment safety budget and disclose it separately.
	chars := []rune(body.String())
	bodyTruncated := len(chars) > maxBriefingCommentsChars
	fmt.Fprintf(output, "Retrieval complete: true; matching comments: %d; selected comments: %d; selection truncated: %t; body output truncated: %t\n", *comments.MatchingCount, comments.ReturnedCount, comments.OutputTruncated, bodyTruncated)
	if bodyTruncated {
		output.WriteString(string(chars[:maxBriefingCommentsChars]))
		output.WriteString("\n[comment output truncated]\n")
	} else {
		output.WriteString(body.String())
	}
}

func historyText(text *string) string {
	if text == nil {
		return "unknown"
	}
	return *text
}
