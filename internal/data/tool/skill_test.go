package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkillReadsNamedSkillInsideConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "skills", "rag-search", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: rag-search\ndescription: Search local RAG\n---\n\n# RAG\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runtime{root: root}
	output, err := r.loadSkill(context.Background(), "rag-search")
	if err != nil {
		t.Fatalf("loadSkill() error = %v", err)
	}
	if !strings.Contains(output, "name: rag-search") || !strings.Contains(output, "path: skills/rag-search/SKILL.md") {
		t.Fatalf("loadSkill() output = %q", output)
	}
}

func TestLoadSkillRejectsTraversal(t *testing.T) {
	r := &Runtime{root: t.TempDir()}
	if _, err := r.loadSkill(context.Background(), "../secret"); err == nil {
		t.Fatal("loadSkill() error = nil, want traversal rejected")
	}
}
