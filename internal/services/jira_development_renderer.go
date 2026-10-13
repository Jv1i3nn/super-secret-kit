package services

import (
	"fmt"
	"sort"
	"strings"

	"ssk/internal/services/jira"
)

func appendDevelopmentInfo(b *strings.Builder, fields map[string]interface{}) {
	value, ok := fields[jira.FieldDevelopmentInfo]
	if !ok || value == nil {
		return
	}

	info := collectDevelopmentInfo(value)
	if info.isEmpty() {
		return
	}

	b.WriteString("\nDevelopment Info:\n")
	if len(info.Repositories) > 0 {
		b.WriteString("- Repositories:\n")
		appendDevelopmentItems(b, info.Repositories)
	}
	if len(info.PullRequests) > 0 {
		b.WriteString("- Pull Requests:\n")
		appendDevelopmentItems(b, info.PullRequests)
	}
	if len(info.Branches) > 0 {
		b.WriteString("- Branches:\n")
		appendDevelopmentItems(b, info.Branches)
	}
	if len(info.Commits) > 0 {
		b.WriteString("- Commits:\n")
		appendDevelopmentItems(b, info.Commits)
	}
}

type developmentInfo struct {
	Repositories []string
	PullRequests []string
	Branches     []string
	Commits      []string
}

func (i developmentInfo) isEmpty() bool {
	return len(i.Repositories) == 0 && len(i.PullRequests) == 0 && len(i.Branches) == 0 && len(i.Commits) == 0
}

func appendDevelopmentItems(b *strings.Builder, items []string) {
	for _, item := range items {
		fmt.Fprintf(b, "  - %s\n", item)
	}
}

func collectDevelopmentInfo(value interface{}) developmentInfo {
	info := developmentInfo{}
	seen := map[string]struct{}{}
	collectDevelopmentInfoValue(value, &info, seen)
	sort.Strings(info.Repositories)
	sort.Strings(info.PullRequests)
	sort.Strings(info.Branches)
	sort.Strings(info.Commits)
	return info
}

func collectDevelopmentInfoValue(value interface{}, info *developmentInfo, seen map[string]struct{}) {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			collectDevelopmentInfoValue(item, info, seen)
		}
	case map[string]interface{}:
		for key, nested := range typed {
			category, ok := developmentCategoryForKey(key)
			if !ok {
				continue
			}
			for _, item := range interfaceList(nested) {
				if itemMap, ok := item.(map[string]interface{}); ok {
					appendDevelopmentItem(info, seen, category, formatDevelopmentItem(itemMap, category))
				}
			}
		}

		for _, nested := range typed {
			collectDevelopmentInfoValue(nested, info, seen)
		}
	}
}

func developmentCategoryForKey(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "repositories":
		return "repository", true
	case "pullrequests", "pullrequest", "pull_requests", "pull_request":
		return "pullrequest", true
	case "branches", "branch":
		return "branch", true
	case "commits", "commit":
		return "commit", true
	default:
		return "", false
	}
}

func interfaceList(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case map[string]interface{}:
		return []interface{}{typed}
	default:
		return nil
	}
}

func appendDevelopmentItem(info *developmentInfo, seen map[string]struct{}, category, item string) {
	item = strings.TrimSpace(item)
	if item == "" {
		return
	}

	key := category + ":" + item
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}

	switch category {
	case "repository":
		info.Repositories = append(info.Repositories, item)
	case "pullrequest":
		info.PullRequests = append(info.PullRequests, item)
	case "branch":
		info.Branches = append(info.Branches, item)
	case "commit":
		info.Commits = append(info.Commits, item)
	}
}

func formatDevelopmentItem(item map[string]interface{}, category string) string {
	switch category {
	case "pullrequest":
		return formatDevelopmentPullRequest(item)
	case "branch":
		return formatDevelopmentBranch(item)
	case "commit":
		return formatDevelopmentCommit(item)
	case "repository":
		return formatDevelopmentRepository(item)
	default:
		return formatDevelopmentGeneric(item)
	}
}

func formatDevelopmentPullRequest(item map[string]interface{}) string {
	title := firstStringField(item, "title", "name", "displayName", "id")
	if title == "" {
		title = "pull request"
	}

	var parts []string
	if status := firstStringField(item, "status", "state"); status != "" {
		parts = append(parts, status)
	}
	if source := branchNameFromField(item["source"]); source != "" {
		parts = append(parts, "source="+source)
	}
	if destination := branchNameFromField(item["destination"]); destination != "" {
		parts = append(parts, "target="+destination)
	}
	if url := firstStringField(item, "url", "self"); url != "" {
		parts = append(parts, url)
	}
	return joinDevelopmentParts(title, parts)
}

func formatDevelopmentBranch(item map[string]interface{}) string {
	name := firstStringField(item, "name", "displayId", "id")
	if name == "" {
		name = "branch"
	}

	var parts []string
	if repository := developmentNestedName(item["repository"]); repository != "" {
		parts = append(parts, "repository="+repository)
	}
	if url := firstStringField(item, "url", "self"); url != "" {
		parts = append(parts, url)
	}
	return joinDevelopmentParts(name, parts)
}

func formatDevelopmentCommit(item map[string]interface{}) string {
	hash := firstStringField(item, "displayId", "hash", "id")
	message := firstStringField(item, "message", "title")

	title := hash
	if message != "" {
		if title != "" {
			title += " - " + message
		} else {
			title = message
		}
	}
	if title == "" {
		title = "commit"
	}

	var parts []string
	if author := developmentNestedName(item["author"]); author != "" {
		parts = append(parts, "author="+author)
	}
	if timestamp := firstStringField(item, "authorTimestamp", "timestamp", "date"); timestamp != "" {
		parts = append(parts, timestamp)
	}
	if url := firstStringField(item, "url", "self"); url != "" {
		parts = append(parts, url)
	}
	return joinDevelopmentParts(title, parts)
}

func formatDevelopmentRepository(item map[string]interface{}) string {
	name := firstStringField(item, "name", "displayName", "slug", "id")
	if name == "" {
		name = "repository"
	}

	var parts []string
	if url := firstStringField(item, "url", "self"); url != "" {
		parts = append(parts, url)
	}
	return joinDevelopmentParts(name, parts)
}

func formatDevelopmentGeneric(item map[string]interface{}) string {
	title := firstStringField(item, "title", "name", "displayName", "displayId", "id", "url")
	if title != "" {
		return title
	}
	return formatFallbackMap(item)
}

func joinDevelopmentParts(title string, parts []string) string {
	if len(parts) == 0 {
		return title
	}
	return fmt.Sprintf("%s (%s)", title, strings.Join(parts, ", "))
}

func firstStringField(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key].(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func developmentNestedName(value interface{}) string {
	nested, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	return firstStringField(nested, "name", "displayName", "displayId", "slug", "id")
}

func branchNameFromField(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	if name := developmentNestedName(value); name != "" {
		return name
	}
	return ""
}
