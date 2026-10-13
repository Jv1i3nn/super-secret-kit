package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"ssk/internal/config"
)

func TestJiraURLFirstUseAndReuse(t *testing.T) {
	cfg := &config.Config{}
	var output bytes.Buffer
	saved := ""
	got, err := configureJiraAPIURL(cfg, "default", strings.NewReader("https://team.example/jira/\n"), &output, func(value string) error {
		saved = value
		return nil
	})
	if err != nil || got != "https://team.example/jira" || saved != got {
		t.Fatalf("first use: got %q, saved %q, error %v", got, saved, err)
	}
	if !strings.Contains(output.String(), "Jira URL saved") {
		t.Fatal("missing confirmation")
	}
	output.Reset()
	got, err = configureJiraAPIURL(cfg, "default", strings.NewReader(""), &output, func(string) error {
		t.Fatal("saved URL again")
		return nil
	})
	if err != nil || got != saved || output.Len() != 0 {
		t.Fatalf("reuse: got %q, error %v, output %q", got, err, output.String())
	}
}

func TestJiraURLPromptFailure(t *testing.T) {
	for _, input := range []string{"", "\n", "not-a-url\n", "https://team.example invalid\n"} {
		cfg := &config.Config{}
		var output bytes.Buffer
		_, err := configureJiraAPIURL(cfg, "", strings.NewReader(input), &output, func(string) error {
			t.Fatal("saved invalid URL")
			return nil
		})
		if err == nil || cfg.JiraAPIURL != "" {
			t.Fatalf("input %q: error %v, URL %q", input, err, cfg.JiraAPIURL)
		}
	}
}

func TestJiraURLSaveFailure(t *testing.T) {
	cfg := &config.Config{}
	var output bytes.Buffer
	_, err := configureJiraAPIURL(cfg, "", strings.NewReader("https://team.example\n"), &output, func(string) error {
		return errors.New("disk full")
	})
	if err == nil || cfg.JiraAPIURL != "" || strings.Contains(output.String(), "saved") {
		t.Fatalf("save failure: error %v, URL %q, output %q", err, cfg.JiraAPIURL, output.String())
	}
}

func TestJiraNamedWorkspaceDoesNotPrompt(t *testing.T) {
	cfg := &config.Config{JiraWorkspaces: map[string]string{"other": "https://other.example"}}
	for _, workspace := range []string{"other", "missing"} {
		var output bytes.Buffer
		got, err := configureJiraAPIURL(cfg, workspace, strings.NewReader(""), &output, func(string) error {
			t.Fatal("saved default URL for named workspace")
			return nil
		})
		if output.Len() != 0 || (workspace == "other" && (err != nil || got != "https://other.example")) || (workspace == "missing" && err == nil) {
			t.Fatalf("workspace %q: URL %q, error %v, output %q", workspace, got, err, output.String())
		}
	}
}

func TestTempoPromptUsesConfiguredJiraURL(t *testing.T) {
	if got := tempoAPIKeyPrompt("https://custom.example/jira/"); !strings.Contains(got, "https://custom.example/jira/plugins/servlet/") {
		t.Fatalf("wrong Tempo settings link: %q", got)
	}
}

func TestHoroquartzFirstUsePrompt(t *testing.T) {
	var output bytes.Buffer
	saved := ""
	got, err := promptServiceURL("Horoquartz", "HOROQUARTZ_URL", "https://team.cloud-horoquartz.fr", strings.NewReader("https://hq.example/\n"), &output, func(value string) error {
		saved = value
		return nil
	})
	if err != nil || got != "https://hq.example" || saved != got || !strings.Contains(output.String(), "Horoquartz URL saved") {
		t.Fatalf("Horoquartz setup: URL %q, saved %q, error %v", got, saved, err)
	}
}
