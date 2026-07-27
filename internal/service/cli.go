package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	dataagent "kratos-demo/internal/data/agent_runtime"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	dataauthz "kratos-demo/internal/data/authz"
	datatasking "kratos-demo/internal/data/tasking"
	datatrace "kratos-demo/internal/data/trace"

	"github.com/go-kratos/kratos/v2/log"
)

type CLIService struct {
	dash         *DashboardService
	trace        datatrace.DelegationTraceStore
	processLog   *CLIProcessLog
	outputStream *CLIOutputStream
	sessionID    string
	in           io.Reader
	out          io.Writer
	shouldExit   bool
	ui           *cliUI
	lineReader   *cliLineReader

	approvalBridge chan approvalRequest

	permMode PermissionMode
	permMu   sync.Mutex

	aiConfig *conf.AI
	security *conf.Security
}

type approvalRequest struct {
	action agentctx.RiskAction
	resp   chan approvalResponse
}

type approvalResponse struct {
	allowed bool
	err     error
}

func NewCLIService(
	dash *DashboardService,
	repo datatasking.TaskRepo,
	runtime biz.AgentRuntime,
	trace datatrace.DelegationTraceStore,
	memory dataagent.AgentMemory,
	security *conf.Security,
	logger log.Logger,
) *CLIService {
	_ = datatasking.NewTaskDispatcher(repo, runtime, trace, memory, logger)
	return &CLIService{
		dash:         dash,
		trace:        trace,
		sessionID:    fmt.Sprintf("cli-%d", time.Now().Unix()),
		in:           os.Stdin,
		out:          os.Stdout,
		permMode:     PermAsk,
		outputStream: NewCLIOutputStream(),
		security:     security,
	}
}

func (c *CLIService) OutputStream() *CLIOutputStream {
	if c == nil {
		return nil
	}
	return c.outputStream
}

func (c *CLIService) BindSession(sessionID string, processLog *CLIProcessLog) {
	if strings.TrimSpace(sessionID) != "" {
		c.sessionID = strings.TrimSpace(sessionID)
	}
	c.processLog = processLog
}

func (c *CLIService) Run(ctx context.Context) error {
	if c == nil || c.dash == nil {
		return errors.New("cli service is not available")
	}
	c.ui = newCLIUI(c.out)

	reader, err := newCLILineReader(c.in, c.ui, c.onShiftTabMode)
	if err != nil {
		return err
	}
	defer reader.Close()
	c.lineReader = reader

	if err := c.ensureAIConfig(); err != nil {
		return err
	}

	c.dash.StartAllAgents()
	c.ui.printWelcome(c.sessionID, c.processLogPath())

	idleCtrlC := 0
	for {
		line, err := reader.ReadLine(c.ui, c.permissionMode())
		if err != nil {
			if err == io.EOF {
				c.ui.println("")
				return nil
			}
			if errors.Is(err, readline.ErrInterrupt) {
				idleCtrlC++
				if idleCtrlC >= 2 {
					c.ui.println(c.ui.dim("  goodbye."))
					return nil
				}
				c.ui.println(c.ui.dim("  按 Ctrl+C 再次退出 · 输入 /exit 也可退出"))
				continue
			}
			return err
		}
		idleCtrlC = 0

		if line == "" {
			continue
		}
		if c.handleCommand(line) {
			if c.shouldExit {
				c.ui.println(c.ui.dim("  goodbye."))
				return nil
			}
			continue
		}

		c.processTurn(ctx, line)
		if c.shouldExit {
			return nil
		}
	}
}

func (c *CLIService) processLogPath() string {
	if c.processLog == nil {
		return ""
	}
	return c.processLog.Path()
}

