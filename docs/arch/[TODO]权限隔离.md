# RBAC 权限隔离设计

## 1. 目标与边界

本设计为 Kratos Agent 增加服务端强制的最小权限控制。当前使用 Casbin 实现本地 principal/role、工具权限、工作区路径和命令 allowlist；OIDC/mTLS、MCP 和 HTTP/gRPC 身份认证仍是后续项。

- 区分调用者身份，避免 HTTP / gRPC / CLI 请求默认拥有全部 Agent 能力。
- 限制 Agent 可以调用的本地工具、MCP 工具、远程 Agent 和工作区路径。
- 将“是否允许执行”与“高风险操作是否需要人工确认”分开处理。
- 为审计提供稳定的主体、角色、权限、资源和决策记录。

RBAC 不是提示词约束。LLM、Skill、工具描述和 CLI 权限模式均不构成安全边界；所有拒绝必须在服务端工具执行前生效。

本设计不负责身份提供商实现。生产环境可由网关、OIDC/JWT 或 mTLS 认证身份后，将主体信息传入本服务；CLI 使用本地显式身份配置。

## 2. 当前架构与风险点

现有系统已经具有以下边界，但尚未具备身份和授权能力：

| 组件 | 当前行为 | RBAC 后的职责 |
| --- | --- | --- |
| HTTP / gRPC Server | 仅日志、恢复中间件 | 认证请求并将 `Principal` 放入 context |
| AgentRuntime | 按 AgentKind 运行工具循环 | 创建授权范围，委派时不能扩权 |
| ToolExecutor | 执行 read/edit/write/delete/exec、Skill 工具 | 每次调用前检查工具和路径权限 |
| MCP Client | 发现并调用远程服务工具 | 检查 MCP 服务和具体 MCP 工具权限 |
| RemoteAgent | 通过 gRPC 委派子任务 | 传递或收窄原授权范围 |
| CLI approval | 高风险操作交互确认 | 只作为已授权动作的二次确认 |

尤其注意：当前 `PermAuto` 仅表示无需人工确认，绝不能表示绕过 RBAC。

## 3. 核心模型

### 3.1 主体、角色、权限

```text
Principal (调用主体)
  ├── id: alice
  ├── roles: [developer]
  └── attributes: {tenant: demo, auth_source: oidc}

Role (角色)
  └── permissions: [tool.read_file, tool.edit_file, ...]

Permission (权限)
  └── resource constraints: 路径、MCP 服务、远程 Agent、命令策略
```

角色只能授予权限，不能直接授予“管理员绕过”。所有未显式匹配的请求默认拒绝。

### 3.2 权限命名

权限遵循 `domain.action`；资源限制通过角色规则表达：

| 权限 | 作用 |
| --- | --- |
| `agent.execute` | 提交任务并执行指定 AgentKind |
| `agent.delegate` | 委派给本地或远程 Agent |
| `tool.read_file` | 读取文件或列目录 |
| `tool.search_skills` | 查询 Skill 摘要 |
| `tool.load_skill` | 读取选定 Skill 正文 |
| `tool.edit_file` | 原子修改已有文件 |
| `tool.write_file` | 创建新文件 |
| `tool.delete_file` | 删除文件 |
| `tool.exec_command` | 执行受控命令 |
| `mcp.<server>.<tool>` | 调用指定 MCP 服务的指定工具 |
| `memory.read` / `memory.write` | 访问会话、用户记忆 |
| `audit.read` | 查询审计记录 |

`*` 通配符只允许在系统管理员角色中使用，生产配置应避免对业务角色使用。

### 3.3 资源约束

仅有工具权限不足以隔离资源。下列约束必须一起匹配：

| 资源 | 约束字段 | 示例 |
| --- | --- | --- |
| 工作区文件 | `paths` | `internal/**`, `docs/**` |
| Shell 命令 | `command_allowlist` | `go test ./...`, `go build ./...` |
| MCP 服务 | `mcp_servers` | `milvus` |
| MCP 工具 | `mcp_tools` | `milvus.milvus_vector_search` |
| 委派目标 | `agents` | `coder`, `reviewer` |
| 会话 | `tenant`、`owner_only` | 当前主体自己的会话 |

路径规则以工作区根目录为基准，先规范化再匹配。拒绝绝对路径、`..` 穿越和符号链接跳出允许根目录的访问。

## 4. 角色基线

| 角色 | 用途 | 核心能力 |
| --- | --- | --- |
| `viewer` | 只读诊断、知识检索 | 执行 `reviewer`、读文件、检索/加载 Skill、只读 MCP |
| `developer` | 常规研发 | `viewer` + coder、编辑/新建受限路径、测试/构建命令 |
| `operator` | 运行维护 | `viewer` + 受控命令、指定 MCP 管理工具；不默认修改源码 |
| `admin` | 平台管理 | 管理角色、MCP、远程 Agent、审计；高风险动作仍可要求审批 |
| `service-agent` | 远程 Agent 服务身份 | 仅接受上游下传且不超过自身上限的委派范围 |

