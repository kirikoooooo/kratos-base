package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	deepagent "github.com/denizumutdereli/go-deepagent/pkg/agent"
	"kratos-demo/third_party/deepagentsdemo"
)

func main() {
	showTree := flag.Bool("show-tree", false, "仅打印多 Agent 配置树（无需 API Key）")
	model := flag.String("model", "gpt-4.1-mini", "主 Agent 模型")
	maxIter := flag.Int("max-iter", 12, "ReAct 最大迭代次数")
	timeout := flag.Duration("timeout", 3*time.Minute, "单次请求超时")
	flag.Parse()

	opts := deepagentsdemo.DemoOptions{Model: *model, MaxIter: *maxIter}
	cfg := deepagentsdemo.MultiAgentConfig(opts)

	if *showTree || flag.NArg() == 0 {
		fmt.Println("=== Deep Agents Multi-Agent Demo ===")
		fmt.Print(deepagentsdemo.AgentTree(cfg))
		if *showTree || flag.NArg() == 0 {
			fmt.Println("\n用法:")
			fmt.Println("  go run ./third_party/deepagentsdemo/cmd/deepagentsdemo --show-tree")
			fmt.Println("  OPENAI_API_KEY=sk-... go run ./third_party/deepagentsdemo/cmd/deepagentsdemo \"设计一个 RAG 入库 CLI 并让人评审\"")
			if flag.NArg() == 0 && !*showTree {
				os.Exit(0)
			}
			if *showTree {
				return
			}
		}
	}

	query := strings.Join(flag.Args(), " ")
	app, err := deepagentsdemo.NewApp(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	eventCh := make(chan deepagent.ReactEvent, 64)
	go deepagentsdemo.StreamEvents(os.Stderr, eventCh)

	result, err := app.ProcessWithHistoryAndEvents(ctx, query, nil, eventCh)
	close(eventCh)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("\n=== Answer ===")
	fmt.Println(result.Output)
	fmt.Fprintf(os.Stderr, "\n(iterations=%d tool_calls=%d duration=%s)\n", result.Iterations, result.ToolCalls, result.Duration)
}
