package horoquartz

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const counterAccessDenied = `{"status":"FAILED","messages":[{"level":"ERROR","code":"SLF_0134","field":"function","reason":"Il y a un problème d'accès à la fonction ou de droit par rapport à l'employé"}]}`

func newCountersServer(t *testing.T, allowedRole string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("roleId")
		if role != allowedRole || r.Header.Get("x-hq-roleid") != allowedRole {
			_, _ = w.Write([]byte(counterAccessDenied))
			return
		}
		_, _ = w.Write([]byte(`{"status":"SUCCESS","content":{"employeeCounters":[{"counterName":"CNT_CP","counterLabel":"Congés","counterDomain":"JOUR","counterValue":"12.5"}]}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestGetCountersRoles(t *testing.T) {
	for _, allowedRole := range []string{"12345/0001", "12345/0002"} {
		t.Run(allowedRole, func(t *testing.T) {
			server := newCountersServer(t, allowedRole)
			client := NewClient(server.URL, &Session{RoleID: "12345/0001", RollNumber: "12345"})

			resp, err := client.GetCounters(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("GetCounters() error = %v", err)
			}
			if len(resp.Content.Counters) != 1 || resp.Content.Counters[0].Code != "CNT_CP" {
				t.Fatalf("GetCounters() counters = %+v, want CNT_CP", resp.Content.Counters)
			}
		})
	}
}

func TestGetCountersReportsAPIFailure(t *testing.T) {
	server := newCountersServer(t, "none")
	client := NewClient(server.URL, &Session{RoleID: "12345/0001", RollNumber: "12345"})

	_, err := client.GetCounters(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "SLF_0134") {
		t.Fatalf("GetCounters() error = %v, want SLF_0134", err)
	}
}
