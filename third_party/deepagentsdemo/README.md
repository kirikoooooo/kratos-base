# Deep Agents 多 Agent Demo

基于 [go-deepagent](https://github.com/denizumutdereli/go-deepagent)（LangChain Deep Agents 的 Go 实现，构建于 langchaingo）的本地演示，方便阅读多 Agent 编排代码。

对标 LangChain 官方 [deepagents](https://github.com/langchain-ai/deepagents) 的核心能力：

| 能力 | 本 Demo | 上游实现位置 |
|------|---------|-------------|
| Sub-agents 委派 | orchestrator → researcher/coder/reviewer | `pkg/agent/tasktool.go` |
| task 工具 | 自动注入 | `pkg/agent/app.go` L67-72 |
| ReAct 循环 | 流式事件 | `pkg/agent/react.go` |
| VFS 工具 | ls/read/write/grep/glob | `pkg/agent/vfs_*.go` |
| Thread/Checkpoint | 可选（本 demo 用无状态 Process） | `pkg/agent/app.go` Send |

## 目录结构

```text
third_party/deepagentsdemo/
  demo.go              # 多 Agent 配置与 App 构造（中文注释）
  events.go            # ReAct 事件流打印
  cmd/deepagentsdemo/  # CLI 入口
  README.md
  CODEMAP.md           # 上游源码阅读指南
```

## 快速查看（无需 API Key）

```bash
go run ./third_party/deepagentsdemo/cmd/deepagentsdemo --show-tree
```

输出 orchestrator 与三个 sub-agent 的职责说明，以及库自动注入的工具列表。

## 运行多 Agent 任务

```bash
export OPENAI_API_KEY=sk-...
go run ./third_party/deepagentsdemo/cmd/deepagentsdemo \
  "先让 researcher 总结 RAG 混合召回，再让 coder 写 Go 接口草稿，最后 reviewer 评审"
```

stderr 会打印 ReAct 事件流，可观察 `task` 工具如何委派 sub-agent。

## 与本项目的关系

- 本仓库 `third_party/tools/router_agent.json` 等是 **单轮 tool-calling 别名**；
- Deep Agents 是 **递归 ReAct + 独立 sub-agent 上下文**，更接近 `docs/competitor/deepagents/` 描述的产品形态；
- 生产集成可参考 go-deepagent 的 `pkg/server`（Agent Protocol HTTP + SSE）。

## 依赖

```bash
# 已写入 go.mod
github.com/denizumutdereli/go-deepagent v0.0.1
github.com/tmc/langchaingo
```

## 进一步阅读

见 [CODEMAP.md](./CODEMAP.md) 获取 go-deepagent 模块内关键文件路径（`go env GOMODCACHE` 下）。
