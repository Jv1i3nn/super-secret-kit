package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"ssk/internal/services/jira"
)

const (
	maxBriefingObjectFieldChars = 8000
	maxBriefingDescriptionChars = 20000
	maxBriefingCommentsChars    = 30000
	defaultBriefingMaxTickets   = 10
	defaultInspectionDepth      = 2
	defaultInspectionMaxRelated = 10
)

type TicketInspectionOptions struct {
	Depth      int
	MaxRelated int
}

type BriefingJiraClient interface {
	GetBriefingCandidateTickets() ([]jira.Ticket, error)
	GetBriefingTicketByKey(issueKey string) (*jira.Ticket, error)
}

type BriefingService struct {
	jiraClient BriefingJiraClient
	maxTickets int
	verbose    bool
	debugOut   io.Writer
}

func NewBriefingService(jiraClient BriefingJiraClient) *BriefingService {
	return &BriefingService{
		jiraClient: jiraClient,
		maxTickets: defaultBriefingMaxTickets,
		debugOut:   io.Discard,
	}
}

func (s *BriefingService) SetMaxTickets(maxTickets int) {
	s.maxTickets = maxTickets
}

func (s *BriefingService) SetVerbose(verbose bool, debugOut io.Writer) {
	s.verbose = verbose
	if debugOut == nil {
		debugOut = io.Discard
	}
	s.debugOut = debugOut
}

func (s *BriefingService) GenerateBriefing(ctx context.Context) (string, error) {
	_ = ctx

	s.debug("fetching Jira briefing candidates")
	candidates, err := s.jiraClient.GetBriefingCandidateTickets()
	if err != nil {
		return "", fmt.Errorf("failed to get briefing candidate tickets: %w", err)
	}
	s.debug("found %d Jira briefing candidate(s)", len(candidates))

	if len(candidates) == 0 {
		return "", errors.New("no assigned non-done Jira ticket updated in the last 30 days")
	}

	maxTickets := s.maxTickets
	if maxTickets <= 0 {
		return "", errors.New("maximum Jira ticket count must be greater than zero")
	}
	if len(candidates) > maxTickets {
		s.debug("limiting Jira briefing candidates from %d to %d", len(candidates), maxTickets)
		candidates = candidates[:maxTickets]
	}

	tickets := make([]jira.Ticket, 0, len(candidates))
	for _, candidate := range candidates {
		s.debug("fetching full Jira context for %s: %s", candidate.Key, candidate.Summary())
		ticket, ticketErr := s.jiraClient.GetBriefingTicketByKey(candidate.Key)
		if ticketErr != nil {
			return "", fmt.Errorf("failed to get Jira ticket %s: %w", candidate.Key, ticketErr)
		}
		tickets = append(tickets, *ticket)
	}

	context, err := BuildBriefingContext(tickets)
	if err != nil {
		return "", fmt.Errorf("failed to build briefing context: %w", err)
	}
	s.debug("built briefing context (%d characters)", len(context))

	return context, nil
}

func (s *BriefingService) GenerateTicketInspection(ctx context.Context, issueKey string) (string, error) {
	return s.GenerateTicketInspectionWithOptions(ctx, issueKey, TicketInspectionOptions{
		Depth:      defaultInspectionDepth,
		MaxRelated: defaultInspectionMaxRelated,
	})
}

func (s *BriefingService) GenerateTicketInspectionWithOptions(ctx context.Context, issueKey string, opts TicketInspectionOptions) (string, error) {
	rootKey, tickets, err := s.inspectionTickets(ctx, issueKey, opts)
	if err != nil {
		return "", err
	}

	context, err := BuildInspectionContext(rootKey, tickets)
	if err != nil {
		return "", fmt.Errorf("failed to build Jira inspection context: %w", err)
	}
	s.debug("built Jira inspection context (%d characters)", len(context))

	return context, nil
}

// GenerateTicketInspectionWithDownloads adds local attachment paths to the inspection output.
// Individual download failures are reported in the Markdown and do not fail inspection.
func (s *BriefingService) GenerateTicketInspectionWithDownloads(ctx context.Context, issueKey string, opts TicketInspectionOptions, directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", errors.New("attachment download directory cannot be empty")
	}
	rootKey, tickets, err := s.inspectionTickets(ctx, issueKey, opts)
	if err != nil {
		return "", err
	}
	downloader, ok := s.jiraClient.(AttachmentDownloader)
	if !ok {
		return "", errors.New("jira client does not support attachment downloads")
	}
	attachments := downloadJiraAttachments(ctx, tickets, directory, downloader)
	context, err := buildInspectionContext(rootKey, tickets, attachments)
	if err != nil {
		return "", fmt.Errorf("failed to build Jira inspection context: %w", err)
	}
	s.debug("built Jira inspection context with attachments (%d characters)", len(context))
	return context, nil
}

