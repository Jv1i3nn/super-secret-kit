package services

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ssk/internal/services/jira"
)

// JiraExport is the versioned, allowlisted contract for machine consumers.
// RetrievalComplete describes issue selection, not comment/history completeness.
type JiraExport struct {
	SchemaVersion       int                   `json:"schema_version"`
	Kind                string                `json:"kind"`
	Workspace           string                `json:"workspace"`
	SiteURL             string                `json:"site_url"`
	CollectionStartedAt time.Time             `json:"collection_started_at"`
	CollectionEndedAt   time.Time             `json:"collection_ended_at"`
	JQL                 *string               `json:"jql"`
	IssueCount          int                   `json:"issue_count"`
	RetrievalComplete   bool                  `json:"retrieval_complete"`
	OutputTruncated     bool                  `json:"output_truncated"`
	Ordering            string                `json:"ordering"`
	Inspection          *JiraExportInspection `json:"inspection"`
	Warnings            []JiraExportWarning   `json:"warnings"`
	Issues              []JiraExportIssue     `json:"issues"`
}

type JiraExportInspection struct {
	RootKey            string `json:"root_key"`
	Depth              int    `json:"depth"`
	MaxRelated         int    `json:"max_related"`
	RelatedOmitted     bool   `json:"related_omitted"`
	IncludeDevelopment bool   `json:"include_development"`
	DescriptionLines   int    `json:"description_lines"`
	CommentChars       int    `json:"comment_chars"`
}

type JiraExportWarning struct {
	Code     string  `json:"code"`
	IssueKey *string `json:"issue_key"`
	Message  string  `json:"message"`
}

type JiraExportNamedValue struct {
	ID   *string `json:"id"`
	Key  *string `json:"key"`
	Name *string `json:"name"`
}

type JiraExportStatus struct {
	ID       *string               `json:"id"`
	Name     *string               `json:"name"`
	Category *JiraExportNamedValue `json:"category"`
}

type JiraExportUser struct {
	AccountID   *string `json:"account_id"`
	DisplayName *string `json:"display_name"`
}

type JiraExportReference struct {
	Workspace string  `json:"workspace"`
	ID        *string `json:"id"`
	Key       *string `json:"key"`
	URL       *string `json:"url"`
	Summary   *string `json:"summary"`
}

type JiraExportLink struct {
	ID           *string               `json:"id"`
	Type         *JiraExportNamedValue `json:"type"`
	InwardLabel  *string               `json:"inward_label"`
	OutwardLabel *string               `json:"outward_label"`
	InwardIssue  *JiraExportReference  `json:"inward_issue"`
	OutwardIssue *JiraExportReference  `json:"outward_issue"`
}

