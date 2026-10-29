package services

import (
	"fmt"
	"strings"
	"time"
)

// RenderJiraActivityMarkdown renders the same normalized data and counts as JSON.
func RenderJiraActivityMarkdown(result *JiraExport) (string, error) {
	if result == nil || result.Kind != "jira.activity" || result.Activity == nil {
		return "", fmt.Errorf("expected a Jira activity export")
	}
	var output strings.Builder
	output.WriteString("# Jira Activity\n\n")
	fmt.Fprintf(&output, "Workspace: %s (%s)\nIssue count: %d\n\n", result.Workspace, result.SiteURL, result.IssueCount)
	appendHistoryPeriod(&output, result.Activity.Period)
	fmt.Fprintf(&output, "\nCollected: %s to %s\n", result.CollectionStartedAt.Format(time.RFC3339Nano), result.CollectionEndedAt.Format(time.RFC3339Nano))
	output.WriteString("Current fields reflect collection time, not a historical or transactional snapshot. Comment bodies are their current versions.\n\n")
	fmt.Fprintf(&output, "Selection JQL: `%s`\n\n", historyText(result.JQL))
	fmt.Fprintf(&output, "%s\n\n%s\n\nOrdering: %s\n", result.Activity.SelectionSemantics, result.Activity.CompletenessScope, result.Ordering)
	output.WriteString("\n## Summary\n\n### Current status counts\n\n")
	for _, status := range result.Activity.Summary.CurrentStatuses {
		name, id, category := "unknown", "unknown", "unknown"
		if status.Status != nil {
			name, id = historyText(status.Status.Name), historyText(status.Status.ID)
			if status.Status.Category != nil {
				category = historyText(status.Status.Category.Key)
			}
		}
		fmt.Fprintf(&output, "- %s (ID %s; native category %s): %d %s\n", name, id, category, status.Count, status.Unit)
	}
	if len(result.Activity.Summary.CurrentStatuses) == 0 {
		output.WriteString("- 0 issues\n")
	}
	output.WriteString("\n### Dated activity\n\n")
	summary := result.Activity.Summary
	for _, item := range []struct {
		label string
		value JiraActivityCount
	}{
		{"Created", summary.CreatedIssues}, {"With status changes", summary.IssuesWithStatusChanges}, {"Status transitions", summary.StatusTransitionEvents},
		{"New comments", summary.NewComments}, {"Edited comments", summary.EditedComments}, {"Unknown creation dates", summary.UnknownCreationDates}, {"Without detailed activity evidence", summary.IssuesWithoutDetailedEvidence},
	} {
		fmt.Fprintf(&output, "- %s: %d %s\n", item.label, item.value.Count, item.value.Unit)
	}
	output.WriteString("\nCounts use all matching sources before comment limits. A comment can be both new and edited in the period.\n")
	if result.OutputTruncated {
		output.WriteString("\nOutput contains requested excerpts; source retrieval is complete within the documented selection.\n")
	}
	for _, warning := range result.Warnings {
		key := ""
		if warning.IssueKey != nil {
			key = *warning.IssueKey + ": "
		}
		fmt.Fprintf(&output, "\nWarning [%s]: %s%s\n", warning.Code, key, warning.Message)
	}
	output.WriteString("\n## Issues\n")
	for _, issue := range result.Issues {
		appendActivityIssue(&output, issue)
	}
	return output.String(), nil
}