`reviewer` 是 AgentKind，不等同于 RBAC 的 `viewer` 角色；前者决定运行模式，后者决定调用者被授予的权限。

## 5. 授权流程

```text
HTTP/gRPC/CLI
  -> AuthenticationMiddleware: 解析 Principal
  -> AuthorizationMiddleware: 检查 agent.execute + AgentKind
  -> AgentRuntime: 创建 AuthorizationScope
  -> ToolExecutor / MCP handler: 检查具体 permission + resource
  -> Risk approval: 仅对已授权的高风险动作确认
  -> Audit: 写入 allow / deny / approve / execute
```

### 5.1 授权顺序

1. **认证**：没有主体即拒绝；开发环境可仅启用明确配置的 anonymous principal。
2. **入口授权**：检查主体是否可执行请求的 AgentKind。
3. **工具授权**：LLM 产生每个 ToolCall 时，检查工具权限与资源约束。
4. **委派收窄**：子任务的有效权限为 `父范围 ∩ 子 Agent 服务上限`；禁止委派扩权。
5. **风险审批**：`delete_file`、破坏性命令及策略标记的 MCP 写操作，在 RBAC 允许后按审批策略处理。
6. **审计**：无论允许或拒绝均记录；不得记录 API key、MCP token、完整敏感文件内容。

### 5.2 默认拒绝规则

- 未匹配角色、权限、路径、MCP 服务或命令规则时拒绝。
- `exec_command` 必须同时匹配 `tool.exec_command` 和命令 allowlist；禁止用 shell 通配规则做默认放行。
- `delete_file`、MCP 写操作和远程委派默认拒绝，必须显式授权。
- Skill 只提供工作流说明，不能提升权限；`load_skill` 的正文内容不改变 `AuthorizationScope`。

## 6. 当前配置与后续扩展

`configs/config.yaml` 顶层 `security` 已生效。CLI 将 `local_principal` 注入每个 turn 的 context；`ToolExecutor` 在文件、Skill 和命令执行前强制授权。未配置主体、角色、权限、路径或命令即拒绝。

实现使用 `github.com/casbin/casbin/v2` 的文件模型：`configs/casbin/model.conf` 定义 `sub, obj, path, cmd` 请求与匹配规则，`configs/casbin/policy.csv` 保存 `p(role, permission, path, command)` 和 `g(principal, role)` 策略。路径使用 Casbin `keyMatch2` 的 `internal/*` 形式；命令仍必须完整匹配。

```yaml
security:
  enabled: true
  local_principal: local-dev
  casbin_model_file: casbin/model.conf
  casbin_policy_file: casbin/policy.csv
```

修改角色、主体或工具范围时编辑 `configs/casbin/policy.csv` 后重启服务；发布产物与首次启动生成的配置目录都会包含这两个文件。

```yaml
security:
  enabled: true
  default_effect: deny

  # 开发期可使用静态身份；生产环境应改为 oidc 或 mTLS。
  authentication:
    mode: static # static | oidc | mtls
    static_principals:
      - id: local-dev
        roles: [developer]
      - id: ci-review
        roles: [viewer]

  roles:
    viewer:
      permissions:
        - agent.execute
        - tool.read_file
        - tool.search_skills
        - tool.load_skill
        - mcp.milvus.milvus_list_collections
        - mcp.milvus.milvus_vector_search
      constraints:
        agents: [default, reviewer]
        paths: [README.md, docs/**, api/**, internal/**, configs/**]
        mcp_servers: [milvus]
        owner_only: true

    developer:
      inherits: [viewer]
      permissions:
        - agent.delegate
        - tool.edit_file
        - tool.write_file
        - tool.exec_command
      constraints:
        agents: [default, router, coder, reviewer]
        paths: [api/**, internal/**, configs/**, docs/**, scripts/**, tests/**]
        command_allowlist:
          - "go test ./..."
          - "go build ./..."
          - "go vet ./..."
          - "make proto"

    operator:
      inherits: [viewer]
      permissions:
        - tool.exec_command
        - mcp.milvus.milvus_create_collection
        - mcp.milvus.milvus_insert_data
      constraints:
        agents: [default]
        command_allowlist:
          - "docker compose ps"
          - "docker compose logs *"
        mcp_servers: [milvus]

    admin:
      permissions: ["*"]
      constraints:
        agents: [default, router, coder, reviewer]
        paths: ["**"]
        mcp_servers: ["*"]
      approval:
        delete_file: required
        exec_command: required
        mcp_write: required
```

### 6.1 配置语义

