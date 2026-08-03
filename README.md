# ActorAgentAlliance

基于 `Kratos` 的 Go 示例项目。当前阶段的主目标不再是继续扩展游戏垂类 agent，而是先做成一个类似 Claude Code 的通用 agent 底座：本地任务接入、工具调用、代码库操作、会话记忆与可观测闭环；后续再考虑 Actor 化执行、多 Agent 编排与 A2A 演进。

## 项目目标

- 把 AI Agent 以低侵入方式接入 `Kratos` 运行时
- 实现类似 Claude Code 的通用 coding agent 基础能力
- 验证任务投递、执行、工具调用和结果回写链路
- 为后续演进到 Actor Mailbox、多 agent 协同、A2A 委派和游戏业务模块打基础

## 当前能力概览

| 能力 | 说明 |
|------|------|
| 任务 API | HTTP `POST /api/v1/tasks` 创建任务，`GET /api/v1/tasks/{taskID}` 查询状态 |
| Agent Runtime | `default` / `router` / `coder` / `reviewer`，基于 OpenAI 兼容 API + 本地工具循环 |
| 本地工具 | `read_file`、`edit_file`、`write_file`、`exec_command`（工作区根目录内） |
| Agent 记忆 | 用户级 / 会话级提示词记忆、对话历史持久化（`.myagent`） |
| 上下文压缩 | 按 session 统计对话体积，超过阈值自动压缩后再送 LLM |
| 会话文件变更 | 按 session 累积 `read_file` / `edit_file` / `write_file` 变更，Dashboard 展示 unified diff |
| 会话错误日志 | 失败写入 JSONL，下一轮 system prompt 注入近期错误摘要 |
| 委派 Trace | 内存 trace + Dashboard 时间线、执行计划、SSE 实时刷新 |
| A2A 验证 | router 本地/远端 A2A JSON-RPC 子任务委派 |

### 架构分层

```text
cmd/kratos-demo/              # 入口、Wire 注入
internal/
  service/                    # HTTP JSON-RPC、Dashboard 适配
  server/                     # Kratos Server
  conf/                       # 配置 Proto
  biz/                        # 仅 AgentRuntime / DelegationVerifier 契约
  data/
    model/                    # 领域模型与 Store 接口
    agent/                    # LangChain runtime、记忆、工具
    tasking/                  # TaskRepo、Dispatcher、Usecase
    trace/                    # 委派 Trace
    session/                  # 会话文件变更
    common/                   # 共享工具
api/                          # Proto 与生成代码
```

### Agent 记忆目录（`data.agent_memory.dir`，默认 `.myagent`）

| 路径 | 内容 |
|------|------|
| `users/{user_id}.json` | 用户级：工具/Skill 提示、命令策略、工作区约定 |
| `sessions/{session_id}.json` | 会话级：提示词微调、本会话已用工具 |
| `conversations/{session_id}.json` | 会话对话历史（human / ai / tool turns） |
| `changes/{session_id}.json` | 本会话文件变更快照与 diff |
| `errors/{session_id}.jsonl` | 本会话错误日志（JSONL，供 AI 排查迭代） |

启动时若用户记忆为空，会从工作区自动发现 `.agents/skills` 与工具目录并写入 bootstrap 提示。

### Dashboard（`/debug/a2a`）

- 按 agent 启动 runtime，固定 session `dashboard-{agent}`，支持**连续对话**（同 session 追加用户消息）
- **Delegation Timeline**：任务阶段、工具调用、委派、失败等事件
- **Context Usage**：上下文体积进度条；发生压缩时展示 `context_compress` 事件
- **Session File Changes**：本会话内文件修改的 unified diff
- **SSE**：`/debug/a2a/stream` 实时推送 session 状态

### 本地工具约定

- 修改已有文件须先 `read_file`，再用 `edit_file` 原子操作（`insert_line`、`search_replace` 等）；`write_file` 仅用于新建
- 对目录调用 `read_file`（如 `read_file internal/biz`）返回条目列表，再读具体文件；避免用 `exec_command` 做目录搜索
- `exec_command` 全任务最多 3 次，超限后返回 tool 观测而非直接终止整轮（引导改用 `read_file`）

## 启动

在项目根目录执行：

```bash
### 启动方式

**HTTP + Dashboard（调试面板）**

```bash
go run ./cmd/kratos-demo -conf ./configs
# 或
make run
```

Dashboard：http://127.0.0.1:8000/debug/a2a

**本地 CLI（类似 Claude Code）**

启动全部 agent（default / router / coder / reviewer），交互消息默认由 **router** 接收并委派：

```bash
go run ./cmd/kratos-demo -conf ./configs -cli
# 或
make cli
```

CLI 命令：`/help` `/agents` `/session` `/new` `/exit`
```

启动后可访问：

- HTTP: `http://127.0.0.1:8000`
- A2A Dashboard: `http://127.0.0.1:8000/debug/a2a`

### 配置示例（`configs/config.yaml`）

```yaml
data:
  agent_memory:
    dir: .myagent
    user_id: default
    context_compress_threshold: 200000   # 超过该字符估算值则压缩
    keep_recent_turns: 24                 # 压缩时保留的最近对话轮次组
    tool_output_max_chars: 8000
ai:
  openai:
    api_key: "<your-key>"
    base_url: "https://api.openai-proxy.org/v1"  # 可选，OpenAI 兼容代理
    model: gpt-5.4-mini
    timeout_seconds: 180
```

