package jira

import (
	"strings"
	"unicode"
)

func PlainTextADF(text string) map[string]interface{} {
	return adfDoc([]interface{}{adfParagraph([]interface{}{adfText(text)})})
}

func MarkdownADF(markdown string) map[string]interface{} {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	var content []interface{}

	for index := 0; index < len(lines); {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			index++
			continue
		}

		if fenceLanguage, ok := codeFenceLanguage(trimmed); ok {
			var codeLines []string
			index++
			for index < len(lines) {
				if strings.HasPrefix(strings.TrimSpace(lines[index]), "```") {
					index++
					break
				}
				codeLines = append(codeLines, lines[index])
				index++
			}
			content = append(content, adfCodeBlock(strings.Join(codeLines, "\n"), fenceLanguage))
			continue
		}

		if level, text, ok := headingLine(trimmed); ok {
			content = append(content, adfHeading(level, parseInlineMarkdown(text)))
			index++
			continue
		}

		if bulletText, ok := bulletLine(trimmed); ok {
			var items []interface{}
			for index < len(lines) {
				itemText, ok := bulletLine(strings.TrimSpace(lines[index]))
				if !ok {
					break
				}
				items = append(items, adfListItem(itemText))
				index++
			}
			content = append(content, map[string]interface{}{"type": "bulletList", "content": items})
			_ = bulletText
			continue
		}

		if numberedText, ok := numberedLine(trimmed); ok {
			var items []interface{}
			for index < len(lines) {
				itemText, ok := numberedLine(strings.TrimSpace(lines[index]))
				if !ok {
					break
				}
				items = append(items, adfListItem(itemText))
				index++
			}
			content = append(content, map[string]interface{}{"type": "orderedList", "content": items})
			_ = numberedText
			continue
		}

		if quoteText, ok := quoteLine(trimmed); ok {
			var quoteLines []string
			quoteLines = append(quoteLines, quoteText)
			index++
			for index < len(lines) {
				nextQuote, ok := quoteLine(strings.TrimSpace(lines[index]))
				if !ok {
					break
				}
				quoteLines = append(quoteLines, nextQuote)
				index++
			}
			content = append(content, map[string]interface{}{
				"type": "blockquote",
				"content": []interface{}{
					adfParagraph(parseInlineMarkdown(strings.Join(quoteLines, "\n"))),
				},
			})
			continue
		}

		var paragraphLines []string
		for index < len(lines) {
			next := strings.TrimSpace(lines[index])
			if next == "" || startsBlock(next) {
				break
			}
			paragraphLines = append(paragraphLines, next)
			index++
		}
		content = append(content, adfParagraph(parseInlineMarkdown(strings.Join(paragraphLines, " "))))
	}

	if len(content) == 0 {
		content = append(content, adfParagraph([]interface{}{adfText(markdown)}))
	}

	return adfDoc(content)
}

func adfDoc(content []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}

func adfParagraph(content []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":    "paragraph",
		"content": content,
	}
}

func adfHeading(level int, content []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":    "heading",
		"attrs":   map[string]interface{}{"level": level},
		"content": content,
	}
}

func adfListItem(text string) map[string]interface{} {
	return map[string]interface{}{
		"type": "listItem",
		"content": []interface{}{
			adfParagraph(parseInlineMarkdown(text)),
		},
	}
}

func adfCodeBlock(code, language string) map[string]interface{} {
	block := map[string]interface{}{
		"type": "codeBlock",
		"content": []interface{}{
			adfText(code),
		},
	}
	if language != "" {
		block["attrs"] = map[string]interface{}{"language": language}
	}
	return block
}

func adfText(text string, marks ...map[string]interface{}) map[string]interface{} {
	node := map[string]interface{}{
		"type": "text",
		"text": text,
	}
	if len(marks) > 0 {
		markValues := make([]interface{}, 0, len(marks))
		for _, mark := range marks {
			markValues = append(markValues, mark)
		}
		node["marks"] = markValues
	}
	return node
}

func parseInlineMarkdown(text string) []interface{} {
	var nodes []interface{}
	var plain strings.Builder

	flushPlain := func() {
		if plain.Len() == 0 {
			return
		}
		nodes = append(nodes, adfText(plain.String()))
		plain.Reset()
	}

	for index := 0; index < len(text); {
		if strings.HasPrefix(text[index:], "**") {
			if end := strings.Index(text[index+2:], "**"); end >= 0 {
				flushPlain()
				nodes = append(nodes, adfText(text[index+2:index+2+end], map[string]interface{}{"type": "strong"}))
				index += end + 4
				continue
			}
		}

		if text[index] == '`' {
			if end := strings.IndexByte(text[index+1:], '`'); end >= 0 {
				flushPlain()
				nodes = append(nodes, adfText(text[index+1:index+1+end], map[string]interface{}{"type": "code"}))
				index += end + 2
				continue
			}
		}

		if text[index] == '[' {
			if node, consumed, ok := parseInlineLink(text[index:]); ok {
				flushPlain()
				nodes = append(nodes, node)
				index += consumed
				continue
			}
		}

		if text[index] == '*' && !strings.HasPrefix(text[index:], "**") {
			if end := strings.IndexByte(text[index+1:], '*'); end >= 0 {
				flushPlain()
				nodes = append(nodes, adfText(text[index+1:index+1+end], map[string]interface{}{"type": "em"}))
				index += end + 2
				continue
			}
		}

		plain.WriteByte(text[index])
		index++
	}

	flushPlain()
	if len(nodes) == 0 {
		return []interface{}{adfText(text)}
	}
	return nodes
}

func parseInlineLink(text string) (map[string]interface{}, int, bool) {
	labelEnd := strings.Index(text, "](")
	if labelEnd <= 0 {
		return nil, 0, false
	}
	urlStart := labelEnd + 2
	urlEnd := strings.IndexByte(text[urlStart:], ')')
	if urlEnd <= 0 {
		return nil, 0, false
	}
	label := text[1:labelEnd]
	url := text[urlStart : urlStart+urlEnd]
	return adfText(label, map[string]interface{}{"type": "link", "attrs": map[string]interface{}{"href": url}}), urlStart + urlEnd + 1, true
}

func codeFenceLanguage(line string) (string, bool) {
	if !strings.HasPrefix(line, "```") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "```")), true
}

func headingLine(line string) (int, string, bool) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(line[level+1:]), true
}

func bulletLine(line string) (string, bool) {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return strings.TrimSpace(line[2:]), true
	}
	return "", false
}

func numberedLine(line string) (string, bool) {
	dot := strings.IndexByte(line, '.')
	if dot <= 0 || dot+1 >= len(line) || line[dot+1] != ' ' {
		return "", false
	}
	for _, r := range line[:dot] {
		if !unicode.IsDigit(r) {
			return "", false
		}
	}
	return strings.TrimSpace(line[dot+2:]), true
}

func quoteLine(line string) (string, bool) {
	if strings.HasPrefix(line, "> ") {
		return strings.TrimSpace(line[2:]), true
	}
	return "", false
}

func startsBlock(line string) bool {
	if strings.HasPrefix(line, "```") {
		return true
	}
	if _, _, ok := headingLine(line); ok {
		return true
	}
	if _, ok := bulletLine(line); ok {
		return true
	}
	if _, ok := numberedLine(line); ok {
		return true
	}
	if _, ok := quoteLine(line); ok {
		return true
	}
	return false
}