type JiraExportComment struct {
	ID              *string         `json:"id"`
	URL             *string         `json:"url"`
	Author          *JiraExportUser `json:"author"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	Body            *string         `json:"body"`
	BodyTruncated   bool            `json:"body_truncated"`
	ActivityAt      *time.Time      `json:"activity_at"`
	CreatedInPeriod *bool           `json:"created_in_period"`
	EditedInPeriod  *bool           `json:"edited_in_period"`
}

type JiraExportComments struct {
	Source              string              `json:"source"`
	Period              *JiraPeriod         `json:"period"`
	RetrievedCount      *int                `json:"retrieved_count"`
	MatchingCount       *int                `json:"matching_count"`
	Limit               *int                `json:"limit"`
	Ordering            *string             `json:"ordering"`
	RetrievalComplete   bool                `json:"retrieval_complete"`
	OutputTruncated     bool                `json:"output_truncated"`
	SelectionTruncated  bool                `json:"selection_truncated"`
	BodyOutputTruncated bool                `json:"body_output_truncated"`
	ReturnedCount       int                 `json:"returned_count"`
	Items               []JiraExportComment `json:"items"`
}

type JiraExportAttachment struct {
	ID            string  `json:"id"`
	Filename      string  `json:"filename"`
	LocalPath     *string `json:"local_path"`
	DownloadError *string `json:"download_error"`
}

type JiraExportIssue struct {
	Workspace            string                 `json:"workspace"`
	ID                   string                 `json:"id"`
	Key                  string                 `json:"key"`
	URL                  string                 `json:"url"`
	Summary              *string                `json:"summary"`
	Description          *string                `json:"description"`
	DescriptionTruncated bool                   `json:"description_truncated"`
	Project              *JiraExportNamedValue  `json:"project"`
	Status               *JiraExportStatus      `json:"status"`
	Resolution           *JiraExportNamedValue  `json:"resolution"`
	CreatedAt            *time.Time             `json:"created_at"`
	UpdatedAt            *time.Time             `json:"updated_at"`
	ResolvedAt           *time.Time             `json:"resolved_at"`
	Assignee             *JiraExportUser        `json:"assignee"`
	Parent               *JiraExportReference   `json:"parent"`
	Subtasks             []JiraExportReference  `json:"subtasks"`
	IssueLinks           []JiraExportLink       `json:"issue_links"`
	Components           []JiraExportNamedValue `json:"components"`
	Labels               []string               `json:"labels"`
	FixVersions          []JiraExportNamedValue `json:"fix_versions"`
	Comments             *JiraExportComments    `json:"comments"`
	Changes              *JiraExportChanges     `json:"changes"`
	Attachments          []JiraExportAttachment `json:"attachments"`
}

// BuildJiraExport normalizes current issue fields without copying arbitrary API data.
// A nil slice means unavailable; an empty slice means known empty.
func BuildJiraExport(kind, workspace, siteURL string, jql *string, started time.Time, tickets []jira.Ticket) (*JiraExport, error) {
	parsed, err := url.Parse(siteURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid Jira export site URL")
	}
	siteURL = strings.TrimRight(siteURL, "/")
	result := &JiraExport{
		SchemaVersion: 1, Kind: kind, Workspace: workspace, SiteURL: siteURL,
		CollectionStartedAt: started.UTC(), JQL: jql, RetrievalComplete: true,
		Ordering: "search order, first occurrence of each issue ID",
		Warnings: make([]JiraExportWarning, 0), Issues: make([]JiraExportIssue, 0, len(tickets)),
	}
	seen := make(map[string]bool)
	for _, ticket := range tickets {
		if ticket.ID == "" || ticket.Key == "" {
			return nil, fmt.Errorf("cannot export a Jira issue without ID and key")
		}
		if seen[ticket.ID] {
			continue
		}
		seen[ticket.ID] = true
		issue := normalizeJiraIssue(workspace, siteURL, ticket)
		result.Issues = append(result.Issues, issue)
		if issue.Comments != nil {
			result.Warnings = append(result.Warnings, JiraExportWarning{Code: "embedded_comments_incomplete", IssueKey: &issue.Key, Message: "Embedded comments are not a complete retrieval of issue discussion."})
		}
		for _, field := range []string{"created", "updated", "resolutiondate"} {
			if text := exportString(ticket.Fields[field]); text != nil && exportTime(*text) == nil {
				result.Warnings = append(result.Warnings, JiraExportWarning{Code: "invalid_timestamp", IssueKey: &issue.Key, Message: "Unavailable normalized timestamp: " + field})
			}
		}
	}
	result.IssueCount = len(result.Issues)
	result.CollectionEndedAt = time.Now().UTC()
	return result, nil
}

func normalizeJiraIssue(workspace, siteURL string, ticket jira.Ticket) JiraExportIssue {
	fields := ticket.Fields
	issue := JiraExportIssue{
		Workspace: workspace, ID: ticket.ID, Key: ticket.Key, URL: issueBrowserURL(siteURL, ticket.Key),
		Summary: exportString(fields["summary"]), Description: exportDoc(fields["description"]),
		Project: exportNamed(fields["project"]), Resolution: exportNamed(fields["resolution"]),
		CreatedAt: exportTimeValue(fields["created"]), UpdatedAt: exportTimeValue(fields["updated"]), ResolvedAt: exportTimeValue(fields["resolutiondate"]),
		Assignee: exportUser(fields["assignee"]), Parent: exportReference(workspace, siteURL, fields["parent"]),
		Components: exportNamedList(fields["components"]), FixVersions: exportNamedList(fields["fixVersions"]),
	}
	if status, ok := fields["status"].(map[string]interface{}); ok {
		issue.Status = &JiraExportStatus{ID: exportString(status["id"]), Name: exportString(status["name"]), Category: exportNamed(status["statusCategory"])}
	}
	if values, ok := fields["labels"].([]interface{}); ok {
		issue.Labels = make([]string, 0, len(values))
		for _, value := range values {
			if label, ok := value.(string); ok {
				issue.Labels = append(issue.Labels, label)
			}
		}
	}
	if values, ok := fields["subtasks"].([]interface{}); ok {
		issue.Subtasks = make([]JiraExportReference, 0, len(values))
		for _, value := range values {
			if ref := exportReference(workspace, siteURL, value); ref != nil {
				issue.Subtasks = append(issue.Subtasks, *ref)
			}
		}
	}
	if values, ok := fields["issuelinks"].([]interface{}); ok {
		issue.IssueLinks = make([]JiraExportLink, 0, len(values))
		for _, value := range values {
			link, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			linkType, _ := link["type"].(map[string]interface{})
			issue.IssueLinks = append(issue.IssueLinks, JiraExportLink{
				ID: exportString(link["id"]), Type: exportNamed(linkType),
				InwardLabel: exportString(linkType["inward"]), OutwardLabel: exportString(linkType["outward"]),
				InwardIssue: exportReference(workspace, siteURL, link["inwardIssue"]), OutwardIssue: exportReference(workspace, siteURL, link["outwardIssue"]),
			})
		}
	}
	if comments, ok := fields["comment"].(map[string]interface{}); ok {
		issue.Comments = &JiraExportComments{Source: "embedded"}
		if values, ok := comments["comments"].([]interface{}); ok {
			issue.Comments.Items = make([]JiraExportComment, 0, len(values))
			for _, value := range values {
				comment, ok := value.(map[string]interface{})
				if !ok {
					continue
				}
				issue.Comments.Items = append(issue.Comments.Items, JiraExportComment{
					ID: exportString(comment["id"]), Author: exportUser(comment["author"]),
					CreatedAt: exportTimeValue(comment["created"]), UpdatedAt: exportTimeValue(comment["updated"]), Body: exportDoc(comment["body"]),
				})
			}
		}
		issue.Comments.ReturnedCount = len(issue.Comments.Items)
	}
	return issue
}

// GenerateTicketInspectionExport shares traversal and attachment behavior with Markdown inspection.
func (s *BriefingService) GenerateTicketInspectionExport(ctx context.Context, workspace, siteURL, issueKey string, opts TicketInspectionOptions, directory string, started time.Time) (*JiraExport, error) {
	s = s.withJiraContext(ctx)
	rootKey, tickets, err := s.inspectionTickets(ctx, issueKey, opts)
	if err != nil {
		return nil, err
	}
	result, err := BuildJiraExport("jira.inspect", workspace, siteURL, nil, started, tickets)
	if err != nil {
		return nil, err
	}
	histories, err := s.inspectionHistory(ctx, tickets, opts)
	if err != nil {
		return nil, err
	}
	applyInspectionHistory(result, histories)
	applyInspectionCompact(result, opts)
	result.Ordering = "root first, then breadth-first related issues"
	result.Inspection = &JiraExportInspection{RootKey: rootKey, Depth: opts.Depth, MaxRelated: opts.MaxRelated, IncludeDevelopment: opts.IncludeDevelopment == nil || *opts.IncludeDevelopment, DescriptionLines: opts.DescriptionLines, CommentChars: opts.CommentChars}
	included := make(map[string]bool, len(tickets))
	for _, ticket := range tickets {
		included[ticket.Key] = true
	}
	for _, ticket := range tickets {
		for _, key := range ExtractRelatedTicketKeys(ticket) {
			if !included[key] {
				result.Inspection.RelatedOmitted = true
			}
		}
	}
	if result.Inspection.RelatedOmitted {
		result.Warnings = append(result.Warnings, JiraExportWarning{Code: "related_selection_limited", Message: "Related issues outside the requested traversal bounds were not fetched; relationship metadata is retained."})
	}
	if directory != "" {
		downloader, ok := s.jiraClient.(AttachmentDownloader)
		if !ok {
			return nil, fmt.Errorf("jira client does not support attachment downloads")
		}
		attachments := downloadJiraAttachments(ctx, tickets, directory, downloader)
		for i := range result.Issues {
			issue := &result.Issues[i]
			issue.Attachments = make([]JiraExportAttachment, 0)
			for _, attachment := range attachments[issue.Key] {
				issue.Attachments = append(issue.Attachments, JiraExportAttachment{ID: attachment.ID, Filename: attachment.Filename, LocalPath: nonemptyExportString(attachment.LocalPath), DownloadError: nonemptyExportString(attachment.DownloadErr)})
				if attachment.DownloadErr != "" {
					result.Warnings = append(result.Warnings, JiraExportWarning{Code: "attachment_download_failed", IssueKey: &issue.Key, Message: "An attachment could not be downloaded; see attachments for details."})
				}
			}
		}
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	result.CollectionEndedAt = time.Now().UTC()
	return result, nil
}

func exportString(value interface{}) *string {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

func exportIdentifier(value interface{}) *string {
	if number, ok := value.(float64); ok {
		text := strconv.FormatFloat(number, 'f', -1, 64)
		return &text
	}
	return exportString(value)
}

func nonemptyExportString(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func exportDoc(value interface{}) *string {
	if doc, ok := value.(map[string]interface{}); ok {
		text := extractAtlassianDocText(doc)
		return &text
	}
	return exportString(value)
}

func exportNamed(value interface{}) *JiraExportNamedValue {
	object, ok := value.(map[string]interface{})
	if !ok || object == nil {
		return nil
	}
	return &JiraExportNamedValue{ID: exportIdentifier(object["id"]), Key: exportString(object["key"]), Name: exportString(object["name"])}
}

func exportNamedList(value interface{}) []JiraExportNamedValue {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]JiraExportNamedValue, 0, len(values))
	for _, item := range values {
		if named := exportNamed(item); named != nil {
			result = append(result, *named)
		}
	}
	return result
}

func exportUser(value interface{}) *JiraExportUser {
	user, ok := value.(map[string]interface{})
	if !ok || user == nil {
		return nil
	}
	return &JiraExportUser{AccountID: exportString(user["accountId"]), DisplayName: exportString(user["displayName"])}
}

func exportReference(workspace, siteURL string, value interface{}) *JiraExportReference {
	issue, ok := value.(map[string]interface{})
	if !ok || issue == nil {
		return nil
	}
	fields, _ := issue["fields"].(map[string]interface{})
	ref := &JiraExportReference{Workspace: workspace, ID: exportString(issue["id"]), Key: exportString(issue["key"]), Summary: exportString(fields["summary"])}
	if ref.Key != nil && *ref.Key != "" {
		browserURL := issueBrowserURL(siteURL, *ref.Key)
		ref.URL = &browserURL
	}
	return ref
}

func issueBrowserURL(siteURL, key string) string {
	return siteURL + "/browse/" + url.PathEscape(key)
}

func exportTimeValue(value interface{}) *time.Time {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return exportTime(text)
}

func exportTime(text string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}
