#!/usr/bin/env python3
"""Restructure internal/data/agent into memory/file/tool/context subpackages."""

from __future__ import annotations

import re
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
AGENT = ROOT / "internal/data/agent"

CONTEXT_TYPES = """package context

import (
	"time"

	datatrace "kratos-demo/internal/data/trace"
)

type ConversationRole string

const (
	ConversationRoleHuman ConversationRole = "human"
	ConversationRoleAI    ConversationRole = "ai"
	ConversationRoleTool  ConversationRole = "tool"
)

type ConversationToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type ConversationTurn struct {
	Role       ConversationRole       `json:"role"`
	Content    string                 `json:"content,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolCalls  []ConversationToolCall `json:"tool_calls,omitempty"`
}

type PrepareMeta struct {
	Stats    CompressStats
	Compress *datatrace.ContextCompressResult
}

const (
	DefaultCompressThreshold = 200_000
	DefaultKeepRecentTurns   = 24
	DefaultToolOutputMaxChars = 8_000
)

type CompressConfig struct {
	Threshold          int
	KeepRecentTurns    int
	ToolOutputMaxChars int
}

type CompressStats struct {
	EstimatedChars int
	Threshold      int
	NeedsCompress  bool
}

func ConfigFromMemory(threshold, keepRecent, toolOutputMax int) CompressConfig {
	return CompressConfig{
		Threshold:          threshold,
		KeepRecentTurns:    keepRecent,
		ToolOutputMaxChars: toolOutputMax,
	}
}
"""

MEMORY_TYPES = """package memory

import (
	"context"
	"strings"
	"time"

	"kratos-demo/internal/biz"
	agentcontext "kratos-demo/internal/data/agent/context"
	datatrace "kratos-demo/internal/data/trace"
)

type AgentMemoryStore interface {
	LoadUser(ctx context.Context, userID string) (*UserAgentMemory, error)
	SaveUser(ctx context.Context, memory *UserAgentMemory) error
	LoadSession(ctx context.Context, sessionID string) (*SessionAgentMemory, error)
	SaveSession(ctx context.Context, memory *SessionAgentMemory) error
	LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error)
	SaveConversation(ctx context.Context, memory *SessionConversation) error
	AppendSessionError(ctx context.Context, record SessionErrorRecord) error
	ListSessionErrors(ctx context.Context, sessionID string, limit int) ([]SessionErrorRecord, error)
}

type AgentMemory interface {
	UserID() string
	StartConversation(ctx context.Context, sessionID string, agent biz.Agent, initialPrompt string) error
	LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error)
	SaveConversation(ctx context.Context, conv *SessionConversation) error
	ConversationContextStats(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) agentcontext.CompressStats
	PrepareTurnsForLLM(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) []agentcontext.ConversationTurn
	PrepareTurnsForLLMWithMeta(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) ([]agentcontext.ConversationTurn, agentcontext.PrepareMeta)
	ConversationContextUsage(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) datatrace.ContextUsageSnapshot
	ConversationPreview(ctx context.Context, sessionID string) string
	PrepareForTask(ctx context.Context, sessionID string, agent biz.Agent) error
	RenderPromptContext(ctx context.Context, sessionID string) string
	RecordSessionError(ctx context.Context, record SessionErrorRecord) error
	RecordSessionToolUsage(ctx context.Context, sessionID, toolName string) error
	UpsertUserMemory(ctx context.Context, memory *UserAgentMemory) error
}

type SessionConversation struct {
	SessionID string                        `json:"session_id"`
	Agent     string                        `json:"agent,omitempty"`
	Turns     []agentcontext.ConversationTurn `json:"turns,omitempty"`
	UpdatedAt time.Time                     `json:"updated_at"`
}

type CommandPolicy struct {
	Situation string   `json:"situation"`
	Commands  []string `json:"commands,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

type ToolHint struct {
	Name        string `json:"name"`
	WhenToUse   string `json:"when_to_use"`
	Constraints string `json:"constraints,omitempty"`
}

type SkillHint struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	WhenToUse   string `json:"when_to_use"`
	Description string `json:"description,omitempty"`
}

type PromptAdjustment struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type UserAgentMemory struct {
	UserID          string             `json:"user_id"`
	WorkspaceRoot   string             `json:"workspace_root,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	SkillHints      []SkillHint        `json:"skill_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type SessionAgentMemory struct {
	SessionID       string             `json:"session_id"`
	Agent           string             `json:"agent,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	ToolsUsed       []string           `json:"tools_used,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type AgentMemoryConfig struct {
	Dir                      string
	UserID                   string
	ContextCompressThreshold int
	KeepRecentTurns          int
	ToolOutputMaxChars       int
}

func (c AgentMemoryConfig) NormalizedUserID() string {
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return "default"
	}
	return userID
}

type SessionErrorRecord struct {
	Time      time.Time `json:"time"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent,omitempty"`
	Stage     string    `json:"stage"`
	Tool      string    `json:"tool,omitempty"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail,omitempty"`
}
"""

