package services

import (
	"fmt"
	"strings"
)

func extractAtlassianDocText(value map[string]interface{}) string {
	return renderADFNode(value, 0)
}

func renderADFNode(value map[string]interface{}, indent int) string {
	nodeType, _ := value["type"].(string)

	switch nodeType {
	case "doc":
		return renderADFChildren(value, indent, "\n\n")
	case "paragraph":
		return renderADFInlineChildren(value)
	case "text":
		text, _ := value["text"].(string)
		if url := atlassianLinkMarkURL(value); url != "" {
			return formatLinkedText(text, url)
		}
		return text
	case "hardBreak":
		return "\n"
	case "mention":
		return formatADFAttrsText(value, "text", "displayName", "id")
	case "emoji":
		return formatADFAttrsText(value, "shortName", "text", "id")
	case "date":
		return formatADFAttrsText(value, "timestamp")
	case "status":
		return formatADFAttrsText(value, "text")
	case "inlineCard", "blockCard", "embedCard":
		return atlassianURL(value)
	case "bulletList":
		return renderADFList(value, indent, false)
	case "orderedList":
		return renderADFList(value, indent, true)
	case "listItem":
		return renderADFChildren(value, indent, "\n")
	case "taskList", "decisionList":
		return renderADFTaskLikeList(value, indent)
	case "taskItem":
		return renderADFTaskItem(value, indent)
	case "decisionItem":
		return renderADFDecisionItem(value, indent)
	case "codeBlock":
		return renderADFCodeBlock(value)
	case "blockquote":
		return renderADFBlockquote(value, indent)
	case "panel":
		return renderADFPanel(value, indent)
	case "expand", "nestedExpand":
		return renderADFExpand(value, indent)
	case "table":
		return renderADFTable(value)
	case "tableRow":
		return renderADFChildren(value, indent, " | ")
	case "tableCell", "tableHeader":
		return renderADFChildren(value, indent, " ")
	case "rule":
		return "---"
	case "mediaSingle", "mediaGroup":
		return renderADFChildren(value, indent, "\n")
	case "media":
		return renderADFMedia(value)
	default:
		if url := atlassianURL(value); url != "" {
			if text, ok := value["text"].(string); ok && text != "" {
				return formatLinkedText(text, url)
			}
			return url
		}
		if text, ok := value["text"].(string); ok {
			if url := atlassianLinkMarkURL(value); url != "" {
				return formatLinkedText(text, url)
			}
			return text
		}
		return renderADFChildren(value, indent, "\n")
	}
}

func renderADFChildren(value map[string]interface{}, indent int, separator string) string {
	content, ok := value["content"].([]interface{})
	if !ok {
		return ""
	}

	var parts []string
	for _, node := range content {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		text := strings.TrimSpace(renderADFNode(nodeMap, indent))
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, separator)
}

func renderADFInlineChildren(value map[string]interface{}) string {
	content, ok := value["content"].([]interface{})
	if !ok {
		return ""
	}

	var b strings.Builder
	for _, node := range content {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		b.WriteString(renderADFNode(nodeMap, 0))
	}

	return strings.TrimSpace(b.String())
}

func renderADFList(value map[string]interface{}, indent int, ordered bool) string {
	content, ok := value["content"].([]interface{})
	if !ok {
		return ""
	}

	var lines []string
	for index, itemValue := range content {
		item, ok := itemValue.(map[string]interface{})
		if !ok {
			continue
		}

		body := renderADFNode(item, indent+1)
		if strings.TrimSpace(body) == "" {
			continue
		}

		prefix := "-"
		if ordered {
			prefix = fmt.Sprintf("%d.", index+1)
		}
		lines = append(lines, formatIndentedBlock(prefix, body, indent))
	}

	return strings.Join(lines, "\n")
}

func renderADFTaskLikeList(value map[string]interface{}, indent int) string {
	content, ok := value["content"].([]interface{})
	if !ok {
		return ""
	}

	var lines []string
	for _, itemValue := range content {
		item, ok := itemValue.(map[string]interface{})
		if !ok {
			continue
		}
		body := renderADFNode(item, indent)
		if strings.TrimSpace(body) != "" {
			lines = append(lines, body)
		}
	}

	return strings.Join(lines, "\n")
}

func renderADFTaskItem(value map[string]interface{}, indent int) string {
	state := " "
	if attrs, ok := value["attrs"].(map[string]interface{}); ok {
		if localState, ok := attrs["state"].(string); ok && localState == "DONE" {
			state = "x"
		}
	}

	body := renderADFChildren(value, indent+1, " ")
	return formatIndentedBlock(fmt.Sprintf("- [%s]", state), body, indent)
}

func renderADFDecisionItem(value map[string]interface{}, indent int) string {
	body := renderADFChildren(value, indent+1, " ")
	return formatIndentedBlock("- Decision:", body, indent)
}

