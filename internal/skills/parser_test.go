package skills

import "testing"

func TestSplitFrontmatterSupportsLF(t *testing.T) {
	meta, body, ok := splitFrontmatter([]byte("---\nname: demo\n---\nPrompt\n"))
	if !ok {
		t.Fatal("expected LF frontmatter to parse")
	}
	if string(meta) != "name: demo" || body != "Prompt\n" {
		t.Fatalf("unexpected split: meta=%q body=%q", meta, body)
	}
}

func TestSplitFrontmatterSupportsCRLF(t *testing.T) {
	meta, body, ok := splitFrontmatter([]byte("---\r\nname: demo\r\n---\r\nPrompt\r\n"))
	if !ok {
		t.Fatal("expected CRLF frontmatter to parse")
	}
	if string(meta) != "name: demo" || body != "Prompt\n" {
		t.Fatalf("unexpected split: meta=%q body=%q", meta, body)
	}
}