EXPORTS = """package agent

import (
	"context"

	agentmemory "kratos-demo/internal/data/agent/memory"
)

type (
	AgentMemory       = agentmemory.AgentMemory
	AgentMemoryStore  = agentmemory.AgentMemoryStore
	AgentMemoryConfig = agentmemory.AgentMemoryConfig
)

var (
	NewAgentMemoryStore        = agentmemory.NewAgentMemoryStore
	NewAgentMemoryConfig       = agentmemory.NewAgentMemoryConfig
	NewAgentMemoryUsecase      = agentmemory.NewAgentMemoryUsecase
	BootstrapUserMemoryIfEmpty = agentmemory.BootstrapUserMemoryIfEmpty
)

func BootstrapUserMemoryIfEmptyAlias(ctx context.Context, store AgentMemoryStore, userID, workspace string) error {
	return agentmemory.BootstrapUserMemoryIfEmpty(ctx, store, userID, workspace)
}
"""

TURNS = """package context

import "strings"

func AppendHumanTurnIfNeeded(turns []ConversationTurn, prompt string) []ConversationTurn {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return turns
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role != ConversationRoleHuman {
			continue
		}
		if strings.TrimSpace(turns[i].Content) == prompt {
			return turns
		}
		break
	}
	return append(turns, ConversationTurn{
		Role:    ConversationRoleHuman,
		Content: prompt,
	})
}
"""


def rewrite(content: str, rules: list[tuple[str, str]]) -> str:
	for old, new in rules:
		content = content.replace(old, new)
	return content


def set_package(content: str, package: str) -> str:
	return re.sub(r"^package agent\s*$", f"package {package}", content, count=1, flags=re.M)


def move_with_rules(src: Path, dst: Path, package: str, rules: list[tuple[str, str]]) -> None:
	content = src.read_text(encoding="utf-8")
	content = set_package(content, package)
	content = rewrite(content, rules)
	dst.parent.mkdir(parents=True, exist_ok=True)
	dst.write_text(content, encoding="utf-8")


CONTEXT_RULES = [
	("ContextCompressConfig", "CompressConfig"),
	("ContextCompressStats", "CompressStats"),
	("ContextPrepareMeta", "PrepareMeta"),
	("DefaultContextCompressThreshold", "DefaultCompressThreshold"),
	("compressConversationTurns", "CompressConversationTurns"),
	("analyzeConversationContext", "AnalyzeConversationContext"),
	("newContextUsageSnapshot", "NewUsageSnapshot"),
	("formatContextCompressEventSummary", "FormatCompressEventSummary"),
	("formatContextCompressEventOutput", "FormatCompressEventOutput"),
	("conversationTurnsFromLLM", "TurnsFromLLM"),
	("llmMessagesFromConversationTurns", "LLMMessagesFromTurns"),
	("buildLLMMessages", "BuildLLMMessages"),
]

MEMORY_RULES = [
	("AgentMemory", "AgentMemory"),
	("compressConversationTurns", "agentcontext.CompressConversationTurns"),
	("analyzeConversationContext", "agentcontext.AnalyzeConversationContext"),
	("ContextCompressConfig", "agentcontext.CompressConfig"),
	("ContextCompressStats", "agentcontext.CompressStats"),
	("ContextPrepareMeta", "agentcontext.PrepareMeta"),
	("ConversationTurn", "agentcontext.ConversationTurn"),
	("ConversationRoleHuman", "agentcontext.ConversationRoleHuman"),
	("appendHumanTurnIfNeeded", "agentcontext.AppendHumanTurnIfNeeded"),
	("newContextUsageSnapshot", "agentcontext.NewUsageSnapshot"),
	("memoryCompressConfig", "agentcontext.ConfigFromMemory"),
	("package agent\n", "package memory\n"),
]