- `enabled: false` 仅允许本地开发；服务启动日志必须明确标记“授权已关闭”。生产环境禁止该值。
- `inherits` 只允许无环继承；加载时需要展开并去重权限。
- 同一角色的 `permissions` 为并集；约束按最小范围合并。多个角色的有效权限为并集，但每条权限只能使用其所属角色的约束，不能把 A 角色的权限与 B 角色的宽松约束拼接。
- `owner_only: true` 要求 session / memory 所属 principal 与调用 principal 相同；多租户环境还要匹配 tenant。
- 命令 allowlist 应解析为命令 AST 或至少严格比较可执行文件及参数，不能仅做 `strings.Contains`。
- MCP 工具名称以运行时 `tools/list` 发现的 `server.tool` 形式匹配；服务启动时需校验配置中引用的工具是否真实存在。

## 7. 代码落点

| 层 | 新增/修改 | 责任 |
| --- | --- | --- |
| `internal/conf/conf.proto` | `Security`、`Principal`、`Role` | 解析静态 RBAC 配置 |
| `internal/biz/authz` | `Principal`、`Authorizer` 接口 | 定义领域模型与端口 |
| `internal/data/authz` | Casbin 策略编译器 | 身份解析和策略评估 |
| `internal/server` | HTTP / gRPC middleware | 认证主体写入 context |
| `internal/data/agent_runtime/ctx` | `WithPrincipal`、`WithAuthorizationScope` | 请求上下文传递 |
| `internal/data/agent_runtime` | ToolCall 前授权、委派收窄 | 阻止模型越权 |
| `internal/data/tool` | ToolExecutor 路径/命令检查 | 本地资源强制隔离 |
| `internal/data/mcp` | MCP 服务/工具授权 | 隔离远程服务能力 |
| `internal/data/trace` | AuditEvent | 记录审计事件 |

建议 `Authorizer` 接口如下：

```go
type Authorizer interface {
    Authorize(ctx context.Context, request AuthorizationRequest) (Decision, error)
}

type AuthorizationRequest struct {
    Principal Principal
    Permission string
    Resource Resource
}

type Resource struct {
    ToolName string
    Path string
    Command string
    MCPServer string
    MCPTool string
    Agent string
}
```

`Decision` 至少包含 `Allowed`、`Reason`、`MatchedRole` 和 `ApprovalRequired`，便于将拒绝原因回传给 LLM，并写入审计。

## 8. 与现有 CLI 审批的关系

| 情况 | RBAC | CLI 审批 | 结果 |
| --- | --- | --- | --- |
| viewer 调用 `edit_file` | 拒绝 | 不触发 | 拒绝 |
| developer 编辑允许路径 | 允许 | 不需要 | 执行 |
| developer 删除允许路径 | 允许 | required | 等待确认 |
| developer 删除不允许路径 | 拒绝 | 不触发 | 拒绝 |
| `PermAuto` 下的未授权命令 | 拒绝 | 不适用 | 拒绝 |

因此 `PermAsk` / `PermAgent` / `PermAuto` 只决定授权后是否要求人工确认，不改变授权结论。

## 9. 审计与运维

每个授权决策写入结构化审计事件：

```json
{
  "time": "2026-07-17T10:00:00+08:00",
  "request_id": "task-123",
  "principal": "local-dev",
  "roles": ["developer"],
  "agent": "coder",
  "permission": "tool.edit_file",
  "resource": {"path": "internal/data/tool/tool_executor.go"},
  "decision": "allow",
  "approval": "not_required"
}
```

审计日志需独立于对话记忆保存，并设置保留期、访问权限和脱敏规则。至少告警以下事件：连续拒绝、管理员通配符调用、命令 allowlist 拒绝、跨租户访问和远程委派拒绝。

## 10. 分阶段落地

1. **观察模式**：加载并校验策略，对每次工具调用计算决策但不阻断，记录 would-deny 审计。
2. **本地工具强制**：先强制 `read/edit/write/delete/exec`、路径和命令规则。
3. **Agent 与委派强制**：限制 AgentKind、远程 Agent，并实现权限交集传递。
4. **MCP 强制**：按 `server.tool` 约束 MCP；对写操作引入审批。
5. **生产认证**：从静态主体迁移到 OIDC/mTLS，启用 tenant 与 owner 校验。

每阶段必须包含 allow、deny、路径穿越、符号链接、命令绕过、委派扩权、MCP 未发现工具等单元/集成测试。观察模式中的 would-deny 记录应先清零，再切换为强制拒绝。

## 11. 验收标准

- 未认证请求、未知角色和未配置权限默认拒绝。
- LLM 即使尝试调用未授权工具，也只能收到结构化拒绝，工具不会执行。
- `exec_command` 无法通过 shell 拼接、相对路径或子进程绕过 allowlist。
- 子 Agent 和远程 Agent 不能获得父请求未拥有的权限。
- `PermAuto` 无法绕过 RBAC。
- 审计可关联任务、主体、角色、工具/MCP 资源和最终决策。
