package service

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/consts/public"
	datatrace "kratos-demo/internal/data/trace"
)

var codeFenceRE = regexp.MustCompile("(?s)```([^\n`]*)\n(.*?)```")

type cliUI struct {
	out           io.Writer
	color         bool
	mu            sync.Mutex
	activeSpinner *cliSpinner
}

func newCLIUI(out io.Writer) *cliUI {
	return &cliUI{out: out, color: cliColorEnabled(out)}
}

func cliColorEnabled(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func (u *cliUI) paint(code, text string) string {
	if !u.color {
		return text
	}
	return code + text + public.AnsiReset
}

func (u *cliUI) dim(text string) string     { return u.paint(public.AnsiDim, text) }
func (u *cliUI) bold(text string) string    { return u.paint(public.AnsiBold, text) }
func (u *cliUI) cyan(text string) string    { return u.paint(public.AnsiCyan, text) }
func (u *cliUI) green(text string) string   { return u.paint(public.AnsiGreen, text) }
func (u *cliUI) yellow(text string) string  { return u.paint(public.AnsiYellow, text) }
func (u *cliUI) magenta(text string) string { return u.paint(public.AnsiMagenta, text) }
func (u *cliUI) blue(text string) string    { return u.paint(public.AnsiBlue, text) }
func (u *cliUI) red(text string) string     { return u.paint(public.AnsiRed, text) }

func (u *cliUI) println(text string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fmt.Fprintln(u.out, text)
}

func (u *cliUI) printf(format string, args ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fmt.Fprintf(u.out, format, args...)
}

func (u *cliUI) printBanner(title, subtitle string) {
	u.println("")
	u.println(u.bold("  " + title))
	u.println(u.dim("  " + subtitle))
	u.println("")
}

func (u *cliUI) printWelcome(sessionID, logPath string) {
	u.printBanner("MyAgent", "local multi-agent workspace · router-first")
	u.println(u.dim("  agents  ") + u.green("●") + " default  " + u.green("●") + " router  " + u.green("●") + " coder  " + u.green("●") + " reviewer")
	u.println(u.dim("  session ") + sessionID)
	if logPath = strings.TrimSpace(logPath); logPath != "" {
		u.println(u.dim("  logs    ") + logPath)
	}
	u.println(u.dim("  tips      ") + "/help · /config · /model · /new · /exit · Shift+Tab 切换权限")
	u.println(u.dim("  safety    ") + "ask/agent/auto · 高风险操作 ↑↓ 选择 Enter 确认")
	u.println(u.dim("  ctrl+c    ") + "生成中：首次停止 · 再次退出 · 空闲时连按两次退出")
	u.println("")
}

func (u *cliUI) printPrompt(mode PermissionMode) {
	u.printf("\n%s %s %s ", u.cyan("router"), u.permissionModeTag(mode), u.dim("›"))
}

func (u *cliUI) printHelp() {
	u.println("")
	u.println(u.bold("  commands"))
	u.println(u.dim("  /help     ") + "show help")
	u.println(u.dim("  /agents   ") + "agent status")
	u.println(u.dim("  /session  ") + "session id")
	u.println(u.dim("  /new      ") + "new session")
	u.println(u.dim("  /mode     ") + "permission mode (ask/agent/auto)")
	u.println(u.dim("  /config   ") + "API key / base URL")
	u.println(u.dim("  /model    ") + "show / switch model")
	u.println(u.dim("  /exit     ") + "quit")
	u.println("")
	u.println(u.dim("  Shift+Tab ") + "cycle permission: ask → agent → auto")
	u.println(u.dim("  Ctrl+C    ") + "stop generation (1st) · exit CLI (2nd)")
	u.println("")
}

func (u *cliUI) printAgents(states []dashboardAgentState) {
	u.println("")
	for _, state := range states {
		dot := u.dim("○")
		status := "idle"
		if state.Started {
			dot = u.green("●")
			status = "ready"
		}
		u.println(fmt.Sprintf("  %s %-10s %s", dot, state.Agent, u.dim(status)))
	}
	u.println("")
}

func (u *cliUI) printUserTurn(prompt string) {
	u.println("")
	for _, line := range strings.Split(prompt, "\n") {
		u.println(u.dim("  you › ") + line)
	}
}

func (u *cliUI) printAssistant(agent, summary, output string) {
	u.println("")
	header := u.magenta("  ◆ ") + u.bold(agent)
	if summary = strings.TrimSpace(summary); summary != "" {
		header += u.dim("  ·  ") + u.dim(summary)
	}
	u.println(header)
	u.println("")
	renderCLIBlocks(u, "  ", output)
	u.println("")
}

func (u *cliUI) printError(err error) {
	u.println("")
	u.println(u.red("  ✕ ") + err.Error())
	u.println("")
}

func (u *cliUI) printProgressLine(line string) {
	u.mu.Lock()
	s := u.activeSpinner
	u.mu.Unlock()
	if s != nil {
		s.note(line)
		return
	}
	u.println(line)
}

func (u *cliUI) printPlanPresented(event datatrace.DelegationEvent) {
	text := strings.TrimSpace(event.ToolOutput)
	if text == "" {
		text = strings.TrimSpace(event.Summary)
	}
	u.printProgressLine(u.cyan("  📋 执行计划"))
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		u.printProgressLine(u.dim("     ") + line)
	}
}

