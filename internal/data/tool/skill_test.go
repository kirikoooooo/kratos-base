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

	r := &ToolExecutor{root: root}
	output, err := r.loadSkill(context.Background(), "rag-search")
	if err != nil {
		t.Fatalf("loadSkill() error = %v", err)
	}
	if !strings.Contains(output, "name: rag-search") || !strings.Contains(output, "path: skills/rag-search/SKILL.md") {
		t.Fatalf("loadSkill() output = %q", output)
	}
}

func TestLoadSkillRejectsTraversal(t *testing.T) {
	r := &ToolExecutor{root: t.TempDir()}
	if _, err := r.loadSkill(context.Background(), "../secret"); err == nil {
		t.Fatal("loadSkill() error = nil, want traversal rejected")
	}
}

func TestSearchSkillsReturnsOnlyMatchingMetadata(t *testing.T) {
	root := t.TempDir()
	writeSkill := func(name, description string) {
		path := filepath.Join(root, "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Long body\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill("rag-search", "Search the local Milvus knowledge base")
	writeSkill("code-review", "Review code changes")
	nested := filepath.Join(root, ".agents", "skills", "debugging", "milvus-debug", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("---\nname: milvus-debug\ndescription: Debug Milvus connections\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := (&ToolExecutor{root: root}).searchSkills(context.Background(), "milvus")
	if err != nil {
		t.Fatalf("searchSkills() error = %v", err)
	}
	if !strings.Contains(output, "rag-search") || !strings.Contains(output, "debugging/milvus-debug") || strings.Contains(output, "code-review") || strings.Contains(output, "Long body") {
		t.Fatalf("searchSkills() output = %q", output)
	}
}
