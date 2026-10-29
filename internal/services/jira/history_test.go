package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ssk/internal/models"
)

func historyFixtureRequest(ctx context.Context, client *Client, source string) (int, error) {
	if source == "comment" {
		items, err := client.GetComments(ctx, "EX-1")
		if err != nil && items != nil {
			return -1, err
		}
		return len(items), err
	}
	items, err := client.GetChangelogs(ctx, "EX-1")
	if err != nil && items != nil {
		return -1, err
	}
	return len(items), err
}

func TestIssueHistoryPagination(t *testing.T) {
	for _, source := range []string{"changelog", "comment"} {
		t.Run(source, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/rest/api/3/issue/EX-1/"+source || r.URL.Query().Get("maxResults") != "100" {
					t.Errorf("unexpected request %s", r.URL)
				}
				if source == "comment" && r.URL.Query().Get("orderBy") != "created" {
					t.Error("comment creation ordering missing")
				}
				offset, err := strconv.Atoi(r.URL.Query().Get("startAt"))
				if err != nil {
					t.Error(err)
				}
				field := "values"
				if source == "comment" {
					field = "comments"
				}
				// Server returns fewer items than requested: advance by actual page size.
				fmt.Fprintf(w, `{"startAt":%d,"maxResults":1,"total":3,"isLast":%t,%q:[{"id":"%d"}]}`, offset, offset == 2, field, offset+1)
			}))
			defer server.Close()
			count, err := historyFixtureRequest(context.Background(), NewClient(server.URL, &models.Credentials{}), source)
			if err != nil || count != 3 || calls != 3 {
				t.Fatalf("count=%d calls=%d error=%v", count, calls, err)
			}
		})
	}
}

func TestIssueHistoryPaginationRejectsIncompleteData(t *testing.T) {
	for _, source := range []string{"changelog", "comment"} {
		for _, tc := range []struct {
			name  string
			pages []string
		}{
			{"missing metadata", []string{`{"ITEMS":[]}`}},
			{"missing collection", []string{`{"startAt":0,"total":0}`}},
			{"wrong offset", []string{`{"startAt":1,"total":1,"ITEMS":[{"id":"1"}]}`}},
			{"no progress", []string{`{"startAt":0,"total":1,"ITEMS":[]}`}},
			{"missing ID", []string{`{"startAt":0,"total":1,"ITEMS":[{}]}`}},
			{"contradictory completion", []string{`{"startAt":0,"total":2,"isLast":true,"ITEMS":[{"id":"1"}]}`}},
			{"invalid counts", []string{`{"startAt":0,"total":0,"ITEMS":[{"id":"1"}]}`}},
			{"changed total", []string{`{"startAt":0,"total":2,"ITEMS":[{"id":"1"}]}`, `{"startAt":1,"total":3,"ITEMS":[{"id":"2"}]}`}},
			{"repeated page", []string{`{"startAt":0,"total":2,"ITEMS":[{"id":"1"}]}`, `{"startAt":0,"total":2,"ITEMS":[{"id":"1"}]}`}},
			{"duplicate ID", []string{`{"startAt":0,"total":2,"ITEMS":[{"id":"1"}]}`, `{"startAt":1,"total":2,"ITEMS":[{"id":"1"}]}`}},
			{"later access failure", []string{`{"startAt":0,"total":2,"ITEMS":[{"id":"1"}]}`, "forbidden"}},
			{"malformed JSON", []string{`{`}},
		} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if calls >= len(tc.pages) {
						t.Error("unexpected page")
						w.WriteHeader(500)
						return
					}
					page := tc.pages[calls]
					calls++
					if page == "forbidden" {
						w.WriteHeader(403)
						fmt.Fprint(w, "fixture-secret-token")
						return
					}
					field := "values"
					if source == "comment" {
						field = "comments"
					}
					fmt.Fprint(w, strings.ReplaceAll(page, "ITEMS", field))
				}))
				defer server.Close()
				count, err := historyFixtureRequest(context.Background(), NewClient(server.URL, &models.Credentials{}), source)
				if err == nil || count != 0 || strings.Contains(err.Error(), "fixture-secret-token") {
					t.Fatalf("count=%d, error=%v", count, err)
				}
			})
		}
	}
}

func TestIssueHistoryEmptyAndPageLimit(t *testing.T) {
	for _, source := range []string{"changelog", "comment"} {
		for _, total := range []int{0, 100, 101} {
			t.Run(fmt.Sprintf("%s/total=%d", source, total), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if calls > 100 {
						t.Error("exceeded 100 page limit")
						w.WriteHeader(500)
						return
					}
					offset, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
					field := "values"
					if source == "comment" {
						field = "comments"
					}
					items := []map[string]string{}
					if total > 0 {
						items = append(items, map[string]string{"id": strconv.Itoa(offset + 1)})
					}
					payload := map[string]interface{}{"startAt": offset, "total": total, "maxResults": 1, field: items}
					_ = json.NewEncoder(w).Encode(payload)
				}))
				defer server.Close()
				count, err := historyFixtureRequest(context.Background(), NewClient(server.URL, &models.Credentials{}), source)
				if total <= 100 {
					if err != nil || count != total {
						t.Fatalf("count=%d error=%v", count, err)
					}
				} else if err == nil || count != 0 || !strings.Contains(err.Error(), "100 page limit") {
					t.Fatalf("count=%d error=%v", count, err)
				}
			})
		}
	}
}

func TestIssueHistoryHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("request after cancellation") }))
	defer server.Close()
	for _, source := range []string{"comment", "changelog"} {
		if _, err := historyFixtureRequest(ctx, NewClient(server.URL, &models.Credentials{}), source); err == nil {
			t.Fatal("cancelled request succeeded")
		}
	}
}