说明：`default` / `router` / `coder` 在 function calling 场景下会将 `gpt-5*` 模型名规范为 `gpt-4o-mini`（兼容代理能力）；`reviewer` 使用配置中的原始模型名。

### HTTP 创建任务示例

```bash
curl -X POST http://127.0.0.1:8000/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"agent":"default","prompt":"阅读 README 并总结项目结构"}'
```

## 进度

说明：下面「已完成」中的 actor / A2A / 多角色能力为仓库内技术验证资产；产品边界仍以**本地通用 agent 闭环**为准。

### 已完成

- [x] `Kratos` HTTP 服务与 JSON-RPC 接口与 `biz / data / service / server` 分层
- [x] 任务创建与查询 API、内存任务存储、actor 任务调度
- [x] `router / coder / reviewer` agent 与 router 本地/远端委派
- [x] A2A 委派 trace 与 Dashboard（时间线、SSE、执行计划）
- [x] LangChain OpenAI 兼容 runtime + 本地工具调用循环
- [x] Agent 记忆：用户/会话提示词、对话持久化、bootstrap 工具与 Skill 发现
- [x] 会话上下文压缩与 Dashboard 用量展示
- [x] 会话级文件变更记录与 Dashboard diff 展示
- [x] 会话级错误日志（JSONL）与 prompt 注入
- [x] Dashboard 连续对话（`dashboard-{agent}` 固定 session）

### 计划中

- [ ] 任务与 trace 的数据库持久化（当前为内存）
- [ ] 密钥与配置管理（环境变量 / 密钥文件，避免明文进仓库）
- [ ] 更完整的重试、审计与可观测（指标、结构化日志）
- [ ] Dashboard 独立「错误历史」面板（当前可通过 JSONL + prompt 注入使用）

### 后续阶段

- [ ] 分布式运行与跨节点调度
- [ ] 统一 `coordinator` 与多 Agent 编排
- [ ] A2A 流式、取消与认证能力
- [ ] 游戏业务 actor 与业务消息模型接入

## 多 Agent 协同畅想（后续阶段）

以下内容不属于当前里程碑，保留为演进方向。

项目后续希望从当前的本地通用 runtime 和过渡性的 `router / coder / reviewer` 验证链路，逐步演进到一个更贴近游戏生产场景的一主多从、多 Agent 协同范式。核心思路是引入一个统一调度的 `coordinator`，负责目标拆解、上下文编排、异步收发、状态汇总与回写；再由多个面向不同职责的从属 Agent 并行协作。

### 角色设想

- `coordinator`（协调器 AI）
  - 作为主控 Agent，接收来自玩家、策划工具、运营平台或测试系统的任务
  - 负责拆解任务、分派给不同从属 Agent、维护共享上下文和全局状态
  - 支持同步调用与异步消息回收，必要时可做重试、仲裁与结果合并

- `chatter`（AI NPC 对话智能体）
  - 面向 NPC 对话、剧情推进、情绪表达、世界观一致性维护
  - 能根据 `coordinator` 提供的任务上下文生成更贴近角色设定的互动内容
  - 可将对话事件、玩家反馈、角色状态变化再异步回传给 `coordinator`

- `gamer`（自动游戏 AI）
  - 面向自动游玩、行为探索、任务通关、战斗或地图交互验证
  - 负责把高层目标转成游戏内可执行动作序列
  - 在执行过程中持续上报路径、行为决策、卡点与异常状态

- `tester`（游戏开发自动化测试 AI）
  - 面向功能测试、冒烟测试、回归测试、性能与稳定性验证
  - 可以围绕版本变更自动生成测试计划，驱动 `gamer` 或其他工具执行
  - 负责沉淀测试报告、失败样本、复现步骤与风险摘要

### 协同范式

- 一主多从：所有任务先进入 `coordinator`，由它决定是串行、并行还是分阶段调度
- 可异步收发：从属 Agent 不要求严格同步返回，可通过消息队列、actor mailbox、事件流或回调接口回传结果
- 状态可汇总：`coordinator` 汇聚各 Agent 的中间态、最终结果与异常信息，形成统一任务视图
- 可扩展工具链：后续可继续接入如 `planner`、`observer`、`builder`、`ops` 等更多专用 Agent
- 可挂接游戏业务：把 NPC、战斗、关卡、剧情、测试平台都视为可被 Agent 协同调度的能力节点

### 简图

```text
                    +----------------------+
                    | coordinator          |
                    | 协调器 AI / 主控中枢 |
                    +----------+-----------+
                               |
         +---------------------+---------------------+
         |                     |                     |
         v                     v                     v
+----------------+   +----------------+   +----------------+
| chatter        |   | gamer          |   | tester         |
| NPC对话智能体  |   | 自动游戏AI     |   | 自动化测试AI   |
+-------+--------+   +-------+--------+   +-------+--------+
        |                    |                    |
        +---------- 异步消息 / 事件回传 / 状态汇总 ----------+ 
                               |
                               v
                    +----------------------+
                    | actor / A2A / tools  |
                    | 游戏服务与工具能力层 |
                    +----------------------+
```

### 演进方向

- 先在现有 actor + A2A 基础上，把 `coordinator` 抽象为统一任务编排入口
- 为 `chatter / gamer / tester` 定义统一的任务协议、状态协议和结果回写协议
- 支持多 Agent 并发执行、超时控制、失败重试、取消和优先级调度
- 在 dashboard 中展示主任务与子 Agent 的异步协作链路
- 逐步把这套范式从 demo 演化成适配真实游戏研发流程的 Agent 协作底座
