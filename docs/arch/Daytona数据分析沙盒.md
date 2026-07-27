# Daytona 数据分析沙盒

## 目标

`daytona_data_analysis` 仅用于数据分析、统计计算和生成 PNG 图表。生成的 Python 代码不会在 Agent 宿主机执行，而是在 Daytona 临时 Sandbox 中运行；图表下载到 `.myagent/artifacts/<task-id>/`。

默认关闭，未启用时工具不会暴露给模型。

## 配置

```yaml
runtime:
  daytona:
    enabled: false
    api_key: dtn_...       # 可选，仅限被 .gitignore 忽略的本地配置
    api_key_env: DAYTONA_API_KEY
    api_url: https://app.daytona.io/api
    target: us              # 可选
    snapshot: daytona-small # 应提供 Python、pandas、matplotlib
    auto_delete_minutes: 0
```

`api_key` 优先于 `api_key_env`；本地配置目录已被 `.gitignore` 忽略，因此可使用明文 key。共享配置或生产环境仍建议只使用环境变量：

```bash
export DAYTONA_API_KEY='...'
```

启用后还必须在 `configs/casbin/policy.csv` 为分析角色添加：

```csv
p, analyst, tool.daytona_data_analysis, *, *
g, local-dev, analyst
```

## 执行链路

```text
数据分析请求
  -> daytona_data_analysis
  -> Casbin 检查 tool.daytona_data_analysis
  -> Daytona Go SDK 创建 NetworkBlockAll 临时 Sandbox
  -> 上传指定工作区数据到 /tmp/input
  -> CodeRun 执行 Python
  -> 下载 /tmp/*.png 到 .myagent/artifacts/<task-id>
  -> 删除 Sandbox，写入 Trace
```

工具参数必须包含 `code` 和 `/tmp/*.png` 的 `output_path`；可选 `files` 是上传到沙盒的工作区相对文件。Sandbox 总会在调用结束后删除，`auto_delete_minutes` 作为远端兜底。

## 常用 Go SDK API

导入：`github.com/daytona/clients/sdk-go/pkg/daytona` 与 `github.com/daytona/clients/sdk-go/pkg/types`。所有操作都应传入带超时的 `context.Context`。

| 场景 | API | 用途 / 要点 |
| --- | --- | --- |
| 创建客户端 | `daytona.NewClientWithConfig(&types.DaytonaConfig{...})` | 使用 API Key、API URL、Target 创建客户端；不传字段时 SDK 会读取 `DAYTONA_*` 环境变量。 |
| 创建 Python Sandbox | `client.Create(ctx, types.SnapshotParams{Snapshot: "daytona-small"})` | 默认等待 Sandbox 启动；分析场景应设置 `NetworkBlockAll` 与 `AutoDeleteInterval`。 |
| 获取 / 枚举 | `client.Get(ctx, id)` / `client.List(ctx, query)` | 查询已有 Sandbox；本项目的临时分析不复用 Sandbox。 |
| 执行 Python 代码 | `sandbox.Process.CodeRun(ctx, code)` | 运行 Python 代码，检查 `ExecuteResponse.ExitCode` 和 `Result`；图表代码保存至 `/tmp/*.png`。 |
| 执行 Shell 命令 | `sandbox.Process.ExecuteCommand(ctx, command)` | 适合安装依赖或诊断；Agent 数据分析流程优先 `CodeRun`。 |
| 创建目录 | `sandbox.FileSystem.CreateFolder(ctx, path)` | 上传数据前创建 `/tmp/input` 等目录。 |
| 上传输入文件 | `sandbox.FileSystem.UploadFile(ctx, source, destination)` | `source` 可为本地路径或字节内容；上传用户指定的数据到 Sandbox。 |
| 下载图表 / 结果 | `sandbox.FileSystem.DownloadFile(ctx, remotePath, nil)` | 返回字节；写入 `.myagent/artifacts/<task-id>/` 后再返回给用户。 |
| 停止 / 删除 | `sandbox.Stop(ctx)` / `sandbox.Delete(ctx)` | 临时 Sandbox 推荐 `defer sandbox.Delete(...)`；删除优先于仅停止，避免资源滞留。 |
| 关闭客户端 | `client.Close(ctx)` | 释放 SDK 客户端资源；使用 `defer` 调用。 |

最小执行模式：`Create -> CreateFolder -> UploadFile -> CodeRun -> DownloadFile -> Delete`。不要将 API Key、业务密钥或工作区全部内容上传到 Sandbox。

## 依赖

使用 Daytona 官方 Go SDK：`github.com/daytona/clients/sdk-go/pkg/daytona`。当前官方 SDK 要求 Go 1.25.4；本项目构建环境也需至少使用该版本。

## 与直接使用 Docker Go SDK 的差异

| 对比维度 | Daytona Go SDK | Docker Go SDK（直接管理容器） |
| --- | --- | --- |
| 抽象层级 | 面向可运行开发环境的 Sandbox，封装创建、生命周期、文件与进程操作。 | 面向 Docker daemon 的容器、镜像、网络、卷等底层资源。 |
| 运行位置 | 可通过 Daytona API 在本地或 Daytona 托管的远端基础设施运行。 | 通常运行在连接到 Docker daemon 的本机或指定远端主机。 |
| 环境准备 | 通过 Snapshot 选择预置环境，例如包含 Python、pandas、matplotlib 的镜像。 | 需要自行选择镜像、构建 Dockerfile，并负责依赖安装与镜像缓存。 |
| 隔离与网络 | 提供 `NetworkBlockAll` 等 Sandbox 级配置，适合默认禁止分析代码出网。 | 需要自行配置 network mode、iptables/网络策略、capabilities、seccomp 等隔离措施。 |
| 文件传输 | 提供 `UploadFile`、`DownloadFile` 等文件系统 API。 | 需要自行使用 archive API、bind mount 或 volume 完成文件交换。 |
| 代码执行 | `Process.CodeRun` 直接执行代码，并返回执行结果。 | 需要创建容器、启动或 exec 进程、收集 stdout/stderr，并处理容器状态。 |
| 生命周期治理 | 支持自动删除时限，且可由 Daytona 服务端统一管理 Sandbox。 | 需要自行实现容器命名、超时回收、异常清理和镜像/卷垃圾回收。 |
| 运维依赖 | 依赖 Daytona API、认证与可用的 Target/Snapshot。 | 依赖 Docker daemon 权限、镜像仓库访问和宿主机容量；本机 daemon 失效会直接阻塞执行。 |
| 可移植性 | 通过统一 API 与 Snapshot 降低不同运行环境之间的差异。 | 行为更依赖宿主机 Docker 配置、CPU 架构、挂载路径和镜像构建结果。 |
| 适用场景 | 需要短生命周期、远端隔离、预置开发环境的数据分析或 Agent 代码执行。 | 需要精细控制容器参数、复用既有 Docker 基础设施，或完全离线本地执行的场景。 |

对于本项目的 `daytona_data_analysis`，Daytona 减少了容器构建、文件打包和回收的实现成本，并能将分析代码从 Agent 宿主机隔离出去；代价是增加 Daytona 服务依赖与 API 调用开销。若改为 Docker Go SDK，应补齐镜像供应链、最小权限容器配置、网络隔离、文件传输、超时与资源回收，不能只将 `CodeRun` 替换为 `ContainerExec`。
