package service

import (
	"bytes"
	"strings"
	"testing"
)

func TestSanitizeCLIInputRemovesIMEArtifacts(t *testing.T) {
	got := sanitizeCLIInput("【你】好世界】")
	want := "你好世界"
	if got != want {
		t.Fatalf("sanitizeCLIInput() = %q, want %q", got, want)
	}
}

func TestSanitizeCLIInputKeepsNormalChinese(t *testing.T) {
	got := sanitizeCLIInput("请读取 README.md")
	want := "请读取 README.md"
	if got != want {
		t.Fatalf("sanitizeCLIInput() = %q, want %q", got, want)
	}
}

func TestFilterCLIInputRune(t *testing.T) {
	if _, ok := filterCLIInputRune('中'); !ok {
		t.Fatal("expected Chinese character to pass filter")
	}
	if _, ok := filterCLIInputRune('【'); ok {
		t.Fatal("expected fullwidth bracket to be filtered")
	}
}

func TestCLILineReaderNonInteractiveUsesScanner(t *testing.T) {
	ui := newCLIUI(&bytes.Buffer{})
	reader, err := newCLILineReader(strings.NewReader("hello\n"), ui, nil)
	if err != nil {
		t.Fatalf("newCLILineReader() error = %v", err)
	}
	line, err := reader.ReadLine(ui, PermAsk)
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if line != "hello" {
		t.Fatalf("ReadLine() = %q, want hello", line)
	}
}
