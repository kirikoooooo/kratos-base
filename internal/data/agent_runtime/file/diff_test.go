package file

import (
	"strings"
	"testing"
)

func TestFormatUnifiedDiffShowsChangedRegion(t *testing.T) {
	before := "line1\nline2\nline3\n"
	after := "line1\nline2-updated\nline3\n"
	diff := FormatUnifiedDiff("README.md", before, after)
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	for _, part := range []string{"--- README.md", "+++ README.md", "-line2", "+line2-updated"} {
		if !strings.Contains(diff, part) {
			t.Fatalf("diff missing %q:\n%s", part, diff)
		}
	}
}

func TestFormatUnifiedDiffIdenticalReturnsEmpty(t *testing.T) {
	content := "same\ncontent\n"
	if got := FormatUnifiedDiff("a.txt", content, content); got != "" {
		t.Fatalf("expected empty diff, got %q", got)
	}
}
