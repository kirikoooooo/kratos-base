# go-deepagent 源码阅读地图

模块路径：`github.com/denizumutdereli/go-deepagent@v0.0.1`

本地查看：

```bash
MOD=$(go env GOMODCACHE)/github.com/denizumutdereli/go-deepagent@v0.0.1
ls "$MOD/pkg/agent"
```

## 推荐阅读顺序

### 1. 入口与配置

| 文件 | 内容 |
|------|------|
| `pkg/agent/types.go` | `AgentConfig` 递归结构体（SubAgents 与主 Agent 同型） |
| `pkg/agent/app.go` | `New()`：注入 VFS 工具、创建 `task` 工具、启动 ReActExecutor |
| `pkg/agent/model.go` | `provider:model` 解析（openai/anthropic/ollama/...） |

### 2. 多 Agent 核心

| 文件 | 内容 |
|------|------|
| `pkg/agent/tasktool.go` | **`task` 工具**：`{"description","subagent_type"}` → 启动 sub-agent ReAct |
| `pkg/agent/react.go` | ReAct 循环、Tool 调用、Event 流、`ReactResult` |
| `pkg/agent/governance.go` | 系统 Prompt 组装、sub-agent 列表写入 orchestrator 指令 |

### 3. 内置工具

| 文件 | 内容 |
|------|------|
| `pkg/agent/vfs_ls.go` 等 | 虚拟文件系统工具（Backend 可插拔） |
| `pkg/agent/skills.go` | Skills Markdown 注入（类似 Claude skills） |

### 4. 中间件与可观测性

| 文件 | 内容 |
|------|------|
| `pkg/agent/middleware.go` | HITL、重试、token 统计等中间件链 |
| `pkg/agent/stream.go` | SSE 事件管理 |
| `pkg/agent/tracing.go` | OpenTelemetry span |

### 5. 测试用例（行为示例）

| 文件 | 内容 |
|------|------|
| `pkg/agent/e2e_test.go` | `TestE2E_MultiProvider_SubAgents` 跨 Provider 委派 |
| `pkg/agent/agent_test.go` | 嵌套 SubAgents 配置测试 |

## 与本 Demo 的对应关系

`third_party/deepagentsdemo/demo.go` 中的 `MultiAgentConfig()` 等价于官方 README 的 Multi-Agent Systems 示例，固定三个 sub-agent：

- **researcher** — 调研归纳
- **coder** — Go 代码方案
- **reviewer** — 评审 checklist

Orchestrator 通过库自动添加的 **`task`** 工具选择 sub-agent；每个 sub-agent 在独立 ReAct 循环中运行，结果返回给 orchestrator 整合。

## LangChain Python deepagents 对照

| Python 概念 | Go (go-deepagent) |
|-------------|-------------------|
| `create_deep_agent(subagents=[...])` | `agent.New(AgentConfig{SubAgents: ...})` |
| SubAgent middleware | 每个 sub-agent 独立 `ReactExecutor` |
| Filesystem backend | `Backend` + VFS tools |
| Human-in-the-loop | `Middleware` HITL |

Python 文档：`https://github.com/langchain-ai/deepagents`
