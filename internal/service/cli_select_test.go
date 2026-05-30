package service

import (
	"strings"
	"testing"
)

func TestParseArrowSelectInput(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		count int
		want  int
	}{
		{name: "enter default", raw: "\n", count: 2, want: 0},
		{name: "down enter", raw: "\x1b[B\n", count: 2, want: 1},
		{name: "up from second", raw: "\x1b[B\x1b[A\n", count: 2, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseArrowSelectInput(tt.raw, tt.count); got != tt.want {
				t.Fatalf("parseArrowSelectInput() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWriteSelectLinesClearsPrevious(t *testing.T) {
	var buf strings.Builder
	count := writeSelectLines(&buf, []string{"line1", "line2"}, 0)
	if count != 2 {
		t.Fatalf("line count = %d", count)
	}
	count = writeSelectLines(&buf, []string{"new1", "new2"}, count)
	if count != 2 {
		t.Fatalf("line count after redraw = %d", count)
	}
	if !strings.Contains(buf.String(), "\033[2K\r") {
		t.Fatal("expected line clear sequence")
	}
}
