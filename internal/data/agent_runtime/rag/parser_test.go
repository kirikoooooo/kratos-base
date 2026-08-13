package rag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentParserParseMarkdown(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mdPath := filepath.Join(root, "notes.md")
	if err := os.WriteFile(mdPath, []byte("# Title\n\nhello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	parser, err := NewDocumentParser(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parser.Parse(t.Context(), "notes.md")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Parser != "markdown" || !strings.Contains(result.Content, "hello world") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDocumentParserRejectsTraversal(t *testing.T) {
	t.Parallel()
	parser, err := NewDocumentParser(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(t.Context(), "../etc/passwd"); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestDocumentParserUnsupportedExt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	parser, err := NewDocumentParser(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(t.Context(), "a.bin"); err == nil {
		t.Fatal("expected unsupported type error")
	}
}
