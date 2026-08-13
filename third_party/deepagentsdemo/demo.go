// Package deepagentsdemo 演示 Deep Agents 风格的多 Agent 编排（基于 go-deepagent）。
//
// 对标 LangChain 官方 deepagents 概念：
//   - Orchestrator 主 Agent 持有 task 工具，按 Description 路由到 SubAgent
//   - 每个 SubAgent 独立上下文窗口与 ReAct 循环
//   - 内置 VFS 工具（ls/read_file/write_file/...）由库自动注入
//
// 上游库源码（建议阅读顺序）：
//   1. pkg/agent/app.go       — App.New，注入 task 工具与 VFS
//   2. pkg/agent/tasktool.go  — task(description, subagent_type) 委派逻辑
//   3. pkg/agent/react.go     — ReAct 循环与 Event 流
//   4. pkg/agent/governance.go — 系统 Prompt 与规划指令
package deepagentsdemo

import (
	"fmt"
	"os"
	"strings"

	deepagent "github.com/denizumutdereli/go-deepagent/pkg/agent"
)

// DemoOptions 运行参数。
type DemoOptions struct {
	OpenAIAPIKey string // 默认读 OPENAI_API_KEY
	Model        string // 主 Agent 模型，默认 gpt-4.1-mini
	MaxIter      int    // ReAct 最大轮次，默认 12
}

func (o DemoOptions) withDefaults() DemoOptions {
	out := o
	if strings.TrimSpace(out.OpenAIAPIKey) == "" {
		out.OpenAIAPIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if out.Model == "" {
		out.Model = "gpt-4.1-mini"
	}
	if out.MaxIter <= 0 {
		out.MaxIter = 12
	}
	return out
}

// MultiAgentConfig 返回 Deep Agents 多 Agent 演示配置。
//
// Agent 树结构：
//
//	orchestrator
//	├── researcher  — 检索/归纳（可接 web search 工具）
//	├── coder       — 代码设计与实现建议
//	└── reviewer    — 方案评审与风险点
func MultiAgentConfig(opts DemoOptions) deepagent.AgentConfig {
	opts = opts.withDefaults()
	return deepagent.AgentConfig{
		Name:  "orchestrator",
		Model: opts.Model,
		// 主 Agent 只负责拆解与委派，不直接写大段答案
		Prompt: strings.TrimSpace(`
你是多 Agent 编排器（Deep Agents orchestrator）。
规则：
1. 复杂任务必须调用 task 工具委派给合适的 sub-agent，不要自己包办。
2. 委派时在 description 里写清背景、约束与期望输出格式。
3. 收到 sub-agent 结果后，整合成简洁中文回复用户。
4. 可用 sub-agent：researcher（调研归纳）、coder（代码方案）、reviewer（评审挑错）。
`),
		MaxIter: opts.MaxIter,
		SubAgents: []deepagent.AgentConfig{
			{
				Name:        "researcher",
				Description: "检索资料、阅读文档、归纳要点，适合背景调研与事实整理",
				Model:       opts.Model,
				MaxIter:     opts.MaxIter,
				Prompt:      "你是研究员。根据任务描述给出结构化要点，标注不确定性，回答简洁。",
			},
			{
				Name:        "coder",
				Description: "编写/审查 Go 代码、设计 API、给出实现步骤与示例",
				Model:       opts.Model,
				MaxIter:     opts.MaxIter,
				Prompt:      "你是 Go 工程师。输出可运行的代码片段与文件路径建议，说明关键设计取舍。",
			},
			{
				Name:        "reviewer",
				Description: "评审方案与代码，列出风险、边界条件与测试建议",
				Model:       opts.Model,
				MaxIter:     opts.MaxIter,
				Prompt:      "你是Reviewer。用 checklist 形式指出问题，区分 blocker 与建议。",
			},
		},
	}
}

// NewApp 创建多 Agent 应用。SubAgent 非空时库会自动注入 task 工具。
func NewApp(opts DemoOptions) (*deepagent.App, error) {
	opts = opts.withDefaults()
	if opts.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}
	return deepagent.New(MultiAgentConfig(opts), opts.OpenAIAPIKey)
}

// AgentTree 返回可读的多 Agent 树形结构，便于 dry-run 查看配置。
func AgentTree(cfg deepagent.AgentConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", cfg.Name, cfg.Model)
	for _, sub := range cfg.SubAgents {
		fmt.Fprintf(&b, "  └─ %s (%s)\n", sub.Name, sub.Model)
		fmt.Fprintf(&b, "       %s\n", sub.Description)
	}
	fmt.Fprintf(&b, "\n自动注入工具: task, ls, read_file, write_file, edit_file, grep, glob\n")
	return b.String()
}
