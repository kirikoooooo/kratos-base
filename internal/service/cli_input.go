package service

import (
	"bufio"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
)

// cliLineReader reads interactive input with UTF-8 aware editing on TTYs.
// Falls back to bufio.Scanner for pipes and tests.
type cliLineReader struct {
	rl    *readline.Instance
	scan  *bufio.Scanner
	useRL bool
}

func newCLILineReader(in io.Reader, ui *cliUI, onShiftTab func()) (*cliLineReader, error) {
	if file, ok := in.(*os.File); ok && stdinIsInteractive(file) {
		stdin := io.ReadCloser(file)
		if onShiftTab != nil {
			stdin = &shiftTabReadCloser{shiftTabReader: newShiftTabReader(file, onShiftTab)}
		}
		rl, err := readline.NewEx(&readline.Config{
			Prompt:                 ui.readlinePrompt(PermAsk),
			Stdin:                  stdin,
			HistoryLimit:           200,
			DisableAutoSaveHistory: true,
			FuncFilterInputRune:    filterCLIInputRune,
			InterruptPrompt:        "^C",
			EOFPrompt:              "exit",
				AutoComplete:           slashCommandCompleter(),
		})
		if err != nil {
			return nil, err
		}
		return &cliLineReader{rl: rl, useRL: true}, nil
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &cliLineReader{scan: scanner, useRL: false}, nil
}

func (r *cliLineReader) Close() error {
	if r == nil || r.rl == nil {
		return nil
	}
	return r.rl.Close()
}

func (r *cliLineReader) ReadLine(ui *cliUI, mode PermissionMode) (string, error) {
	if r.useRL {
		r.rl.SetPrompt(ui.readlinePrompt(mode))
		ui.println("")
		line, err := r.rl.Readline()
		if err != nil {
			return "", err
		}
		return sanitizeCLIInput(line), nil
	}

	ui.printPrompt(PermAsk)
	if !r.scan.Scan() {
		if err := r.scan.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return sanitizeCLIInput(r.scan.Text()), nil
}

func (r *cliLineReader) SetPrompt(ui *cliUI, mode PermissionMode) {
	if r.rl != nil {
		r.rl.SetPrompt(ui.readlinePrompt(mode))
	}
}

func (r *cliLineReader) ReadLineWithPrompt(ui *cliUI, prompt string, mode PermissionMode) (string, error) {
	if r.useRL {
		r.rl.SetPrompt(prompt)
		line, err := r.rl.Readline()
		if err != nil {
			return "", err
		}
		return sanitizeCLIInput(line), nil
	}
	ui.printf("%s", prompt)
	if !r.scan.Scan() {
		if err := r.scan.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return sanitizeCLIInput(r.scan.Text()), nil
}

func (r *cliLineReader) PauseForOverlay() {
	if r != nil && r.rl != nil {
		r.rl.Clean()
	}
}

func (r *cliLineReader) ResumeAfterOverlay() {
	if r != nil && r.rl != nil {
		r.rl.Refresh()
	}
}

func stdinIsInteractive(file *os.File) bool {
	fi, err := file.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func (u *cliUI) readlinePrompt(mode PermissionMode) string {
	tag := u.permissionModeTag(mode)
	return u.cyan("router") + " " + tag + " " + u.dim("›") + " "
}

func (u *cliUI) permissionModeTag(mode PermissionMode) string {
	switch mode {
	case PermAgent:
		return u.yellow("[" + mode.String() + "]")
	case PermAuto:
		return u.green("[" + mode.String() + "]")
	default:
		return u.dim("[" + mode.String() + "]")
	}
}

func (u *cliUI) printPermissionMode(mode PermissionMode) {
	u.println(u.dim("  权限模式  ") + u.permissionModeTag(mode) + u.dim("  ·  "+mode.Description()))
}

// filterCLIInputRune drops IME composition artifacts common on macOS terminals.
func filterCLIInputRune(r rune) (rune, bool) {
	switch r {
	case '\u3010', '\u3011', // 【】
		'\uFF3B', '\uFF3D', // ［］ fullwidth brackets
		'\u200B', '\u200C', '\u200D', '\uFEFF': // zero-width / BOM
		return r, false
	}
	if unicode.In(r, unicode.Cf) {
		return r, false
	}
	return r, true
}

func sanitizeCLIInput(line string) string {
	line = strings.Map(func(r rune) rune {
		switch r {
		case '\u3010', '\u3011', '\uFF3B', '\uFF3D', '\u200B', '\u200C', '\u200D', '\uFEFF':
			return -1
		}
		if unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, line)
	return strings.TrimSpace(line)
}

// slashCommandCompleter builds a Tab completer for all slash commands.
// Press Tab to autocomplete or see available commands in a grid.
func slashCommandCompleter() *readline.PrefixCompleter {
	items := []readline.PrefixCompleterInterface{
		readline.PcItem("/help"),
		readline.PcItem("/agents"),
		readline.PcItem("/session"),
		readline.PcItem("/new"),
		readline.PcItem("/model"),
		readline.PcItem("/mode",
			readline.PcItem("ask"),
			readline.PcItem("agent"),
			readline.PcItem("auto"),
		),
		readline.PcItem("/config",
			readline.PcItem("api_key"),
			readline.PcItem("base_url"),
		),
		readline.PcItem("/exit"),
	}
	return readline.NewPrefixCompleter(items...)
}