func renderADFCodeBlock(value map[string]interface{}) string {
	language := ""
	if attrs, ok := value["attrs"].(map[string]interface{}); ok {
		language, _ = attrs["language"].(string)
	}

	code := renderADFChildren(value, 0, "\n")
	return fmt.Sprintf("```%s\n%s\n```", language, code)
}

func renderADFBlockquote(value map[string]interface{}, indent int) string {
	body := renderADFChildren(value, indent, "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

func renderADFPanel(value map[string]interface{}, indent int) string {
	panelType := "panel"
	if attrs, ok := value["attrs"].(map[string]interface{}); ok {
		if localType, ok := attrs["panelType"].(string); ok && localType != "" {
			panelType = localType
		}
	}

	body := renderADFChildren(value, indent, "\n")
	if strings.TrimSpace(body) == "" {
		return fmt.Sprintf("Panel: %s", panelType)
	}
	return fmt.Sprintf("Panel (%s):\n%s", panelType, body)
}

func renderADFExpand(value map[string]interface{}, indent int) string {
	title := "Expand"
	if attrs, ok := value["attrs"].(map[string]interface{}); ok {
		if localTitle, ok := attrs["title"].(string); ok && localTitle != "" {
			title = localTitle
		}
	}

	body := renderADFChildren(value, indent, "\n")
	if strings.TrimSpace(body) == "" {
		return title
	}
	return fmt.Sprintf("%s:\n%s", title, body)
}

func renderADFTable(value map[string]interface{}) string {
	content, ok := value["content"].([]interface{})
	if !ok {
		return ""
	}

	var rows [][]string
	for _, rowValue := range content {
		row, ok := rowValue.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, renderADFTableRow(row))
	}
	if len(rows) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "| "+strings.Join(rows[0], " | ")+" |")
	separator := make([]string, len(rows[0]))
	for i := range separator {
		separator[i] = "---"
	}
	lines = append(lines, "| "+strings.Join(separator, " | ")+" |")
	for _, row := range rows[1:] {
		lines = append(lines, "| "+strings.Join(row, " | ")+" |")
	}

	return strings.Join(lines, "\n")
}

func renderADFTableRow(value map[string]interface{}) []string {
	content, ok := value["content"].([]interface{})
	if !ok {
		return nil
	}

	var cells []string
	for _, cellValue := range content {
		cell, ok := cellValue.(map[string]interface{})
		if !ok {
			continue
		}
		cells = append(cells, sanitizeTableCell(renderADFChildren(cell, 0, " ")))
	}

	return cells
}

func renderADFMedia(value map[string]interface{}) string {
	attrs, ok := value["attrs"].(map[string]interface{})
	if !ok {
		return "Media"
	}

	var parts []string
	for _, key := range []string{"alt", "filename", "type", "id", "collection"} {
		if formatted := strings.TrimSpace(formatJiraValue(attrs[key])); formatted != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, formatted))
		}
	}
	if len(parts) == 0 {
		return "Media"
	}
	return "Media: " + strings.Join(parts, ", ")
}

func formatADFAttrsText(value map[string]interface{}, keys ...string) string {
	attrs, ok := value["attrs"].(map[string]interface{})
	if !ok {
		return ""
	}

	for _, key := range keys {
		if text, ok := attrs[key].(string); ok && text != "" {
			return text
		}
	}

	return ""
}

func formatIndentedBlock(prefix, body string, indent int) string {
	body = strings.TrimSpace(body)
	padding := strings.Repeat("  ", indent)
	if body == "" {
		return padding + prefix
	}

	lines := strings.Split(body, "\n")
	lines[0] = padding + prefix + " " + lines[0]
	for i := 1; i < len(lines); i++ {
		lines[i] = padding + "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func sanitizeTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

func atlassianURL(value map[string]interface{}) string {
	attrs, ok := value["attrs"].(map[string]interface{})
	if !ok {
		return ""
	}

	for _, key := range []string{"url", "href"} {
		if url, ok := attrs[key].(string); ok && url != "" {
			return url
		}
	}

	return ""
}

func atlassianLinkMarkURL(value map[string]interface{}) string {
	marks, ok := value["marks"].([]interface{})
	if !ok {
		return ""
	}

	for _, markValue := range marks {
		mark, ok := markValue.(map[string]interface{})
		if !ok {
			continue
		}
		markType, _ := mark["type"].(string)
		if markType != "link" {
			continue
		}

		attrs, ok := mark["attrs"].(map[string]interface{})
		if !ok {
			continue
		}
		if href, ok := attrs["href"].(string); ok && href != "" {
			return href
		}
	}

	return ""
}

func formatLinkedText(text, url string) string {
	text = strings.TrimSpace(text)
	url = strings.TrimSpace(url)
	if text == "" {
		return url
	}
	if url == "" || text == url {
		return text
	}
	return fmt.Sprintf("%s (%s)", text, url)
}