FILE_RULES = [
	("formatUnifiedDiff", "FormatUnifiedDiff"),
	("applySearchReplace", "ApplySearchReplace"),
	("applyReplaceLines", "ApplyReplaceLines"),
	("applyDeleteString", "ApplyDeleteString"),
	("applyAppendContent", "ApplyAppendContent"),
	("applyInsertLine", "ApplyInsertLine"),
	("applyInsertAfterLine", "ApplyInsertAfterLine"),
	("applyDeleteLine", "ApplyDeleteLine"),
	("applyDeleteLines", "ApplyDeleteLines"),
	("applyReplaceLine", "ApplyReplaceLine"),
	("applyPrepend", "ApplyPrepend"),
]

TOOL_RULES = [
	("localToolRuntime", "Runtime"),
	("newLocalToolRuntime", "NewRuntime"),
	("applySearchReplace", "agentfile.ApplySearchReplace"),
	("applyReplaceLines", "agentfile.ApplyReplaceLines"),
	("applyDeleteString", "agentfile.ApplyDeleteString"),
	("applyAppendContent", "agentfile.ApplyAppendContent"),
	("applyInsertLine", "agentfile.ApplyInsertLine"),
	("applyInsertAfterLine", "agentfile.ApplyInsertAfterLine"),
	("applyDeleteLine", "agentfile.ApplyDeleteLine"),
	("applyDeleteLines", "agentfile.ApplyDeleteLines"),
	("applyReplaceLine", "agentfile.ApplyReplaceLine"),
	("applyPrepend", "agentfile.ApplyPrepend"),
	('package agent\n', 'package tool\n'),
]

TEST_REMAP = {
	"conversation_compress_test.go": ("context/compress_test.go", "context", CONTEXT_RULES),
	"memory_llm_test.go": ("context/llm_test.go", "context", CONTEXT_RULES + [("conversationTurnsFromLLM", "TurnsFromLLM")]),
	"memory_helpers_test.go": ("memory/helpers_test.go", "memory", MEMORY_RULES),
	"memory_store_test.go": ("memory/store_test.go", "memory", MEMORY_RULES),
	"session_error_helpers_test.go": ("memory/session_error_test.go", "memory", MEMORY_RULES),
	"file_edit_ops_test.go": ("file/edit_ops_test.go", "file", FILE_RULES),
	"file_diff_test.go": ("file/diff_test.go", "file", FILE_RULES),
	"local_tools_change_test.go": ("tool/runtime_change_test.go", "tool", TOOL_RULES + [("newLocalToolRuntime", "NewRuntime")]),
	"local_tools_edit_test.go": ("tool/runtime_edit_test.go", "tool", TOOL_RULES + [("newLocalToolRuntime", "NewRuntime")]),
}


def strip_append_human_from_helpers(content: str) -> str:
	start = content.find("func appendHumanTurnIfNeeded")
	if start == -1:
		return content
	end = content.find("\nfunc formatAgentMemoryForPrompt", start)
	if end == -1:
		return content[:start]
	return content[:start] + content[end+1:]