func (s *BriefingService) inspectionTickets(ctx context.Context, issueKey string, opts TicketInspectionOptions) (string, []jira.Ticket, error) {
	_ = ctx
	issueKey = strings.TrimSpace(issueKey)
	if issueKey == "" {
		return "", nil, errors.New("jira ticket key cannot be empty")
	}
	if opts.Depth < 0 {
		return "", nil, errors.New("jira inspection depth cannot be negative")
	}
	if opts.MaxRelated < 0 {
		return "", nil, errors.New("maximum related Jira ticket count cannot be negative")
	}

	s.debug("fetching full Jira context for root ticket %s", issueKey)
	rootTicket, err := s.jiraClient.GetBriefingTicketByKey(issueKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get Jira ticket %s: %w", issueKey, err)
	}

	tickets, err := s.fetchInspectionTickets(*rootTicket, opts)
	if err != nil {
		return "", nil, err
	}
	return rootTicket.Key, tickets, nil
}

type inspectionQueueItem struct {
	key   string
	depth int
}

func (s *BriefingService) fetchInspectionTickets(rootTicket jira.Ticket, opts TicketInspectionOptions) ([]jira.Ticket, error) {
	tickets := []jira.Ticket{rootTicket}
	seen := map[string]struct{}{rootTicket.Key: {}}
	queue := make([]inspectionQueueItem, 0, opts.MaxRelated)

	enqueueRelated := func(source jira.Ticket, depth int) {
		if depth > opts.Depth {
			return
		}

		relatedKeys := ExtractRelatedTicketKeys(source)
		s.debug("found %d related Jira ticket candidate(s) for %s at depth %d", len(relatedKeys), source.Key, depth)

		for _, relatedKey := range relatedKeys {
			if len(seen)-1 >= opts.MaxRelated {
				return
			}
			if _, ok := seen[relatedKey]; ok {
				continue
			}

			seen[relatedKey] = struct{}{}
			queue = append(queue, inspectionQueueItem{key: relatedKey, depth: depth})
		}
	}

	enqueueRelated(rootTicket, 1)

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		s.debug("fetching full Jira context for related ticket %s at depth %d", item.key, item.depth)
		ticket, err := s.jiraClient.GetBriefingTicketByKey(item.key)
		if err != nil {
			return nil, fmt.Errorf("failed to get related Jira ticket %s: %w", item.key, err)
		}
		tickets = append(tickets, *ticket)

		enqueueRelated(*ticket, item.depth+1)
	}

	return tickets, nil
}

func (s *BriefingService) debug(format string, args ...interface{}) {
	if !s.verbose {
		return
	}
	_, _ = fmt.Fprintf(s.debugOut, "briefing: "+format+"\n", args...)
}

func BuildBriefingContext(tickets []jira.Ticket) (string, error) {
	if len(tickets) == 0 {
		return "", errors.New("cannot build briefing context without Jira tickets")
	}

	var b strings.Builder
	b.WriteString("# Jira Briefing Context\n\n")
	b.WriteString("This output is intended for a code agent. It is source context, not a user-facing briefing.\n\n")
	b.WriteString("Agent instructions:\n")
	b.WriteString("- Use this Jira context to produce the actual briefing for the user.\n")
	b.WriteString("- Identify overall work theme, recommended order, risks, blockers, open questions, and synergies across tickets.\n")
	b.WriteString("- Do not invent facts not present in this context.\n")
	b.WriteString("- Treat truncated fields as incomplete source data.\n\n")
	fmt.Fprintf(&b, "Ticket count: %d\n", len(tickets))
	b.WriteString("Ordering: most recently updated first\n")
	b.WriteString("Jira query: assignee = currentUser() AND updated >= -30d AND statusCategory != Done ORDER BY updated DESC\n\n")
	appendTicketRelationships(&b, tickets)

	for index, ticket := range tickets {
		if index > 0 {
			b.WriteString("\n---\n\n")
		}
		appendTicketContext(&b, ticket, index+1, nil)
	}

	return b.String(), nil
}

