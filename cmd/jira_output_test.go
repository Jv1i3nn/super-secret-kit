package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ssk/internal/database"
	"ssk/internal/models"
	"ssk/internal/repository"
	"ssk/internal/services"
)

// Exercise real command wiring against a temporary encrypted store and HTTP fixtures.
func setupJiraOutputTest(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	directory := t.TempDir()
	t.Setenv("HOME", directory)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("external_project_path", directory)
	viper.Set("database_url", filepath.Join(directory, "test.db"))
	viper.Set("jira_api_url", server.URL)
	viper.Set("horoquartz_url", "https://hq.example")
	passphrase := filepath.Join(directory, ".passphrase")
	if err := os.WriteFile(passphrase, []byte("fixture-passphrase"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.Connect(filepath.Join(directory, "test.db"), false); err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	encryption, err := services.NewEncryptionService(passphrase)
	if err != nil {
		t.Fatal(err)
	}
	credentials := services.NewCredentialsService(repository.NewCredentialsRepository(database.GetDB()), encryption)
	password := "fixture-secret-token"
	if err := credentials.Update(models.ServiceJira, "fixture@example.com", &password); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	oldWorkspace, oldVerbose := jiraWorkspace, verbose
	jiraWorkspace, verbose = "default", true
	t.Cleanup(func() { jiraWorkspace, verbose = oldWorkspace, oldVerbose })
	return server
}

func outputTestCommand(format string) (*cobra.Command, *bytes.Buffer) {
	command := &cobra.Command{}
	command.Flags().String("format", format, "")
	output := new(bytes.Buffer)
	command.SetOut(output)
	return command, output
}

func TestJiraQueryCommandJSONAndText(t *testing.T) {
	calls := 0
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("nextPageToken") == "" {
			fmt.Fprint(w, `{"issues":[{"id":"2","key":"EX-2","fields":{"summary":"Second","status":{"name":"Open"}}}],"isLast":false,"nextPageToken":"next"}`)
		} else {
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1","fields":{"summary":"First","status":{"name":"Open"}}}],"isLast":true}`)
		}
	})
	command, output := outputTestCommand("json")
	if err := runJiraQuery(command, []string{"project = EX ORDER BY updated DESC"}); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(output)
	var result services.JiraExport
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contains more than one document: %v", err)
	}
	if calls != 2 || result.Kind != "jira.query" || result.SchemaVersion != 1 || result.IssueCount != 2 || result.Issues[0].Key != "EX-2" || *result.JQL != "project = EX ORDER BY updated DESC" {
		t.Fatalf("wrong export: %+v", result)
	}
	command, output = outputTestCommand("text")
	if err := runJiraQuery(command, []string{"project = EX"}); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "2 ticket(s)\n\nOpen (2)\n  EX-1         First\n  EX-2         Second\n" {
		t.Fatalf("changed default text: %q", got)
	}
}

func TestJiraQueryCommandLaterFailureHasNoOutput(t *testing.T) {
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("nextPageToken") == "" {
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"EX-1"}],"isLast":false,"nextPageToken":"next"}`)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	})
	for _, format := range []string{"json", "text"} {
		command, output := outputTestCommand(format)
		if err := runJiraQuery(command, []string{"project = EX"}); err == nil || output.Len() != 0 {
			t.Fatalf("format %s: error %v, output %q", format, err, output.String())
		}
	}
}

func TestJiraInspectCommandJSONAndMarkdown(t *testing.T) {
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/dev-status/") {
			fmt.Fprint(w, `{"detail":[]}`)
			return
		}
		if r.URL.Path != "/rest/api/3/issue/EX-1" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{"summary":"Root","parent":{"id":"2","key":"EX-2"},"comment":{"comments":[],"total":0}}}`)
	})
	oldDepth, oldMax, oldDir := jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir
	jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir = 0, 0, ""
	t.Cleanup(func() { jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir = oldDepth, oldMax, oldDir })
	command, output := outputTestCommand("json")
	if err := runJiraInspect(command, []string{"EX-1"}); err != nil {
		t.Fatal(err)
	}
	var result services.JiraExport
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Kind != "jira.inspect" || result.JQL != nil || result.Inspection.RootKey != "EX-1" || !result.Inspection.RelatedOmitted || result.Issues[0].Parent == nil {
		t.Fatalf("wrong inspection: %+v", result)
	}
	if strings.Contains(output.String(), "fixture-secret-token") || strings.Contains(output.String(), "fixture@example.com") {
		t.Fatal("credential leak")
	}
	command, output = outputTestCommand("markdown")
	if err := runJiraInspect(command, []string{"EX-1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "# Jira Ticket Inspection Context\n") || !strings.Contains(output.String(), "Root ticket: EX-1") {
		t.Fatal("changed Markdown default")
	}
}

func TestJiraOutputInvalidFormatFailsBeforeSetup(t *testing.T) {
	for _, run := range []func(*cobra.Command, []string) error{runJiraQuery, runJiraInspect} {
		command, output := outputTestCommand("yaml")
		if err := run(command, []string{"EX-1"}); err == nil || !strings.Contains(err.Error(), "--format") || output.Len() != 0 {
			t.Fatalf("unexpected validation: %v", err)
		}
	}
}

func TestJiraInspectRelatedFailureHasNoOutput(t *testing.T) {
	setupJiraOutputTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/dev-status/"):
			fmt.Fprint(w, `{"detail":[]}`)
		case r.URL.Path == "/rest/api/3/issue/EX-1":
			fmt.Fprint(w, `{"id":"1","key":"EX-1","fields":{"parent":{"id":"2","key":"EX-2"}}}`)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
	oldDepth, oldMax, oldDir := jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir
	jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir = 1, 1, ""
	t.Cleanup(func() { jiraInspectDepth, jiraInspectMaxRelated, jiraInspectDownloadDir = oldDepth, oldMax, oldDir })
	for _, format := range []string{"json", "markdown"} {
		command, output := outputTestCommand(format)
		if err := runJiraInspect(command, []string{"EX-1"}); err == nil || output.Len() != 0 {
			t.Fatalf("format %s: error %v, output %q", format, err, output.String())
		}
	}
}
