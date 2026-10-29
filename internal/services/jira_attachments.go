package services

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"ssk/internal/services/jira"
)

// AttachmentDownloader retrieves one Jira-hosted file into destination.
type AttachmentDownloader interface {
	DownloadAttachment(sourceURL, destination string) error
}

type jiraAttachment struct {
	ID           string
	Filename     string
	MimeType     string
	Size         string
	Author       string
	SourceURL    string
	ReferencedIn []string
	LocalPath    string
	DownloadErr  string
}

func downloadJiraAttachments(ctx context.Context, tickets []jira.Ticket, directory string, downloader AttachmentDownloader) map[string][]jiraAttachment {
	artifacts := collectJiraAttachments(tickets)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		for key, attachments := range artifacts {
			for index := range attachments {
				attachments[index].DownloadErr = fmt.Sprintf("create output directory: %v", err)
			}
			artifacts[key] = attachments
		}
		return artifacts
	}

	for _, ticket := range tickets {
		attachments := artifacts[ticket.Key]
		for index := range attachments {
			if ctx.Err() != nil {
				return artifacts
			}
			attachment := &attachments[index]
			if attachment.SourceURL == "" {
				attachment.DownloadErr = "no downloadable Jira URL"
				continue
			}
			filename := attachmentFilename(ticket.Key, attachment.ID, attachment.Filename, index)
			localPath := filepath.Join(directory, filename)
			if err := downloader.DownloadAttachment(attachment.SourceURL, localPath); err != nil {
				attachment.DownloadErr = sanitizeDownloadError(err, attachment.SourceURL)
				continue
			}
			attachment.LocalPath = localPath
		}
		artifacts[ticket.Key] = attachments
	}
	return artifacts
}

func collectJiraAttachments(tickets []jira.Ticket) map[string][]jiraAttachment {
	artifacts := make(map[string][]jiraAttachment, len(tickets))
	for _, ticket := range tickets {
		attachments, _ := ticket.Fields["attachment"].([]interface{})
		if len(attachments) == 0 {
			continue
		}
		references := mediaReferences(ticket.Fields)
		for _, value := range attachments {
			attachment, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := attachment["id"].(string)
			filename, _ := attachment["filename"].(string)
			if strings.TrimSpace(filename) == "" {
				filename = id
			}
			if filename == "" {
				continue
			}
			media := jiraAttachment{
				ID:        id,
				Filename:  filename,
				MimeType:  stringValue(attachment["mimeType"]),
				Size:      formatAttachmentSize(attachment["size"]),
				Author:    attachmentAuthor(attachment["author"]),
				SourceURL: stringValue(attachment["content"]),
			}
			media.ReferencedIn = attachmentReferences(references, id, filename)
			artifacts[ticket.Key] = append(artifacts[ticket.Key], media)
		}
	}
	return artifacts
}

func mediaReferences(fields map[string]interface{}) map[string][]string {
	references := make(map[string][]string)
	collectMediaReferences(fields["description"], "description", references)
	if comments, ok := fields["comment"].(map[string]interface{}); ok {
		collectMediaReferences(comments, "comments", references)
	}
	return references
}

func collectMediaReferences(value interface{}, location string, references map[string][]string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if typed["type"] == "media" {
			if attrs, ok := typed["attrs"].(map[string]interface{}); ok {
				appendMediaReference(references, stringValue(attrs["id"]), location)
				appendMediaReference(references, "alt:"+stringValue(attrs["alt"]), location)
			}
		}
		for _, child := range typed {
			collectMediaReferences(child, location, references)
		}
	case []interface{}:
		for _, child := range typed {
			collectMediaReferences(child, location, references)
		}
	}
}

func appendMediaReference(references map[string][]string, key, location string) {
	if key == "" || key == "alt:" || containsString(references[key], location) {
		return
	}
	references[key] = append(references[key], location)
}

func attachmentReferences(references map[string][]string, id, filename string) []string {
	var locations []string
	for _, location := range []string{"description", "comments"} {
		if containsString(references[id], location) || containsString(references["alt:"+filename], location) {
			locations = append(locations, location)
		}
	}
	return locations
}

func appendDownloadedAttachments(b *strings.Builder, attachments []jiraAttachment) {
	if len(attachments) == 0 {
		return
	}
	b.WriteString("\n### Attachments\n\n")
	for _, attachment := range attachments {
		b.WriteString("- `")
		b.WriteString(attachment.Filename)
		b.WriteString("`")
		if attachment.MimeType != "" {
			b.WriteString(" — " + attachment.MimeType)
		}
		b.WriteString("\n")
		if attachment.Size != "" {
			b.WriteString("  - Size: " + attachment.Size + "\n")
		}
		if attachment.Author != "" {
			b.WriteString("  - Author: " + attachment.Author + "\n")
		}
		if attachment.SourceURL != "" {
			b.WriteString("  - Source URL: " + displaySourceURL(attachment.SourceURL) + "\n")
		}
		for _, location := range attachment.ReferencedIn {
			b.WriteString("  - Referenced in: " + location + "\n")
		}
		if attachment.LocalPath != "" {
			b.WriteString("  - Local file: " + attachment.LocalPath + "\n")
		}
		if attachment.DownloadErr != "" {
			b.WriteString("  - Download error: " + attachment.DownloadErr + "\n")
		}
	}
}

func safeFilename(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = filepath.Base(filename)
	if filename == "." || filename == "" {
		return "attachment"
	}
	return filename
}

func attachmentFilename(ticketKey, id, filename string, index int) string {
	if id == "" {
		id = fmt.Sprintf("attachment-%d", index+1)
	}
	return strings.Join([]string{safeFilename(ticketKey), safeFilename(id), safeFilename(filename)}, "_")
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func attachmentAuthor(value interface{}) string {
	if author, ok := value.(map[string]interface{}); ok {
		return stringValue(author["displayName"])
	}
	return ""
}

func formatAttachmentSize(value interface{}) string {
	switch size := value.(type) {
	case float64:
		return fmt.Sprintf("%.0f bytes", size)
	case int:
		return fmt.Sprintf("%d bytes", size)
	case int64:
		return fmt.Sprintf("%d bytes", size)
	}
	return ""
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func displaySourceURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid URL]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func sanitizeDownloadError(err error, sourceURL string) string {
	return strings.ReplaceAll(err.Error(), sourceURL, displaySourceURL(sourceURL))
}