func (c *CLIService) processTurn(ctx context.Context, prompt string) {
	c.publishOutput("cli.turn.started", prompt)
	c.ui.printUserTurn(prompt)
	if c.processLog != nil {
		c.processLog.LogTurnStart(c.sessionID, prompt)
	}

	turnCtx, cancelTurn := context.WithCancel(ctx)
	defer cancelTurn()

	turnCtx = c.contextWithApproval(turnCtx)
	if c.security != nil && c.security.GetEnabled() {
		authorizer, err := dataauthz.NewCasbinFileAuthorizer(c.security)
		if err != nil {
			c.ui.printError(fmt.Errorf("load RBAC policy: %w", err))
			return
		}
		principal, err := authorizer.LocalPrincipal(c.security.GetLocalPrincipal())
		if err != nil {
			c.ui.printError(err)
			return
		}
		turnCtx = agentctx.WithPrincipal(turnCtx, principal)
	}

	c.approvalBridge = make(chan approvalRequest)
	defer func() { c.approvalBridge = nil }()

	baseline := c.sessionEventCount(c.sessionID)
	var (
		result *taskv1.TaskResult
		runErr error
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, runErr = c.dash.runConversation(turnCtx, c.sessionID, public.AgentKindRouter, prompt)
	}()

	stopWatch := make(chan struct{})
	watchDone := make(chan int, 1)
	go func() { watchDone <- c.watchSessionEvents(turnCtx, c.sessionID, baseline, stopWatch) }()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	spinner := c.ui.startSpinner("router thinking…")
	turnCancelled := false
	spinnerStopped := false
	stopSpinner := func() {
		if spinnerStopped {
			return
		}
		spinnerStopped = true
		spinner.halt()
	}
	for {
		select {
		case req := <-c.approvalBridge:
			spinner.freeze()
			allowed, err := c.handleApprovalRequest(req.action)
			req.resp <- approvalResponse{allowed: allowed, err: err}
			spinner.unfreeze()
		case <-sigCh:
			if !turnCancelled {
				turnCancelled = true
				cancelTurn()
				stopSpinner()
				c.ui.println(c.ui.dim("  已停止生成（Ctrl+C）· 再次 Ctrl+C 退出"))
				continue
			}
			c.shouldExit = true
			cancelTurn()
			stopSpinner()
			close(stopWatch)
			<-watchDone
			c.ui.println(c.ui.dim("  goodbye."))
			return
		case <-done:
			stopSpinner()
			close(stopWatch)
			lastDisplayed := <-watchDone
			c.flushNewEvents(c.sessionID, lastDisplayed)
			goto turnDone
		}
	}
turnDone:

	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(turnCtx.Err(), context.Canceled) {
			return
		}
		if c.processLog != nil {
			c.processLog.LogTurnDone(c.sessionID, "", "", runErr)
		}
		c.ui.printError(runErr)
		return
	}
	if c.processLog != nil {
		summary, output := "", ""
		if result != nil {
			summary = result.GetSummary()
			output = result.GetOutput()
		}
		c.processLog.LogTurnDone(c.sessionID, summary, output, nil)
	}
	if runErr != nil {
		c.publishOutput("cli.turn.failed", runErr.Error())
	} else if result != nil {
		c.publishOutput("cli.turn.completed", result.GetOutput())
	}
	c.printResult(result)
}

func (c *CLIService) watchSessionEvents(ctx context.Context, sessionID string, baseline int, stop <-chan struct{}) int {
	subscriber, ok := c.trace.(datatrace.DelegationTraceSubscriber)
	if !ok {
		return baseline
	}
	ch, cancel := subscriber.Subscribe()
	defer cancel()

	lastLogged := baseline
	lastDisplayed := baseline
	for {
		select {
		case <-ctx.Done():
			return lastDisplayed
		case <-stop:
			return lastDisplayed
		case sessions, ok := <-ch:
			if !ok {
				return lastDisplayed
			}
			lastLogged, lastDisplayed = c.processSessionEvents(sessions, sessionID, lastLogged, lastDisplayed)
		}
	}
}

func (c *CLIService) flushNewEvents(sessionID string, from int) {
	if c.trace == nil {
		return
	}
	_, _ = c.processSessionEvents(c.trace.ListSessions(8), sessionID, from, from)
}

