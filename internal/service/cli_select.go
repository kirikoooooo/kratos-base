package service

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type selectOption struct {
	Label string
	Value bool
}

type selectTerminal struct {
	in        *os.File
	out       io.Writer
	oldState  *term.State
	lineCount int
}

func newSelectTerminal(out io.Writer, in io.Reader) (*selectTerminal, bool) {
	file, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, false
	}
	return &selectTerminal{in: file, out: out}, true
}

func (t *selectTerminal) enter() error {
	state, err := term.MakeRaw(int(t.in.Fd()))
	if err != nil {
		return fmt.Errorf("select: terminal raw mode: %w", err)
	}
	t.oldState = state
	return nil
}

func (t *selectTerminal) leave() {
	fmt.Fprint(t.out, "\033[?25h")
	if t.oldState != nil {
		_ = term.Restore(int(t.in.Fd()), t.oldState)
	}
}

// promptArrowSelect shows an interactive menu (↑/↓ + Enter).
func promptArrowSelect(out io.Writer, in io.Reader, paint func(string, string) string, options []selectOption, defaultIndex int) (bool, error) {
	if len(options) == 0 {
		return false, fmt.Errorf("select: no options")
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	termUI, ok := newSelectTerminal(out, in)
	if !ok {
		return options[defaultIndex].Value, nil
	}
	if err := termUI.enter(); err != nil {
		return options[defaultIndex].Value, err
	}
	defer termUI.leave()

	fmt.Fprint(out, "\033[?25l")
	selected := defaultIndex
	hint := paint(ansiDim, "  ↑↓ 选择 · Enter 确认 · Esc 拒绝")

	draw := func() {
		lines := buildSelectLines(hint, options, selected, paint)
		termUI.lineCount = writeSelectLines(out, lines, termUI.lineCount)
		flushOut(out)
	}

	clearMenu := func() {
		if termUI.lineCount <= 0 {
			return
		}
		fmt.Fprintf(out, "\033[%dA", termUI.lineCount)
		for i := 0; i < termUI.lineCount; i++ {
			fmt.Fprint(out, "\033[2K\r\n")
		}
		termUI.lineCount = 0
	}

	draw()
	for {
		key, err := readTerminalKey(termUI.in)
		if err != nil {
			clearMenu()
			return false, err
		}
		switch key {
		case keyUp:
			if selected > 0 {
				selected--
				draw()
			}
		case keyDown:
			if selected < len(options)-1 {
				selected++
				draw()
			}
		case keyEnter:
			clearMenu()
			return options[selected].Value, nil
		case keyEscape, keyCtrlC:
			clearMenu()
			return false, nil
		}
	}
}

func buildSelectLines(hint string, options []selectOption, selected int, paint func(string, string) string) []string {
	lines := make([]string, 0, 2+len(options))
	lines = append(lines, hint, "")
	for i, opt := range options {
		row := "    " + opt.Label
		if i == selected {
			row = paint(ansiCyan, "  ❯ ") + paint(ansiBold, opt.Label)
		}
		lines = append(lines, row)
	}
	return lines
}

func writeSelectLines(out io.Writer, lines []string, prevCount int) int {
	if prevCount > 0 {
		fmt.Fprintf(out, "\033[%dA", prevCount)
	}
	for _, line := range lines {
		fmt.Fprintf(out, "\033[2K\r%s\n", line)
	}
	return len(lines)
}

func flushOut(out io.Writer) {
	if f, ok := out.(*os.File); ok {
		_ = f.Sync()
	}
}

type termKey int

const (
	keyUnknown termKey = iota
	keyUp
	keyDown
	keyEnter
	keyEscape
	keyCtrlC
)

func readTerminalKey(in *os.File) (termKey, error) {
	var b [1]byte
	if _, err := io.ReadFull(in, b[:]); err != nil {
		return keyUnknown, err
	}
	switch b[0] {
	case 3:
		return keyCtrlC, nil
	case 13, 10:
		return keyEnter, nil
	case 27:
		in.SetReadDeadline(deadlineSoon())
		defer in.SetReadDeadline(noDeadline())

		var next [1]byte
		if _, err := in.Read(next[:]); err != nil || next[0] == 0 {
			return keyEscape, nil
		}
		if next[0] == '[' || next[0] == 'O' {
			var code [1]byte
			if _, err := in.Read(code[:]); err != nil {
				return keyEscape, nil
			}
			switch code[0] {
			case 'A':
				return keyUp, nil
			case 'B':
				return keyDown, nil
			}
		}
		return keyEscape, nil
	default:
		return keyUnknown, nil
	}
}

func parseArrowSelectInput(raw string, optionCount int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	selected := 0
	for i := 0; i < len(raw); {
		if raw[i] == '\x1b' && i+2 < len(raw) && raw[i+1] == '[' {
			switch raw[i+2] {
			case 'A':
				if selected > 0 {
					selected--
				}
				i += 3
				continue
			case 'B':
				if selected < optionCount-1 {
					selected++
				}
				i += 3
				continue
			}
		}
		if raw[i] == '\n' || raw[i] == '\r' {
			return selected
		}
		i++
	}
	return selected
}
