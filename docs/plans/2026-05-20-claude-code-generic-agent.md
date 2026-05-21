# Claude Code 风格通用 Agent 第一阶段实现计划

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** 先把项目做成一个可在单机内稳定运行的通用 Agent 底座，具备任务接入、上下文管理、工具调用、结果回写和基础自检能力，形态上接近 Claude Code 这种“对代码库和本地工具有操作能力”的通用 agent。

**Architecture:** 第一阶段不再把 `router / coder / reviewer` 作为核心产品形态，而是把它们收敛成同一套通用 agent runtime 的不同提示词/策略配置。主链路仍然沿用 Kratos 的 `server -> service -> biz -> data` 分层，但 agent 层要补上通用工具抽象、会话状态、操作日志和结果产物，先把“读代码、改代码、跑命令、总结结果”这一闭环跑通。

**Tech Stack:** Go, Kratos, HTTP/JSON, 当前项目内存任务存储, `langchaingo`, 本地工具目录 `third_party/tools`

---

### Task 1: 明确第一阶段的产品边界

**Files:**
- Modify: `README.md`
- Modify: `doc/设计思路.md`

**Step 1: 写清楚第一阶段目标**

在 README 和设计思路里把第一阶段目标改成“通用 agent 底座”，明确不是继续扩展业务型 `router/coder/reviewer` 演示，而是先实现类似 Claude Code 的通用工作流。

**Step 2: 写清楚暂缓项**

明确暂缓分布式、多 Agent 协同编排、远程委派、复杂 dashboard，只保留本地闭环和可观测性。

**Step 3: 验证文档一致性**

Run: `git diff -- README.md doc/设计思路.md`
Expected: 第一阶段叙述统一，没有和“通用 agent”相冲突的旧表述。

### Task 2: 抽象通用 Agent Runtime 接口

**Files:**
- Modify: `internal/biz/agent.go`
- Modify: `internal/data/langchain.go`
- Modify: `internal/biz/task.go`

**Step 1: 先写失败的测试**

补一组测试，覆盖“同一个 runtime 可以接受通用任务，而不是只识别 router/coder/reviewer”。

**Step 2: 运行测试确认失败**

Run: `go test ./internal/... -run TestAgentRuntime -v`
Expected: 失败，暴露当前 runtime 仍然强绑定三类 agent。

**Step 3: 实现最小接口**

把 `Supports` / `Execute` / `ReceiveTask` 的语义收敛为“通用 agent + profile/strategy”，保留旧 agent 名称仅作兼容入口。

**Step 4: 验证测试通过**

Run: `go test ./internal/... -run TestAgentRuntime -v`
Expected: PASS

### Task 3: 补齐 Claude Code 式工具层

**Files:**
- Create: `internal/data/tools/*.go`
- Modify: `internal/data/langchain.go`
- Modify: `third_party/tools/catalog.go`

**Step 1: 先写失败的测试**

覆盖至少三类工具能力：读文件、写文件、执行命令；同时验证工具结果会回到 agent 上下文里。

**Step 2: 运行测试确认失败**

Run: `go test ./internal/data -run TestToolCatalog -v`
Expected: 失败，提示工具尚未接入或工具返回值不符合预期。

**Step 3: 实现工具绑定**

把工具入口统一挂到 catalog，再由 runtime 注入；优先支持 repo 内文件读取、局部修改和只读命令执行。

**Step 4: 验证测试通过**

Run: `go test ./internal/data -run TestToolCatalog -v`
Expected: PASS

### Task 4: 让任务链路支持“读-改-跑-总结”

**Files:**
- Modify: `internal/service/task.go`
- Modify: `internal/biz/task.go`
- Modify: `internal/data/task_dispatcher.go`

**Step 1: 先写失败的测试**

覆盖提交任务后，任务会进入 `pending -> running -> done/failed`，并且结果里有摘要、输出和错误信息。

**Step 2: 运行测试确认失败**

Run: `go test ./internal/... -run TestTaskFlow -v`
Expected: 失败，暴露结果回写或状态迁移不完整。

**Step 3: 实现最小闭环**

让任务服务层把通用任务投递给 runtime，runtime 负责产出可读结果，data 层回写最终状态。

**Step 4: 验证测试通过**

Run: `go test ./internal/... -run TestTaskFlow -v`
Expected: PASS

### Task 5: 增强可观测性和人工验证面板

**Files:**
- Modify: `internal/service/dashboard.go`
- Modify: `internal/data/trace_memory.go`
- Modify: `internal/biz/trace.go`

**Step 1: 先写失败的测试**

覆盖会话列表、事件时间线、最后一次工具调用和最终结果摘要。

**Step 2: 运行测试确认失败**

Run: `go test ./internal/service -run TestDashboard -v`
Expected: 失败，说明 dashboard 还没展示通用 agent 的关键过程。

**Step 3: 完善展示**

把 dashboard 从“agent 启动/委派演示”改成“任务执行过程观察器”，突出输入、工具调用、结果和失败原因。

**Step 4: 验证测试通过**

Run: `go test ./internal/service -run TestDashboard -v`
Expected: PASS

### Task 6: 做一次端到端验收

**Files:**
- Modify: `README.md`
- Modify: `doc/设计思路.md`

**Step 1: 跑最小端到端场景**

提交一个需要读代码、改代码、再总结的任务，确认通用 agent 能完整执行。

**Step 2: 验证结果**

Run: `go test ./...`
Expected: 全量测试通过，且 README 中的第一阶段描述与实现一致。

**Step 3: 收尾**

补上第一阶段完成标准和下一阶段入口，避免后续又回退到“先做垂类 agent 演示”的方向。

