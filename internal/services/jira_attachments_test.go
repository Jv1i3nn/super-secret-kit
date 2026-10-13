package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ssk/internal/services/jira"
)

type fakeAttachmentDownloader struct {
	failFor     string
	failWithURL bool
}

func (d fakeAttachmentDownloader) DownloadAttachment(sourceURL, destination string) error {
	if sourceURL == d.failFor {
		if d.failWithURL {
			return fmt.Errorf("request failed for %s", sourceURL)
		}
		return errors.New("download unavailable")
	}
	return os.WriteFile(destination, []byte("artifact"), 0o644)
}

func TestDownloadJiraAttachmentsDownloadsAndAnnotatesEmbeddedMedia(t *testing.T) {
	directory := t.TempDir()
	tickets := []jira.Ticket{{
		Key: "SP-1997",
		Fields: map[string]interface{}{
			"attachment": []interface{}{
				map[string]interface{}{
					"id":       "image-id",
					"filename": "image.png",
					"mimeType": "image/png",
					"size":     float64(12),
					"author":   map[string]interface{}{"displayName": "Ada"},
					"content":  "https://jira.example/rest/api/3/attachment/content/image-id?token=secret",
				},
				map[string]interface{}{
					"id":       "log-id",
					"filename": "../../logs.gz",
					"mimeType": "application/gzip",
					"content":  "https://jira.example/rest/api/3/attachment/content/log-id",
				},
			},
			"description": map[string]interface{}{
				"type": "doc",
				"content": []interface{}{map[string]interface{}{
					"type":  "media",
					"attrs": map[string]interface{}{"id": "media-id", "alt": "image.png"},
				}},
			},
			"comment": map[string]interface{}{
				"comments": []interface{}{map[string]interface{}{
					"body": map[string]interface{}{
						"type":  "media",
						"attrs": map[string]interface{}{"id": "image-id"},
					},
				}},
			},
		},
	}}

	artifacts := downloadJiraAttachments(context.Background(), tickets, directory, fakeAttachmentDownloader{})["SP-1997"]
	if len(artifacts) != 2 {
		t.Fatalf("artifact count = %d, want 2", len(artifacts))
	}
	if artifacts[0].LocalPath != filepath.Join(directory, "SP-1997_image-id_image.png") {
		t.Fatalf("local path = %q", artifacts[0].LocalPath)
	}
	if got := strings.Join(artifacts[0].ReferencedIn, ","); got != "description,comments" {
		t.Fatalf("references = %q", got)
	}
	if artifacts[1].LocalPath != filepath.Join(directory, "SP-1997_log-id_logs.gz") {
		t.Fatalf("sanitized path = %q", artifacts[1].LocalPath)
	}

	var output strings.Builder
	appendDownloadedAttachments(&output, artifacts)
	for _, expected := range []string{
		"Size: 12 bytes",
		"Author: Ada",
		"Source URL: https://jira.example/rest/api/3/attachment/content/image-id",
		"Referenced in: description",
		"Referenced in: comments",
		"Local file: " + filepath.Join(directory, "SP-1997_image-id_image.png"),
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output missing %q:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatalf("output exposes URL query: %s", output.String())
	}
}

func TestDownloadJiraAttachmentsKeepsInspectionAvailableOnFailure(t *testing.T) {
	directory := t.TempDir()
	tickets := []jira.Ticket{{
		Key: "SP-1997",
		Fields: map[string]interface{}{
			"attachment": []interface{}{map[string]interface{}{
				"filename": "failed.txt",
				"content":  "https://jira.example/content/failed?token=secret",
			}},
		},
	}}
	artifacts := downloadJiraAttachments(context.Background(), tickets, directory, fakeAttachmentDownloader{
		failFor:     "https://jira.example/content/failed?token=secret",
		failWithURL: true,
	})
	var output strings.Builder
	appendDownloadedAttachments(&output, artifacts["SP-1997"])
	if !strings.Contains(output.String(), "Download error: request failed for https://jira.example/content/failed") {
		t.Fatalf("missing download error:\n%s", output.String())
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatalf("output exposes URL query: %s", output.String())
	}
}
