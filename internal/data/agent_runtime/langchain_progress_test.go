package agent

import (
	"context"
	"testing"

	lmm "kratos-demo/internal/biz/llm"
	dattrace "kratos-demo/internal/data/trace"
)

func TestClassifyAgentProgressStagePlan(t *testing.T) {
	text := "## 执行计划\n1. 读取 README\n2. 编写脚本"
	if stage := classifyAgentProgressStage(text, true); stage != "plan_presented" {
		t.Fatalf("stage = %q, want plan_presented", stage)
	}
}

func TestClassifyAgentProgressStageStep(t *testing.T) {
	text := "✓ 步骤 1 完成：已创建 hello.ps1"
	if stage := classifyAgentProgressStage(text, false); stage != "step_progress" {
		t.Fatalf("stage = %q, want step_progress", stage)
	}
}

func TestPublishAgentProgressSuppressesGenericToolPreamble(t *testing.T) {
	trace := dattrace.NewDelegationTraceStore()
	runtime := &langChainAgentRuntime{trace: trace}
	runtime.publishAgentProgress(context.Background(), "## 最终结果总结\n尚待工具执行", true)
	if sessions := trace.ListSessions(1); len(sessions) != 0 {
		t.Fatalf("generic preamble should not be published: %+v", sessions)
	}
}

func TestFirstMeaningfulLine(t *testing.T) {
	line := firstMeaningfulLine("\n\n## 执行计划\n1. 读取文件\n")
	if line != "## 执行计划" {
		t.Fatalf("line = %q", line)
	}
}

func TestToolCallTargetJSONPath(t *testing.T) {
	target := toolCallTarget(lmm.ToolCallPart{
		FunctionCall: &lmm.FunctionCall{
			Arguments: `{"path":"scripts/hello.ps1","content":"echo hi"}`,
		},
	})
	if target != "scripts/hello.ps1" {
		t.Fatalf("target = %q", target)
	}
}

func TestToolCallTargetPlainPath(t *testing.T) {
	target := toolCallTarget(lmm.ToolCallPart{
		FunctionCall: &lmm.FunctionCall{
			Arguments: `README.md`,
		},
	})
	if target != "README.md" {
		t.Fatalf("target = %q", target)
	}
}
