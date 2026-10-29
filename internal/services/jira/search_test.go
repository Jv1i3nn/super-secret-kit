package jira

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ssk/internal/models"
)

func TestSearchTicketsPaginatesAndDeduplicates(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/rest/api/3/search/jql" || r.URL.Query().Get("jql") != "project = EX ORDER BY key" {
			t.Errorf("unexpected search request: %s", r.URL)
		}
		switch r.URL.Query().Get("nextPageToken") {
		case "":
			fmt.Fprint(w, `{"issues":[{"id":"2","key":"EX-2"}],"nextPageToken":"page 2","isLast":false}`)
		case "page 2":
			fmt.Fprint(w, `{"issues":[{"id":"2","key":"EX-2"},{"id":"1","key":"EX-1"}],"nextPageToken":"last","isLast":false}`)
		case "last":
			fmt.Fprint(w, `{"issues":[],"isLast":true}`)
		default:
			t.Errorf("unexpected token")
		}
	}))
	defer server.Close()
	tickets, err := NewClient(server.URL, &models.Credentials{}).SearchTickets("project = EX ORDER BY key")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ticket := range tickets {
		ids = append(ids, ticket.ID)
	}
	if !reflect.DeepEqual(ids, []string{"2", "1"}) || calls != 3 {
		t.Fatalf("IDs = %v, calls = %d", ids, calls)
	}
}

func TestSearchTicketsRejectsIncompleteResults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages []string
	}{
		{"missing metadata", []string{`{"issues":[]}`}},
		{"missing issues", []string{`{"isLast":true}`}},
		{"missing token", []string{`{"issues":[],"isLast":false}`}},
		{"contradictory final", []string{`{"issues":[],"isLast":true,"nextPageToken":"again"}`}},
		{"repeated token", []string{`{"issues":[{"id":"1"}],"isLast":false,"nextPageToken":"a"}`, `{"issues":[{"id":"2"}],"isLast":false,"nextPageToken":"a"}`}},
		{"no progress", []string{`{"issues":[{"id":"1"}],"isLast":false,"nextPageToken":"a"}`, `{"issues":[{"id":"1"}],"isLast":false,"nextPageToken":"b"}`}},
		{"missing identity", []string{`{"issues":[{"key":"EX-1"}],"isLast":true}`}},
		{"later failure", []string{`{"issues":[{"id":"1"}],"isLast":false,"nextPageToken":"a"}`, "failure"}},
		{"malformed JSON", []string{`{"issues":`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls >= len(tc.pages) {
					t.Error("unexpected extra page")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				page := tc.pages[calls]
				calls++
				if page == "failure" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				fmt.Fprint(w, page)
			}))
			defer server.Close()
			tickets, err := NewClient(server.URL, &models.Credentials{}).SearchTickets("project = EX")
			if err == nil || tickets != nil {
				t.Fatalf("got tickets %v, error %v; want no partial results and an error", tickets, err)
			}
		})
	}
}

func TestSearchTicketsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"issues":[],"isLast":true}`)
	}))
	defer server.Close()
	tickets, err := NewClient(server.URL, &models.Credentials{}).SearchTickets("project = EX")
	if err != nil || tickets == nil || len(tickets) != 0 {
		t.Fatalf("tickets = %v, error = %v", tickets, err)
	}
}

func TestSearchErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "private description and fixture-secret-token")
	}))
	defer server.Close()
	_, err := NewClient(server.URL, &models.Credentials{}).SearchTickets("project = EX")
	if err == nil || strings.Contains(err.Error(), "private description") || strings.Contains(err.Error(), "fixture-secret-token") {
		t.Fatalf("unsafe diagnostic: %v", err)
	}
}