func BuildInspectionContext(rootKey string, tickets []jira.Ticket) (string, error) {
	return buildInspectionContext(rootKey, tickets, nil)
}

func buildInspectionContext(rootKey string, tickets []jira.Ticket, attachments map[string][]jiraAttachment) (string, error) {
	if len(tickets) == 0 {
		return "", errors.New("cannot build Jira inspection context without Jira tickets")
	}

	var b strings.Builder
	b.WriteString("# Jira Ticket Inspection Context\n\n")
	b.WriteString("This output is intended for a code agent. It is source context, not a user-facing briefing.\n\n")
	b.WriteString("Agent instructions:\n")
	b.WriteString("- Use this Jira ticket context to understand the requested ticket and directly related work.\n")
	b.WriteString("- Identify implementation scope, dependencies, risks, blockers, and open questions.\n")
	b.WriteString("- Do not invent facts not present in this context.\n")
	b.WriteString("- Treat truncated fields as incomplete source data.\n\n")
	fmt.Fprintf(&b, "Root ticket: %s\n", rootKey)
	fmt.Fprintf(&b, "Ticket count: %d\n", len(tickets))
	b.WriteString("Ordering: root ticket first, then breadth-first related tickets discovered from parent, subtasks, and issue links\n\n")
	appendTicketRelationships(&b, tickets)

	for index, ticket := range tickets {
		if index > 0 {
			b.WriteString("\n---\n\n")
		}
		appendTicketContext(&b, ticket, index+1, attachments[ticket.Key])
	}

	return b.String(), nil
}

