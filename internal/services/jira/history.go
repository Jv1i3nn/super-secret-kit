package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Changelog records one dated Jira edit, which may change several fields.
type Changelog struct {
	ID      string          `json:"id"`
	Created string          `json:"created"`
	Author  *User           `json:"author"`
	Items   []ChangelogItem `json:"items"`
}

type ChangelogItem struct {
	FieldID    *string `json:"fieldId"`
	Field      *string `json:"field"`
	From       *string `json:"from"`
	FromString *string `json:"fromString"`
	To         *string `json:"to"`
	ToString   *string `json:"toString"`
}

type issueHistoryPage[T any] struct {
	StartAt    *int  `json:"startAt"`
	Total      *int  `json:"total"`
	MaxResults *int  `json:"maxResults"`
	IsLast     *bool `json:"isLast"`
	Values     []T   `json:"values"`
	Comments   []T   `json:"comments"`
}

// GetChangelogs retrieves complete visible history, without trusting issue expansions.
func (c *Client) GetChangelogs(ctx context.Context, issueKey string) ([]Changelog, error) {
	return getIssueHistoryPages[Changelog](ctx, c, issueKey, "changelog", func(entry Changelog) string { return entry.ID })
}

// GetComments retrieves all visible comments, including old comments edited recently.
func (c *Client) GetComments(ctx context.Context, issueKey string) ([]Comment, error) {
	return getIssueHistoryPages[Comment](ctx, c, issueKey, "comment", func(entry Comment) string { return entry.ID })
}

func getIssueHistoryPages[T any](ctx context.Context, client *Client, issueKey, source string, identity func(T) string) ([]T, error) {
	const maxPages = 100
	result := make([]T, 0)
	seen := make(map[string]bool)
	offset, total := 0, -1
	for page := 1; page <= maxPages; page++ {
		params := url.Values{"startAt": {strconv.Itoa(offset)}, "maxResults": {"100"}}
		if source == "comment" {
			params.Set("orderBy", "created")
		}
		endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/%s?%s", client.BaseURL(), url.PathEscape(issueKey), source, params.Encode())
		response, err := client.makeRequestWithBodyContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("%s page %d failed: %w", source, page, err)
		}
		var data issueHistoryPage[T]
		decodeErr := json.NewDecoder(response.Body).Decode(&data)
		_ = response.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode %s page %d: %w", source, page, decodeErr)
		}
		items := data.Values
		if source == "comment" {
			items = data.Comments
		}
		if items == nil || data.StartAt == nil || data.Total == nil || *data.StartAt != offset || *data.Total < 0 {
			return nil, fmt.Errorf("%s page %d lacks valid pagination metadata", source, page)
		}
		if total >= 0 && total != *data.Total {
			return nil, fmt.Errorf("%s total changed during retrieval; retry the request", source)
		}
		total = *data.Total
		if len(items) > total-offset || (data.MaxResults != nil && (len(items) > *data.MaxResults || *data.MaxResults < 0)) {
			return nil, fmt.Errorf("%s page %d has inconsistent counts", source, page)
		}
		next := offset + len(items)
		last := next == total
		if data.IsLast != nil && *data.IsLast != last {
			return nil, fmt.Errorf("%s page %d has inconsistent completion metadata", source, page)
		}
		before := len(result)
		for _, item := range items {
			id := identity(item)
			if id == "" {
				return nil, fmt.Errorf("%s page %d contains an item without an ID", source, page)
			}
			if !seen[id] {
				seen[id] = true
				result = append(result, item)
			}
		}
		if last {
			if len(result) != total {
				return nil, fmt.Errorf("%s pagination did not retrieve every distinct item", source)
			}
			return result, nil
		}
		if next <= offset || len(result) == before {
			return nil, fmt.Errorf("%s page %d made no progress", source, page)
		}
		offset = next
	}
	return nil, fmt.Errorf("%s exceeded the %d page limit", source, maxPages)
}