func (c *CLIService) processSessionEvents(sessions []datatrace.DelegationSession, sessionID string, logFrom, displayFrom int) (logged, displayed int) {
	logged = logFrom
	displayed = displayFrom
	for _, session := range sessions {
		if session.TaskID != sessionID {
			continue
		}
		if logFrom > len(session.Events) {
			logFrom = len(session.Events)
		}
		if displayFrom > len(session.Events) {
			displayFrom = len(session.Events)
		}
		from := logFrom
		if displayFrom < from {
			from = displayFrom
		}
		for i := from; i < len(session.Events); i++ {
			event := session.Events[i]
			if i >= logFrom && c.processLog != nil {
				c.processLog.LogEvent(event)
			}
			if i >= displayFrom {
				c.displayEvent(event)
			}
		}
		return len(session.Events), len(session.Events)
	}
	return logged, displayed
}

func (c *CLIService) displayEvent(event datatrace.DelegationEvent) {
	c.publishOutput("cli.trace", formatCLIEventLogLine(event))
	if c == nil || c.ui == nil {
		return
	}
	switch strings.TrimSpace(event.Stage) {
	case "plan_presented":
		c.ui.printPlanPresented(event)
		return
	}
	if line := formatCLIEventLine(event, c.ui.color); line != "" {
		c.ui.printProgressLine(line)
	}
}

func (c *CLIService) publishOutput(eventType, text string) {
	if c == nil || c.outputStream == nil {
		return
	}
	c.outputStream.Publish(CLIOutputEvent{SessionID: c.sessionID, Type: eventType, Text: strings.TrimSpace(text)})
}

func (c *CLIService) sessionEventCount(sessionID string) int {
	if c.trace == nil {
		return 0
	}
	for _, session := range c.trace.ListSessions(8) {
		if session.TaskID == sessionID {
			return len(session.Events)
		}
	}
	return 0
}

func (c *CLIService) handleCommand(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "/exit", "/quit", "exit", "quit":
		c.shouldExit = true
		return true
	case "/help", "/h", "help":
		c.ui.printHelp()
		return true
	case "/agents":
		c.ui.printAgents(c.dash.agentStates())
		return true
	case "/session":
		c.ui.println(c.ui.dim("  session  ") + c.sessionID)
		return true
	case "/new":
		c.sessionID = fmt.Sprintf("cli-%d", time.Now().Unix())
		if c.processLog != nil {
			_ = c.processLog.Close()
			if next, err := NewCLIProcessLog(c.sessionID); err == nil {
				c.processLog = next
			}
		}
		c.ui.println(c.ui.dim("  new session  ") + c.sessionID)
		if path := c.processLogPath(); path != "" {
			c.ui.println(c.ui.dim("  logs    ") + path)
		}
		return true
	case "/mode":
		c.onShiftTabMode()
		return true
	case "/model":
		c.showModelSelect()
		return true
	default:
		if c.handleConfigCommand(line) {
			return true
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "/model ") {
			c.handleModelSwitch(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(line)), "/model "))
			return true
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "/mode ") {
			arg := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(line)), "/mode"))
			switch arg {
			case "ask":
				c.setPermissionMode(PermAsk)
			case "agent":
				c.setPermissionMode(PermAgent)
			case "auto":
				c.setPermissionMode(PermAuto)
			default:
				c.ui.println(c.ui.dim("  用法: /mode ask|agent|auto"))
				return true
			}
			c.refreshPermissionPrompt()
			c.ui.printPermissionMode(c.permissionMode())
			return true
		}
		return false
	}
}

func (c *CLIService) onShiftTabMode() {
	mode := c.cyclePermissionMode()
	c.refreshPermissionPrompt()
	c.ui.printPermissionMode(mode)
}

func (c *CLIService) refreshPermissionPrompt() {
	if c.lineReader != nil {
		c.lineReader.SetPrompt(c.ui, c.permissionMode())
	}
}

func (c *CLIService) printResult(result interface {
	GetSummary() string
	GetOutput() string
}) {
	if result == nil {
		c.ui.printAssistant("router", "", "(no response)")
		return
	}
	c.ui.printAssistant("router", result.GetSummary(), result.GetOutput())
}
