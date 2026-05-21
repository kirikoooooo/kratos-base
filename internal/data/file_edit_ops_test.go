package data

import (
	"strings"
	"testing"
)

func TestApplyInsertDeleteReplaceOps(t *testing.T) {
	seed := "L1\nL2\nL3\n"

	got, err := applyInsertLine(seed, 2, "X")
	if err != nil || got != "L1\nX\nL2\nL3\n" {
		t.Fatalf("insert_line got=%q err=%v", got, err)
	}

	got, err = applyInsertAfterLine(seed, 2, "Y")
	if err != nil || got != "L1\nL2\nY\nL3\n" {
		t.Fatalf("insert_after_line got=%q err=%v", got, err)
	}

	got, _, err = applyPrepend(seed, "H")
	if err != nil || !strings.HasPrefix(got, "H\nL1") {
		t.Fatalf("prepend got=%q err=%v", got, err)
	}

	got, err = applyDeleteLine(seed, 2)
	if err != nil || got != "L1\nL3\n" {
		t.Fatalf("delete_line got=%q err=%v", got, err)
	}

	got, err = applyDeleteLines(seed, 2, 3)
	if err != nil || got != "L1\n" {
		t.Fatalf("delete_lines got=%q err=%v", got, err)
	}

	got, err = applyReplaceLine(seed, 2, "R2")
	if err != nil || got != "L1\nR2\nL3\n" {
		t.Fatalf("replace_line got=%q err=%v", got, err)
	}

	got, err = applyReplaceLines(seed, 1, 2, "A\nB")
	if err != nil || got != "A\nB\nL3\n" {
		t.Fatalf("replace_lines got=%q err=%v", got, err)
	}

	got, count, err := applyDeleteString("aa bb aa", "bb", false)
	if err != nil || count != 1 || got != "aa  aa" {
		t.Fatalf("delete_string got=%q count=%d err=%v", got, count, err)
	}
}

func TestApplyInsertMultiline(t *testing.T) {
	seed := "a\nb\n"
	got, err := applyInsertLine(seed, 2, "x\ny")
	if err != nil {
		t.Fatalf("insert multiline err=%v", err)
	}
	if got != "a\nx\ny\nb\n" {
		t.Fatalf("insert multiline got=%q", got)
	}
}