func appendTicketRelationships(b *strings.Builder, tickets []jira.Ticket) {
	included := make(map[string]jira.Ticket, len(tickets))
	for _, ticket := range tickets {
		included[ticket.Key] = ticket
	}

	var relationships []string
	for _, ticket := range tickets {
		fields := ticket.Fields
		if fields == nil {
			continue
		}

		if parent, ok := fields["parent"].(map[string]interface{}); ok {
			parentKey, _ := parent["key"].(string)
			if parentKey != "" {
				scope := "external"
				if _, ok := included[parentKey]; ok {
					scope = "included"
				}
				relationships = append(relationships, fmt.Sprintf("- %s parent of %s (%s)", parentKey, ticket.Key, scope))
			}
		}

		if subtasks, ok := fields["subtasks"].([]interface{}); ok {
			for _, subtaskValue := range subtasks {
				subtask, ok := subtaskValue.(map[string]interface{})
				if !ok {
					continue
				}
				subtaskKey, _ := subtask["key"].(string)
				if subtaskKey != "" {
					scope := "external"
					if _, ok := included[subtaskKey]; ok {
						scope = "included"
					}
					relationships = append(relationships, fmt.Sprintf("- %s parent of %s (%s)", ticket.Key, subtaskKey, scope))
				}
			}
		}

		if links, ok := fields["issuelinks"].([]interface{}); ok {
			for _, linkValue := range links {
				link, ok := linkValue.(map[string]interface{})
				if !ok {
					continue
				}
				relationships = append(relationships, ticketLinkRelationships(ticket.Key, link, included)...)
			}
		}
	}

	b.WriteString("## Ticket Relationships\n\n")
	if len(relationships) == 0 {
		b.WriteString("- No parent or issue-link relationships found in the included Jira data.\n\n")
		return
	}

	for _, relationship := range relationships {
		b.WriteString(relationship)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func ExtractRelatedTicketKeys(ticket jira.Ticket) []string {
	seen := map[string]struct{}{
		ticket.Key: {},
	}
	var keys []string

	fields := ticket.Fields
	if fields == nil {
		return keys
	}

	if parent, ok := fields["parent"].(map[string]interface{}); ok {
		appendRelatedTicketKey(&keys, seen, parent["key"])
	}

	if subtasks, ok := fields["subtasks"].([]interface{}); ok {
		for _, subtaskValue := range subtasks {
			if subtask, ok := subtaskValue.(map[string]interface{}); ok {
				appendRelatedTicketKey(&keys, seen, subtask["key"])
			}
		}
	}

	if links, ok := fields["issuelinks"].([]interface{}); ok {
		for _, linkValue := range links {
			link, ok := linkValue.(map[string]interface{})
			if !ok {
				continue
			}
			if outwardIssue, ok := link["outwardIssue"].(map[string]interface{}); ok {
				appendRelatedTicketKey(&keys, seen, outwardIssue["key"])
			}
			if inwardIssue, ok := link["inwardIssue"].(map[string]interface{}); ok {
				appendRelatedTicketKey(&keys, seen, inwardIssue["key"])
			}
		}
	}

	return keys
}

func appendRelatedTicketKey(keys *[]string, seen map[string]struct{}, value interface{}) {
	key, ok := value.(string)
	if !ok {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*keys = append(*keys, key)
}

func ticketLinkRelationships(sourceKey string, link map[string]interface{}, included map[string]jira.Ticket) []string {
	var relationships []string
	typeMap, _ := link["type"].(map[string]interface{})

	if outwardIssue, ok := link["outwardIssue"].(map[string]interface{}); ok {
		targetKey, _ := outwardIssue["key"].(string)
		if targetKey != "" {
			relation, _ := typeMap["outward"].(string)
			relationships = append(relationships, formatTicketRelationship(sourceKey, relation, targetKey, included))
		}
	}

	if inwardIssue, ok := link["inwardIssue"].(map[string]interface{}); ok {
		targetKey, _ := inwardIssue["key"].(string)
		if targetKey != "" {
			relation, _ := typeMap["inward"].(string)
			relationships = append(relationships, formatTicketRelationship(sourceKey, relation, targetKey, included))
		}
	}

	return relationships
}

func formatTicketRelationship(sourceKey, relation, targetKey string, included map[string]jira.Ticket) string {
	if relation == "" {
		relation = "linked to"
	}
	scope := "external"
	if _, ok := included[targetKey]; ok {
		scope = "included"
	}
	return fmt.Sprintf("- %s %s %s (%s)", sourceKey, relation, targetKey, scope)
}

func appendTicketContext(b *strings.Builder, ticket jira.Ticket, index int, attachments []jiraAttachment) {
	fields := ticket.Fields
	if fields == nil {
		fields = map[string]interface{}{}
	}

	fmt.Fprintf(b, "## Ticket %d: %s\n\n", index, ticket.Key)
	fmt.Fprintf(b, "- Key: %s\n", ticket.Key)
	fmt.Fprintf(b, "- ID: %s\n", ticket.ID)
	appendStringField(b, fields, "summary", "Summary")
	appendObjectField(b, fields, "status", "Status")
	appendObjectField(b, fields, "project", "Project")
	appendStringField(b, fields, "created", "Created")
	appendStringField(b, fields, "updated", "Updated")
	appendStringField(b, fields, "resolutiondate", "Resolution Date")
	appendObjectField(b, fields, "assignee", "Assignee")
	appendObjectField(b, fields, "reporter", "Reporter")
	appendObjectField(b, fields, "creator", "Creator")
	appendObjectField(b, fields, "priority", "Priority")
	appendObjectField(b, fields, "labels", "Labels")
	appendObjectField(b, fields, "components", "Components")
	appendObjectField(b, fields, "fixVersions", "Fix Versions")
	appendObjectField(b, fields, "parent", "Parent")
	appendObjectField(b, fields, "subtasks", "Subtasks")
	appendObjectField(b, fields, "issuelinks", "Issue Links")
	if attachments != nil {
		appendDownloadedAttachments(b, attachments)
	} else {
		appendObjectField(b, fields, "attachment", "Attachments")
	}
	appendDevelopmentInfo(b, fields)

	if description, ok := fields["description"]; ok && description != nil {
		appendLongTextField(b, "Description", formatJiraValue(description), maxBriefingDescriptionChars)
	}

	if comments, ok := fields["comment"]; ok && comments != nil {
		appendLongTextField(b, "Comments", formatJiraValue(comments), maxBriefingCommentsChars)
	}

}

func truncateString(value string, maxChars int) string {
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}

	notice := fmt.Sprintf("\n[truncated %d characters]\n", len(value)-maxChars)
	limit := maxChars - len(notice)
	if limit <= 0 {
		return value[:maxChars]
	}

	return value[:limit] + notice
}

func appendStringField(b *strings.Builder, fields map[string]interface{}, key, label string) {
	value, ok := fields[key].(string)
	if !ok || value == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", label, value)
}

func appendObjectField(b *strings.Builder, fields map[string]interface{}, key, label string) {
	value, ok := fields[key]
	if !ok || value == nil {
		return
	}

	switch typed := value.(type) {
	case string:
		if typed == "" {
			return
		}
		fmt.Fprintf(b, "- %s: %s\n", label, typed)
	default:
		formatted := formatJiraValue(typed)
		if formatted == "" {
			return
		}
		formatted = truncateString(formatted, maxBriefingObjectFieldChars)
		fmt.Fprintf(b, "- %s: %s\n", label, formatted)
	}
}

func appendLongTextField(b *strings.Builder, label, value string, maxChars int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}

	fmt.Fprintf(b, "\n%s:\n", label)
	b.WriteString(truncateString(value, maxChars))
	b.WriteString("\n")
}

func formatJiraValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return fmt.Sprintf("%t", typed)
	case float64:
		return fmt.Sprintf("%v", typed)
	case []interface{}:
		return formatJiraList(typed)
	case map[string]interface{}:
		return formatJiraMap(typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func formatJiraList(values []interface{}) string {
	var parts []string
	for _, value := range values {
		formatted := strings.TrimSpace(formatJiraValue(value))
		if formatted != "" {
			parts = append(parts, formatted)
		}
	}
	return strings.Join(parts, "; ")
}

func formatJiraMap(value map[string]interface{}) string {
	if comments, ok := value["comments"].([]interface{}); ok {
		return formatComments(comments)
	}

	if text := extractAtlassianDocText(value); text != "" {
		return text
	}

	if displayName, ok := value["displayName"].(string); ok {
		if email, ok := value["emailAddress"].(string); ok && email != "" {
			return fmt.Sprintf("%s <%s>", displayName, email)
		}
		return displayName
	}

	if key, hasKey := value["key"].(string); hasKey {
		if fields, ok := value["fields"].(map[string]interface{}); ok && fields != nil {
			if summary, ok := fields["summary"].(string); ok && summary != "" {
				return fmt.Sprintf("%s - %s", key, summary)
			}
		}
		if summary, ok := value["summary"].(string); ok && summary != "" {
			return fmt.Sprintf("%s - %s", key, summary)
		}
		return key
	}

	if name, ok := value["name"].(string); ok {
		return name
	}

	if filename, ok := value["filename"].(string); ok {
		if mimeType, ok := value["mimeType"].(string); ok && mimeType != "" {
			return fmt.Sprintf("%s (%s)", filename, mimeType)
		}
		return filename
	}

	if outwardIssue, ok := value["outwardIssue"].(map[string]interface{}); ok {
		return formatIssueLink(value, "outward", outwardIssue)
	}
	if inwardIssue, ok := value["inwardIssue"].(map[string]interface{}); ok {
		return formatIssueLink(value, "inward", inwardIssue)
	}

	return formatFallbackMap(value)
}

func formatComments(comments []interface{}) string {
	var parts []string
	for _, comment := range comments {
		commentMap, ok := comment.(map[string]interface{})
		if !ok {
			continue
		}

		author := formatJiraValue(commentMap["author"])
		created, _ := commentMap["created"].(string)
		body := strings.TrimSpace(formatJiraValue(commentMap["body"]))
		if body == "" {
			continue
		}

		prefix := "-"
		var metadata []string
		if author != "" {
			metadata = append(metadata, author)
		}
		if created != "" {
			metadata = append(metadata, created)
		}
		if len(metadata) > 0 {
			prefix = fmt.Sprintf("- %s:", strings.Join(metadata, ", "))
		}
		parts = append(parts, fmt.Sprintf("%s %s", prefix, body))
	}
	return strings.Join(parts, "\n")
}

func formatIssueLink(link map[string]interface{}, direction string, issue map[string]interface{}) string {
	linkType := ""
	if typeMap, ok := link["type"].(map[string]interface{}); ok {
		if direction == "outward" {
			linkType, _ = typeMap["outward"].(string)
		} else {
			linkType, _ = typeMap["inward"].(string)
		}
	}

	issueText := formatJiraMap(issue)
	if linkType == "" {
		return issueText
	}
	return fmt.Sprintf("%s %s", linkType, issueText)
}

func formatFallbackMap(value map[string]interface{}) string {
	preferredKeys := []string{"value", "title", "accountId", "id", "self"}
	var parts []string
	for _, key := range preferredKeys {
		if formatted := strings.TrimSpace(formatJiraValue(value[key])); formatted != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, formatted))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}

	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return fmt.Sprintf("object with fields: %s", strings.Join(keys, ", "))
	}
	return ""
}