func formatCLIToolTarget(event datatrace.DelegationEvent) string {
	tool := strings.TrimSpace(event.ToolName)
	if tool == "" && strings.HasPrefix(event.Stage, "tool_") {
		tool = strings.TrimPrefix(event.Stage, "tool_")
	}
	target := strings.TrimSpace(event.ToolInput)
	if target == "" {
		target = strings.TrimSpace(event.Summary)
	}
	switch tool {
	case "read_file", "write_file", "edit_file", "delete_file":
		return target
	case "exec_command":
		if len([]rune(target)) > 72 {
			return string([]rune(target)[:72]) + "…"
		}
		return target
	default:
		return target
	}
}

func formatCLIEventLine(event datatrace.DelegationEvent, color bool) string {
	stage := strings.TrimSpace(event.Stage)
	if stage == "" || stage == "message_received" || stage == "message_done" || stage == "final_answer" || stage == "handle_direct" {
		return ""
	}

	prefix := "  "
	var body string
	switch {
	case stage == "delegate_local" || stage == "delegate_remote":
		target := strings.TrimSpace(event.Mode)
		if target == "" {
			target = strings.TrimSpace(event.Target)
		}
		body = fmt.Sprintf("↪  %s → %s", event.Agent, target)
	case stage == "plan_presented":
		return ""
	case stage == "step_progress", stage == "agent_progress":
		body = strings.TrimSpace(event.Summary)
		if body == "" {
			body = firstCLIProgressLine(event.ToolOutput)
		}
		if body == "" {
			return ""
		}
		body = "✓  " + body
	case strings.HasPrefix(stage, "tool_"):
		tool := strings.TrimSpace(event.ToolName)
		if tool == "" {
			tool = strings.TrimPrefix(stage, "tool_")
		}
		target := formatCLIToolTarget(event)
		if errMsg := strings.TrimSpace(event.Error); errMsg != "" {
			body = fmt.Sprintf("✕  %s", tool)
			if target != "" {
				body += "  " + target
			}
			if len([]rune(errMsg)) > 72 {
				errMsg = string([]rune(errMsg)[:72]) + "…"
			}
			body += "  " + errMsg
		} else {
			body = fmt.Sprintf("⏺  %s", tool)
			if target != "" {
				body += "  " + target
			}
		}
	case stage == "context_compress":
		body = "…  compressing conversation context"
	case stage == "message_failed", stage == "task_failed", stage == "verification_failed":
		body = "✕  " + strings.TrimSpace(event.Error)
	default:
		label := stage
		if s := strings.TrimSpace(event.Summary); s != "" {
			body = label + "  " + s
		} else {
			body = label
		}
	}

	if !color {
		return prefix + body
	}
	switch {
	case strings.HasPrefix(body, "↪"):
		return prefix + "\033[35m" + body + public.AnsiReset
	case strings.HasPrefix(body, "⏺"):
		return prefix + "\033[33m" + body + public.AnsiReset
	case strings.HasPrefix(body, "✕"):
		return prefix + "\033[31m" + body + public.AnsiReset
	case strings.HasPrefix(body, "✓"):
		return prefix + "\033[32m" + body + public.AnsiReset
	default:
		return prefix + "\033[2m" + body + public.AnsiReset
	}
}

func renderCLIBlocks(u *cliUI, indent, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		u.println(u.dim(indent + "(empty)"))
		return
	}

	cursor := 0
	matches := codeFenceRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		writeCLIPlain(u, indent, text)
		return
	}

	for _, m := range matches {
		if m[0] > cursor {
			writeCLIPlain(u, indent, text[cursor:m[0]])
		}
		lang := strings.TrimSpace(text[m[2]:m[3]])
		code := strings.TrimRight(text[m[4]:m[5]], "\n")
		writeCLICodeBlock(u, indent, lang, code)
		cursor = m[1]
	}
	if cursor < len(text) {
		writeCLIPlain(u, indent, text[cursor:])
	}
}