func appendActivityIssue(output *strings.Builder, issue JiraExportIssue) {
	fmt.Fprintf(output, "\n### %s: %s\n\n", issue.Key, historyText(issue.Summary))
	fmt.Fprintf(output, "URL: %s\nWorkspace: %s; Jira ID: %s\n", issue.URL, issue.Workspace, issue.ID)
	fmt.Fprintf(output, "Current update: %s; created: %s; resolved: %s\n", activityTimeText(issue.UpdatedAt), activityTimeText(issue.CreatedAt), activityTimeText(issue.ResolvedAt))
	if issue.Status == nil {
		output.WriteString("Current status: unknown\n")
	} else {
		fmt.Fprintf(output, "Current status: %s (ID %s)\n", historyText(issue.Status.Name), historyText(issue.Status.ID))
		appendActivityNamed(output, "Native status category", issue.Status.Category)
	}
	appendActivityNamed(output, "Project", issue.Project)
	appendActivityNamed(output, "Resolution", issue.Resolution)
	if issue.Assignee == nil {
		output.WriteString("Assignee: unknown\n")
	} else {
		fmt.Fprintf(output, "Assignee: %s (account ID %s)\n", historyText(issue.Assignee.DisplayName), historyText(issue.Assignee.AccountID))
	}
	appendActivityReference(output, "Parent", issue.Parent)
	if issue.Subtasks == nil {
		output.WriteString("Subtasks: unknown\n")
	} else {
		fmt.Fprintf(output, "Subtasks: %d\n", len(issue.Subtasks))
		for index := range issue.Subtasks {
			appendActivityReference(output, "Subtask", &issue.Subtasks[index])
		}
	}
	if issue.IssueLinks == nil {
		output.WriteString("Issue links: unknown\n")
	} else {
		fmt.Fprintf(output, "Issue links: %d\n", len(issue.IssueLinks))
		for _, link := range issue.IssueLinks {
			if link.InwardIssue != nil {
				appendActivityReference(output, "Inward link ("+historyText(link.InwardLabel)+")", link.InwardIssue)
			}
			if link.OutwardIssue != nil {
				appendActivityReference(output, "Outward link ("+historyText(link.OutwardLabel)+")", link.OutwardIssue)
			}
		}
	}
	appendActivityNamedList(output, "Components", issue.Components)
	appendActivityNamedList(output, "Fix versions", issue.FixVersions)
	if issue.Labels == nil {
		output.WriteString("Labels: unknown\n")
	} else {
		fmt.Fprintf(output, "Labels (%d): %s\n", len(issue.Labels), strings.Join(issue.Labels, ", "))
	}
	output.WriteString("\nDescription:\n\n")
	if issue.Description == nil {
		output.WriteString("Unavailable\n")
	} else {
		output.WriteString(truncateString(*issue.Description, maxBriefingDescriptionChars))
		output.WriteString("\n")
	}
	if issue.DescriptionTruncated {
		output.WriteString("Description output truncated by the requested line count.\n")
	}
	if issue.Activity != nil {
		fmt.Fprintf(output, "\nDetailed period activity evidence: %t\n", issue.Activity.HasDetailedEvidence)
		if issue.Activity.CreatedInPeriod == nil {
			output.WriteString("Created in period: unknown\n")
		} else {
			fmt.Fprintf(output, "Created in period: %t\n", *issue.Activity.CreatedInPeriod)
		}
		output.WriteString("\n#### Event timeline\n\n")
		for _, event := range issue.Activity.Events {
			fmt.Fprintf(output, "- %s %s [%s]\n", event.At.Format(time.RFC3339Nano), event.Kind, event.SourceID)
		}
		if len(issue.Activity.Events) == 0 {
			output.WriteString("- No dated evidence found in the visible sources for this period.\n")
		}
	}
	if issue.Changes != nil {
		appendHistoryChanges(output, issue.Changes)
	}
	if issue.Comments != nil {
		appendHistoryComments(output, issue.Comments)
	}
	if issue.Development != nil {
		output.WriteString("\nDevelopment links (best effort; completeness unavailable):\n")
		for _, items := range [][]string{issue.Development.Repositories, issue.Development.PullRequests, issue.Development.Branches, issue.Development.Commits} {
			appendDevelopmentItems(output, items)
		}
	}
}

func activityTimeText(value *time.Time) string {
	if value == nil {
		return "unknown"
	}
	return value.Format(time.RFC3339Nano)
}

func appendActivityNamed(output *strings.Builder, label string, value *JiraExportNamedValue) {
	if value == nil {
		fmt.Fprintf(output, "%s: unknown\n", label)
		return
	}
	fmt.Fprintf(output, "%s: %s (ID %s; key %s)\n", label, historyText(value.Name), historyText(value.ID), historyText(value.Key))
}

func appendActivityNamedList(output *strings.Builder, label string, values []JiraExportNamedValue) {
	if values == nil {
		fmt.Fprintf(output, "%s: unknown\n", label)
		return
	}
	fmt.Fprintf(output, "%s: %d\n", label, len(values))
	for index := range values {
		appendActivityNamed(output, label, &values[index])
	}
}

func appendActivityReference(output *strings.Builder, label string, value *JiraExportReference) {
	if value == nil {
		fmt.Fprintf(output, "%s: unknown\n", label)
		return
	}
	fmt.Fprintf(output, "%s: %s (workspace %s; ID %s; URL %s)\n", label, historyText(value.Key), value.Workspace, historyText(value.ID), historyText(value.URL))
}
