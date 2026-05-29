package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	datatasking "kratos-demo/internal/data/tasking"
	dataagent "kratos-demo/internal/data/agent"
	datatrace "kratos-demo/internal/data/trace"

	"github.com/go-kratos/kratos/v2/log"
)

type CLIService struct {
	dash       *DashboardService
	trace      datatrace.DelegationTraceStore
	processLog *CLIProcessLog
	sessionID  string
	in         io.Reader
	out        io.Writer
	shouldExit bool
	ui         *cliUI
}

func NewCLIService(
	dash *DashboardService,
	repo datatasking.TaskRepo,
	runtime biz.AgentRuntime,
	trace datatrace.DelegationTraceStore,
	memory dataagent.AgentMemory,
	logger log.Logger,
) *CLIService {
	_ = datatasking.NewTaskDispatcher(repo, runtime, trace, memory, logger)
	return &CLIService{
		dash:      dash,
		trace:     trace,
		sessionID: fmt.Sprintf("cli-%d", time.Now().Unix()),
		in:        os.Stdin,
		out:       os.Stdout,
	}
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
	c.dash.StartAllAgents()
	c.ui.printWelcome(c.sessionID, c.processLogPath())

	scanner := bufio.NewScanner(c.in)
	for {
		c.ui.printPrompt()
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			c.ui.println("")
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
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
	}
}

func (c *CLIService) processLogPath() string {
	if c.processLog == nil {
		return ""
	}
	return c.processLog.Path()
}

func (c *CLIService) processTurn(ctx context.Context, prompt string) {
	c.ui.printUserTurn(prompt)
	if c.processLog != nil {
		c.processLog.LogTurnStart(c.sessionID, prompt)
	}

	baseline := c.sessionEventCount(c.sessionID)
	var (
		result *taskv1.TaskResult
		runErr error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, runErr = c.dash.runConversation(ctx, c.sessionID, biz.AgentRouter, prompt)
	}()

	stopWatch := make(chan struct{})
	go c.watchSessionEvents(ctx, c.sessionID, baseline, stopWatch)

	spinner := c.ui.startSpinner("router thinking…")
	wg.Wait()
	close(stopWatch)
	spinner.halt()

	c.flushNewEvents(c.sessionID, baseline)

	if runErr != nil {
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
	c.printResult(result)
}

func (c *CLIService) watchSessionEvents(ctx context.Context, sessionID string, baseline int, stop <-chan struct{}) {
	subscriber, ok := c.trace.(datatrace.DelegationTraceSubscriber)
	if !ok {
		return
	}
	ch, cancel := subscriber.Subscribe()
	defer cancel()

	lastLogged := baseline
	lastDisplayed := baseline
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case sessions, ok := <-ch:
			if !ok {
				return
			}
			lastLogged, lastDisplayed = c.processSessionEvents(sessions, sessionID, lastLogged, lastDisplayed)
		}
	}
}

func (c *CLIService) flushNewEvents(sessionID string, baseline int) {
	if c.trace == nil {
		return
	}
	_, _ = c.processSessionEvents(c.trace.ListSessions(8), sessionID, baseline, baseline)
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
	default:
		return false
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