func writeCLIPlain(u *cliUI, indent, text string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		u.println(indent + renderCLIInlineCode(u, line))
	}
}

var inlineCodeRE = regexp.MustCompile("`([^`]+)`")

func renderCLIInlineCode(u *cliUI, line string) string {
	if !strings.Contains(line, "`") {
		return line
	}
	var b strings.Builder
	last := 0
	for _, loc := range inlineCodeRE.FindAllStringSubmatchIndex(line, -1) {
		b.WriteString(line[last:loc[0]])
		b.WriteString(u.cyan(line[loc[2]:loc[3]]))
		last = loc[1]
	}
	b.WriteString(line[last:])
	return b.String()
}

func writeCLICodeBlock(u *cliUI, indent, lang, code string) {
	label := lang
	if label == "" {
		label = "code"
	}
	border := strings.Repeat("─", max(24, len(label)+8))
	u.println(u.dim(indent + "┌─ " + label + " " + border))
	for _, line := range strings.Split(code, "\n") {
		u.println(u.blue(indent+"│ ") + line)
	}
	u.println(u.dim(indent + "└" + strings.Repeat("─", len(border)+4)))
}

type cliSpinner struct {
	ui       *cliUI
	label    string
	stopCh   chan struct{}
	done     chan struct{}
	frames   []string
	frozen   bool
	haltOnce sync.Once
}

func (u *cliUI) startSpinner(label string) *cliSpinner {
	s := &cliSpinner{
		ui:     u,
		label:  label,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
	u.mu.Lock()
	u.activeSpinner = s
	u.mu.Unlock()
	go s.run()
	return s
}

func (u *cliUI) freezeSpinner() {
	u.mu.Lock()
	s := u.activeSpinner
	u.mu.Unlock()
	if s != nil {
		s.freeze()
	}
}

func (u *cliUI) unfreezeSpinner() {
	u.mu.Lock()
	s := u.activeSpinner
	u.mu.Unlock()
	if s != nil {
		s.unfreeze()
	}
}

func (s *cliSpinner) freeze() {
	s.ui.mu.Lock()
	s.frozen = true
	s.ui.mu.Unlock()
	s.clear()
}

func (s *cliSpinner) unfreeze() {
	s.ui.mu.Lock()
	s.frozen = false
	s.ui.mu.Unlock()
}

func (s *cliSpinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(90 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.stopCh:
			s.clear()
			return
		case <-ticker.C:
			s.ui.mu.Lock()
			frozen := s.frozen
			s.ui.mu.Unlock()
			if frozen {
				continue
			}
			frame := s.frames[i%len(s.frames)]
			i++
			s.draw(frame)
		}
	}
}

func (s *cliSpinner) draw(frame string) {
	s.ui.mu.Lock()
	defer s.ui.mu.Unlock()
	text := fmt.Sprintf("  %s  %s", frame, s.ui.yellow(s.label))
	if s.ui.color {
		fmt.Fprintf(s.ui.out, "\r\033[K%s", text)
	} else {
		fmt.Fprintf(s.ui.out, "\r%s", text)
	}
}

func (s *cliSpinner) clear() {
	s.ui.mu.Lock()
	defer s.ui.mu.Unlock()
	if s.ui.color {
		fmt.Fprint(s.ui.out, "\r\033[K")
	} else {
		fmt.Fprint(s.ui.out, "\r")
	}
}

func (s *cliSpinner) note(line string) {
	s.ui.mu.Lock()
	defer s.ui.mu.Unlock()
	if s.ui.color {
		fmt.Fprint(s.ui.out, "\r\033[K")
	} else {
		fmt.Fprint(s.ui.out, "\r")
	}
	fmt.Fprintln(s.ui.out, line)
}

func (s *cliSpinner) halt() {
	s.haltOnce.Do(func() {
		close(s.stopCh)
		<-s.done
		s.ui.mu.Lock()
		if s.ui.activeSpinner == s {
			s.ui.activeSpinner = nil
		}
		s.ui.mu.Unlock()
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstCLIProgressLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "##") {
			continue
		}
		if len([]rune(line)) > 96 {
			return string([]rune(line)[:96]) + "…"
		}
		return line
	}
	return ""
}
