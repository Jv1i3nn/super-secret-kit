package services

import (
	"fmt"
	"strings"

	"ssk/internal/services/jira"
)

func compactDescription(text *string, lines int) (*string, bool) {
	if text == nil || lines <= 0 {
		return text, false
	}
	parts := strings.Split(*text, "\n")
	if len(parts) <= lines {
		return text, false
	}
	excerpt := strings.Join(parts[:lines], "\n")
	return &excerpt, true
}

func compactComments(comments *JiraExportComments, chars int) {
	if comments == nil || chars <= 0 {
		return
	}
	for index := range comments.Items {
		item := &comments.Items[index]
		if item.Body == nil {
			continue
		}
		runes := []rune(*item.Body)
		if len(runes) > chars {
			text := string(runes[:chars])
			item.Body = &text
			item.BodyTruncated = true
			comments.BodyOutputTruncated = true
			comments.OutputTruncated = true
		}
	}
}

func applyInspectionCompact(result *JiraExport, opts TicketInspectionOptions) {
	for index := range result.Issues {
		issue := &result.Issues[index]
		issue.Description, issue.DescriptionTruncated = compactDescription(issue.Description, opts.DescriptionLines)
		if issue.DescriptionTruncated {
			result.OutputTruncated = true
			result.Warnings = append(result.Warnings, JiraExportWarning{Code: "description_output_limited", IssueKey: &issue.Key, Message: "Description output was limited by the requested line count."})
		}
	}
}

func compactMarkdownTickets(tickets []jira.Ticket, opts TicketInspectionOptions) ([]jira.Ticket, string) {
	if opts.DescriptionLines <= 0 {
		return tickets, ""
	}
	rendered := make([]jira.Ticket, len(tickets))
	copy(rendered, tickets)
	var notices strings.Builder
	for index, ticket := range tickets {
		text, truncated := compactDescription(exportDoc(ticket.Fields["description"]), opts.DescriptionLines)
		if !truncated {
			continue
		}
		fields := make(map[string]interface{}, len(ticket.Fields))
		for key, value := range ticket.Fields {
			fields[key] = value
		}
		fields["description"] = *text
		rendered[index].Fields = fields
		fmt.Fprintf(&notices, "\nDescription output truncated for %s: retained first %d lines.\n", ticket.Key, opts.DescriptionLines)
	}
	return rendered, notices.String()
}
