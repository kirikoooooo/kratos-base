// Package public 集中管理项目中所有非错误的通用常量。
package public

import "time"

// ---- Agent 身份类型 ----

// AgentKind 标识特定 Agent 角色/能力。
type AgentKind string

const (
	AgentKindDefault  AgentKind = "default"
	AgentKindGeneric  AgentKind = "generic"
	AgentKindRouter   AgentKind = "router"
	AgentKindCoder    AgentKind = "coder"
	AgentKindReviewer AgentKind = "reviewer"
)

// ---- 运行时操作参数 ----

const (
	DefaultRemoteTimeout    = 8 * time.Second // 远程 Agent gRPC 调用默认超时
	MaxToolLoopIterations   = 12              // 单次对话最大工具循环次数
	MaxToolCorrectionRounds = 5               // 工具纠错最大轮数
	MaxExecCommandAttempts  = 3               // exec_command 单任务最大调用次数
	RuntimeActorPID         = 9001            // 运行时 actor 进程 ID
	RuntimeMailboxSize      = 128             // actor mailbox 缓冲大小
)

// ---- 提示词 ----

// FailureAnalysisInstruction 是工具耗尽后的失败分析提示词。
const FailureAnalysisInstruction = `【系统通知】当前任务因工具多次失败或流程中断，无法继续自动执行。请不要再调用任何工具，直接向用户输出中文失败分析，必须包含：
1. **任务目标**：简要复述用户要什么
2. **已尝试的操作**：调用了哪些工具、每次失败的具体 error 是什么
3. **根因**：为什么会出现这些错误（例如绝对路径、JSON 转义、文件不存在等）
4. **建议**：用户应如何改写指令、提供什么信息，或改用什么相对路径/命令才能完成

语气清晰、具体，不要输出占位语或空内容。`

// ---- 任务状态 ----

// TaskStatus 表示任务的执行状态。
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
)

// ---- Provider 厂商 ----

const (
	ProviderOpenAI   = "openai"
	ProviderDeepSeek = "deepseek"
	ProviderGemini   = "gemini"
	ProviderGrok     = "grok"
	ProviderClaude   = "claude"
	ProviderOpenRouter = "openrouter"
)

const (
	OpenAIDefaultModel   = "gpt-4o-mini"
	DeepSeekDefaultModel = "deepseek-v4-flash"
	GeminiDefaultModel   = "gemini-2.5-flash"
	GrokDefaultModel     = "grok-3-mini"
	ClaudeDefaultModel   = "claude-sonnet-4-5"
)

const (
	DeepSeekDefaultBaseURL    = "https://api.deepseek.com"
	GeminiDefaultBaseURL      = "https://generativelanguage.googleapis.com/v1beta/openai/"
	GrokDefaultBaseURL        = "https://api.x.ai/v1"
	ClaudeDefaultBaseURL      = "https://api.anthropic.com/v1"
	OpenRouterDefaultBaseURL  = "https://openrouter.ai/api/v1"
	DefaultCLIOpenAIBaseURL   = "https://api.openai-proxy.org/v1"
	DefaultCLIDeepSeekBaseURL = "https://api.deepseek.com"
)

const DefaultTimeout = 120 * time.Second

// ---- CLI 权限模式 ----

// PermissionMode 控制 CLI 中风险工具操作的审批方式。
type PermissionMode int

const (
	PermAsk PermissionMode = iota
	PermAgent
	PermAuto
)

// String 返回权限模式的字符串表示。
func (m PermissionMode) String() string {
	switch m {
	case PermAsk:
		return "ask"
	case PermAgent:
		return "agent"
	case PermAuto:
		return "auto"
	default:
		return "ask"
	}
}

// Label 返回权限模式的展示标签。
func (m PermissionMode) Label() string {
	switch m {
	case PermAsk:
		return "Ask"
	case PermAgent:
		return "Agent"
	case PermAuto:
		return "Auto"
	default:
		return "Ask"
	}
}

// Description 返回权限模式的说明文案。
func (m PermissionMode) Description() string {
	switch m {
	case PermAsk:
		return "删除文件 / 高风险命令均需确认"
	case PermAgent:
		return "文件操作自动批准，高风险 shell 命令仍需确认"
	case PermAuto:
		return "全部自动批准（无确认弹窗）"
	default:
		return ""
	}
}

// Next 返回下一个权限模式（循环切换）。
func (m PermissionMode) Next() PermissionMode {
	switch m {
	case PermAsk:
		return PermAgent
	case PermAgent:
		return PermAuto
	default:
		return PermAsk
	}
}

// ---- 终端按键 ----

// TermKey 标识终端特殊按键。
type TermKey int

const (
	KeyUnknown TermKey = iota
	KeyUp
	KeyDown
	KeyEnter
	KeyEscape
	KeyCtrlC
)

// ---- ANSI 控制码 ----

const (
	AnsiReset   = "\033[0m"
	AnsiDim     = "\033[2m"
	AnsiBold    = "\033[1m"
	AnsiCyan    = "\033[36m"
	AnsiGreen   = "\033[32m"
	AnsiYellow  = "\033[33m"
	AnsiMagenta = "\033[35m"
	AnsiBlue    = "\033[34m"
	AnsiRed     = "\033[31m"
)

// ---- 记忆与持久化 ----

const (
	DefaultMemoryDir    = ".myagent"
	DefaultMemoryUserID = "default"
)

const (
	MaxSessionErrorMessage       = 2000
	MaxSessionErrorDetail        = 4000
	DefaultSessionErrorsInPrompt = 8
)

const MaxTraceSessions = 32
