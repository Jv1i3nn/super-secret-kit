package jira

import "testing"

func TestMarkdownADFBuildsJiraBlocks(t *testing.T) {
	doc := MarkdownADF("# Title\n\n- **bold** item\n- [link](https://example.com)\n\n```go\nfmt.Println(\"hi\")\n```")

	content := doc["content"].([]interface{})
	if len(content) != 3 {
		t.Fatalf("block count = %d, want 3", len(content))
	}

	heading := content[0].(map[string]interface{})
	if got := heading["type"]; got != "heading" {
		t.Fatalf("first block type = %v, want heading", got)
	}
	if got := heading["attrs"].(map[string]interface{})["level"]; got != 1 {
		t.Fatalf("heading level = %v, want 1", got)
	}

	list := content[1].(map[string]interface{})
	if got := list["type"]; got != "bulletList" {
		t.Fatalf("second block type = %v, want bulletList", got)
	}
	items := list["content"].([]interface{})
	firstItem := items[0].(map[string]interface{})
	firstParagraph := firstItem["content"].([]interface{})[0].(map[string]interface{})
	firstText := firstParagraph["content"].([]interface{})[0].(map[string]interface{})
	firstMark := firstText["marks"].([]interface{})[0].(map[string]interface{})
	if got := firstMark["type"]; got != "strong" {
		t.Fatalf("first list item mark = %v, want strong", got)
	}

	secondItem := items[1].(map[string]interface{})
	secondParagraph := secondItem["content"].([]interface{})[0].(map[string]interface{})
	secondText := secondParagraph["content"].([]interface{})[0].(map[string]interface{})
	secondMark := secondText["marks"].([]interface{})[0].(map[string]interface{})
	if got := secondMark["type"]; got != "link" {
		t.Fatalf("second list item mark = %v, want link", got)
	}

	codeBlock := content[2].(map[string]interface{})
	if got := codeBlock["type"]; got != "codeBlock" {
		t.Fatalf("third block type = %v, want codeBlock", got)
	}
	if got := codeBlock["attrs"].(map[string]interface{})["language"]; got != "go" {
		t.Fatalf("code language = %v, want go", got)
	}
}
