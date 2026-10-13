package jira

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ssk/internal/models"
)

func TestClientAddCommentPostsADFBody(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/PROJ-123/comment" {
			t.Fatalf("path = %s, want /rest/api/3/issue/PROJ-123/comment", r.URL.Path)
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:token"))
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Fatalf("Authorization = %q, want %q", got, wantAuth)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"10000","self":"https://example.atlassian.net/rest/api/3/issue/PROJ-123/comment/10000"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, &models.Credentials{
		Username: "user@example.com",
		Password: "token",
	})

	comment, err := client.AddComment("PROJ-123", "hello world")
	if err != nil {
		t.Fatalf("AddComment() error = %v", err)
	}
	if comment.ID != "10000" {
		t.Fatalf("comment ID = %q, want 10000", comment.ID)
	}

	body, ok := gotBody["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("body missing or wrong type: %#v", gotBody["body"])
	}
	if got := body["type"]; got != "doc" {
		t.Fatalf("ADF body type = %v, want doc", got)
	}
	if got := body["version"]; got != float64(1) {
		t.Fatalf("ADF body version = %v, want 1", got)
	}

	content := body["content"].([]interface{})
	paragraph := content[0].(map[string]interface{})
	textNodes := paragraph["content"].([]interface{})
	textNode := textNodes[0].(map[string]interface{})
	if got := textNode["text"]; got != "hello world" {
		t.Fatalf("comment text = %v, want hello world", got)
	}
}

func TestClientDownloadAttachmentRejectsExternalURL(t *testing.T) {
	client := NewClient("https://jira.example", &models.Credentials{Username: "user", Password: "token"})
	err := client.DownloadAttachment("https://other.example/file", filepath.Join(t.TempDir(), "file"))
	if err == nil || !strings.Contains(err.Error(), "outside the configured Jira site") {
		t.Fatalf("DownloadAttachment() error = %v, want external URL rejection", err)
	}
}

func TestClientDownloadAttachmentAcceptsAnyContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Fatalf("Accept = %q, want */*", got)
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write([]byte("artifact"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "artifact.gz")
	client := NewClient(server.URL, &models.Credentials{Username: "user", Password: "token"})
	if err := client.DownloadAttachment(server.URL+"/attachment", destination); err != nil {
		t.Fatalf("DownloadAttachment() error = %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "artifact" {
		t.Fatalf("content = %q, want artifact", content)
	}
}

func TestClientDownloadAttachmentDoesNotOverwriteExistingFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("new artifact"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "artifact.gz")
	if err := os.WriteFile(destination, []byte("existing artifact"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	client := NewClient(server.URL, &models.Credentials{Username: "user", Password: "token"})
	if err := client.DownloadAttachment(server.URL+"/attachment", destination); err == nil {
		t.Fatal("DownloadAttachment() error = nil, want existing file error")
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "existing artifact" {
		t.Fatalf("content = %q, want existing artifact", content)
	}
}