def main() -> None:
	for sub in ("context", "memory", "file", "tool"):
		(AGENT / sub).mkdir(exist_ok=True)

	(AGENT / "context/types.go").write_text(CONTEXT_TYPES, encoding="utf-8")
	(AGENT / "memory/types.go").write_text(MEMORY_TYPES, encoding="utf-8")
	(AGENT / "context/turns.go").write_text(TURNS, encoding="utf-8")
	(AGENT / "export.go").write_text(
		EXPORTS.replace("BootstrapUserMemoryIfEmptyAlias", "BootstrapUserMemoryIfEmpty").replace(
			"func BootstrapUserMemoryIfEmpty(ctx context.Context, store AgentMemoryStore, userID, workspace string) error {\n\treturn agentmemory.BootstrapUserMemoryIfEmpty(ctx, store, userID, workspace)\n}\n",
			"",
		),
		encoding="utf-8",
	)

	move_with_rules(AGENT / "conversation_compress.go", AGENT / "context/compress.go", "context", CONTEXT_RULES)
	move_with_rules(AGENT / "context_usage_helpers.go", AGENT / "context/usage.go", "context", CONTEXT_RULES)
	move_with_rules(AGENT / "memory_llm.go", AGENT / "context/llm.go", "context", CONTEXT_RULES)

	helpers = (AGENT / "memory_helpers.go").read_text(encoding="utf-8")
	helpers = strip_append_human_from_helpers(helpers)
	helpers = set_package(helpers, "memory")
	helpers = rewrite(helpers, MEMORY_RULES)
	if "agentcontext" not in helpers and "kratos-demo/internal/data/agent/context" not in helpers:
		helpers = helpers.replace(
			"import (\n\t\"strings\"\n\n\t\"kratos-demo/internal/data/common\"\n)",
			"import (\n\t\"strings\"\n\n\t\"kratos-demo/internal/data/common\"\n\tagentcontext \"kratos-demo/internal/data/agent/context\"\n)",
		)
	(AGENT / "memory/helpers.go").write_text(helpers, encoding="utf-8")

	move_with_rules(AGENT / "agent_memory.go", AGENT / "memory/usecase.go", "memory", MEMORY_RULES)
	if "agentcontext" not in (AGENT / "memory/usecase.go").read_text(encoding="utf-8"):
		text = (AGENT / "memory/usecase.go").read_text(encoding="utf-8")
		text = text.replace(
			"datatrace \"kratos-demo/internal/data/trace\"\n)",
			"agentcontext \"kratos-demo/internal/data/agent/context\"\n\tdatatrace \"kratos-demo/internal/data/trace\"\n)",
		)
		(AGENT / "memory/usecase.go").write_text(text, encoding="utf-8")

	move_with_rules(AGENT / "memory_store.go", AGENT / "memory/store.go", "memory", MEMORY_RULES)
	move_with_rules(AGENT / "memory_bootstrap.go", AGENT / "memory/bootstrap.go", "memory", MEMORY_RULES)
	move_with_rules(AGENT / "session_error_helpers.go", AGENT / "memory/session_error.go", "memory", MEMORY_RULES)

	move_with_rules(AGENT / "file_edit_ops.go", AGENT / "file/edit_ops.go", "file", FILE_RULES)
	move_with_rules(AGENT / "file_diff.go", AGENT / "file/diff.go", "file", FILE_RULES)

	tool_content = (AGENT / "local_tools.go").read_text(encoding="utf-8")
	tool_content = set_package(tool_content, "tool")
	tool_content = rewrite(tool_content, TOOL_RULES)
	tool_content = tool_content.replace(
		"import (\n",
		"import (\n\tagentfile \"kratos-demo/internal/data/agent/file\"\n\t\"kratos-demo/internal/data/common\"\n",
		1,
	)
	tool_content = tool_content.replace(
		"func previewPrompt(prompt string) string {\n\treturn common.PreviewPrompt(prompt)\n}\n\n",
		"",
	)
	tool_content = tool_content.replace("previewPrompt(input)", "common.PreviewPrompt(input)")
	(AGENT / "tool/runtime.go").write_text(tool_content, encoding="utf-8")

	for src_name, (dst_rel, pkg, rules) in TEST_REMAP.items():
		src = AGENT / src_name
		if src.exists():
			move_with_rules(src, AGENT / dst_rel, pkg, rules)

	# remove migrated root files
	for name in [
		"memory_types.go", "conversation_compress.go", "context_usage_helpers.go", "memory_llm.go",
		"memory_helpers.go", "agent_memory.go", "memory_store.go", "memory_bootstrap.go",
		"session_error_helpers.go", "file_edit_ops.go", "file_diff.go", "local_tools.go",
		"conversation_compress_test.go", "memory_llm_test.go", "memory_helpers_test.go",
		"memory_store_test.go", "session_error_helpers_test.go", "file_edit_ops_test.go",
		"file_diff_test.go", "local_tools_change_test.go", "local_tools_edit_test.go",
	]:
		path = AGENT / name
		if path.exists():
			path.unlink()

	print("restructure_agent: subpackages created")


if __name__ == "__main__":
	main()
