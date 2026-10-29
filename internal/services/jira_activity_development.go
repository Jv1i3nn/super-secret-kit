package services

import "strings"

// JiraExportDevelopment is an optional, best-effort view of development links.
type JiraExportDevelopment struct {
	RetrievalComplete bool     `json:"retrieval_complete"`
	Repositories      []string `json:"repositories"`
	PullRequests      []string `json:"pull_requests"`
	Branches          []string `json:"branches"`
	Commits           []string `json:"commits"`
}

func normalizeActivityDevelopment(value interface{}) *JiraExportDevelopment {
	if value == nil {
		return nil
	}
	info := collectDevelopmentInfo(sanitizeDevelopmentLinks(value))
	if info.isEmpty() {
		return nil
	}
	return &JiraExportDevelopment{
		Repositories: nonNilStrings(info.Repositories), PullRequests: nonNilStrings(info.PullRequests), Branches: nonNilStrings(info.Branches), Commits: nonNilStrings(info.Commits),
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func sanitizeDevelopmentLinks(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if strings.EqualFold(key, "url") || strings.EqualFold(key, "self") {
				if link, ok := item.(string); ok {
					result[key] = displaySourceURL(link)
				}
			} else {
				result[key] = sanitizeDevelopmentLinks(item)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = sanitizeDevelopmentLinks(item)
		}
		return result
	default:
		return value
	}
}
