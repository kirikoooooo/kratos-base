以下是 `https://docs.langchain.com/oss/python/deepagents/sandboxes` 页面的完整中文翻译。为了提升阅读体验，我对页面中大量重复的（仅模型名称不同的）代码块进行了合理精简与合并，保留了核心的架构逻辑和 API 用法。

---

智能体（Agents）会生成代码、与文件系统交互并运行 Shell 命令。因为我们无法预测智能体具体会做什么，所以必须将其运行环境隔离，防止其访问凭证、本地文件或网络。**沙盒（Sandboxes）** 通过在智能体的执行环境与宿主机系统之间建立边界来提供这种隔离。

在 Deep Agents 中，**沙盒是一种后端（Backend）**，它定义了智能体运行的环境。与其他仅暴露文件操作的后端（如 State、Filesystem、Store）不同，沙盒后端还为智能体提供了一个 `execute` 工具，用于运行 Shell 命令。当您配置沙盒后端时，智能体将获得：

- 所有标准的文件系统工具（`ls`, `read_file`, `write_file`, `edit_file`, `delete`, `glob`, `grep`）
- 用于在沙盒内运行任意 Shell 命令的 `execute` 工具
- 保护您宿主机系统的安全边界

## 为什么使用沙盒？

沙盒主要用于**安全防护**。它们允许智能体执行任意代码、访问文件和使用网络，而不会危及您的凭证、本地文件或宿主系统。当智能体自主运行时，这种隔离至关重要。

沙盒在以下场景尤其有用：
- **编码智能体**：自主运行的智能体可以使用 Shell、Git、克隆代码仓库（许多提供商提供原生的 Git API，例如 Daytona 的 Git 操作），并运行 Docker-in-Docker 来执行构建和测试流水线。
- **数据分析智能体**：在安全、隔离的环境中加载文件、安装数据分析库（如 pandas、numpy 等）、运行统计计算，并生成如 PowerPoint 演示文稿等输出。

> **提示**：
> - **使用 Deep Agents Code？** Deep Agents Code 通过 `--sandbox` 标志内置了沙盒支持。
> - **寻找 LangSmith 沙盒？** LangSmith 提供第一方托管的沙盒，您可以直接从 LangSmith UI 或 SDK 使用，无需第三方账号。

---

## 基本用法

以下示例假设您已经使用提供商的 SDK 创建了沙盒/devbox，并配置好了凭证。

### 1. LangSmith 沙盒示例
```python
from deepagents import create_deep_agent
from deepagents.backends import LangSmithSandbox
from langchain_anthropic import ChatAnthropic
from langsmith.sandbox import SandboxClient

# 1. 创建沙盒
client = SandboxClient()
ls_sandbox = client.create_sandbox()
backend = LangSmithSandbox(sandbox=ls_sandbox)

# 2. 初始化智能体
agent = create_deep_agent(
    model=ChatAnthropic(model="anthropic:claude-sonnet-4-6"), # 可替换为其他模型
    system_prompt="You are a Python coding assistant with sandbox access.",
    backend=backend,
)

# 3. 运行并清理
try:
    result = agent.invoke({
        "messages": [{"role": "user", "content": "Create a small Python package and run pytest"}]
    })
finally:
    client.delete_sandbox(ls_sandbox.name) # 确保销毁沙盒以停止计费
```

### 2. 其他提供商示例 (Daytona, E2B, Modal, Runloop, Vercel 等)
其他提供商的用法类似，只需替换对应的 SDK 和 Backend 类：
```python
# 以 E2B 为例
from e2b import Sandbox
from langchain_e2b import E2BSandbox

e2b_sandbox = Sandbox.create()
backend = E2BSandbox(sandbox=e2b_sandbox)
# ... 传入 create_deep_agent 的 backend 参数 ...
#  finally: e2b_sandbox.kill()
```

---

## 生命周期与作用域 (Lifecycle and scoping)

大多数应用程序会选择**每个线程（会话）一个沙盒**（线程级作用域），或者**同一助手的所有线程共享一个沙盒**（助手级作用域）。
**注意**：沙盒在关闭之前会持续消耗资源并产生费用。请确保在不再使用沙盒时将其关闭。

### 线程级作用域（默认）
每次对话都会获得一个专属的沙盒。首次运行时会创建它；同一线程上的后续运行会重用该沙盒。当线程结束或沙盒的 TTL（生存时间）过期时，该环境将被销毁。
建议配置 `idle_ttl_seconds`，以便提供商自动删除或归档空闲环境。

```python
async def agent(config: RunnableConfig):
    thread_id = config["configurable"]["thread_id"]
    sandbox_name = f"thread-{thread_id}"

    # 查找现有沙盒或创建新沙盒
    existing = [sb for sb in client.list_sandboxes() if getattr(sb, "name", None) == sandbox_name]
    if existing:
        ls_sandbox = existing[0]
    else:
        ls_sandbox = client.create_sandbox(
            name=sandbox_name,
            idle_ttl_seconds=3600,  # 空闲 1 小时后自动清理
        )

    return create_deep_agent(
        model="anthropic:claude-sonnet-4-6",
        backend=LangSmithSandbox(sandbox=ls_sandbox),
    )
```

### 助手级作用域 (Assistant-scoped)
同一助手下的所有线程都会重用同一个沙盒。文件、已安装的包和克隆的代码仓库会在不同对话之间持久保存。
**注意**：助手级沙盒会随着时间推移积累状态。请务必配置 TTL、定期使用快照重置，或实现清理逻辑，以防止磁盘和内存无限增长。

---

## 集成模式 (Integration patterns)

根据智能体运行的位置，将智能体与沙盒集成有两种架构模式：

### 1. 沙盒内运行智能体模式 (Agent in sandbox pattern)
智能体在沙盒内部运行，您通过网络与其通信。您需要构建一个预装了智能体框架的 Docker 或虚拟机镜像，在沙盒内运行它，并从外部连接以发送消息。
- **优势**：✅ 紧密镜像本地开发环境；✅ 智能体与环境之间耦合度高。
- **权衡**：🔴 API 密钥必须存放在沙盒内部（存在安全风险）；🔴 更新需要重新构建镜像；🔴 需要额外的基础设施（WebSocket/HTTP）来进行通信。

### 2. 沙盒作为工具模式 (Sandbox as tool pattern) - *推荐*
智能体在您的本地机器或服务器上运行。当它需要执行代码时，会调用沙盒工具（如 `execute`、`read_file`），这些工具会调用提供商的 API 在远程沙盒中执行操作。
- **优势**：✅ 无需重新构建镜像即可即时更新智能体代码；✅ API 密钥保留在沙盒外部，更安全；✅ 沙盒故障不会导致智能体状态丢失；✅ 仅按执行时间付费。
- **权衡**：🔴 每次执行调用都会产生网络延迟。

---

## 沙盒的工作原理 (How sandboxes work)

### 隔离边界
所有沙盒提供商都能保护您的宿主机免受智能体的文件系统和 Shell 操作的影响。但是，沙盒本身**无法**防范：
1. **上下文注入 (Context injection)**：控制智能体部分输入的攻击者可以指示它在沙盒内运行任意命令。沙盒是隔离的，但智能体在其中拥有完全控制权。
2. **网络数据窃取 (Network exfiltration)**：除非阻止网络访问，否则被注入的智能体可以通过 HTTP 或 DNS 将数据发送出沙盒。

### `execute` 方法的核心地位
沙盒后端的架构非常简单：提供商**唯一必须实现**的方法是 `execute()`，它运行 Shell 命令并返回输出。
其他所有文件系统操作（`read`, `write`, `edit`, `delete`, `ls` 等）都是由 `BaseSandbox` 基类在 `execute()` 之上构建的（基类会构造脚本并通过 `execute()` 在沙盒内运行）。
这意味着：
- **添加新提供商非常简单**：只需实现 `execute()`，基类会处理其余所有事情。
- 如果命令输出过大，结果会自动保存到文件中，并指示智能体使用 `read_file` 增量读取，以防止上下文窗口溢出。

### 文件访问的两个平面
理解文件进出沙盒的两种不同方式非常重要：
1. **智能体文件系统工具**：`read_file`, `write_file`, `execute` 等。这是 LLM 在执行期间调用的工具，它们通过沙盒内的 `execute()` 运行。
2. **文件传输 API**：`upload_files()` 和 `download_files()`。这是**您的应用程序代码**调用的方法。它们使用提供商原生的文件传输 API（而非 Shell 命令），专用于在宿主机和沙盒之间移动文件。

---

## 处理文件 (Working with files)

### 初始化沙盒 (Seeding the sandbox)
在智能体运行之前，使用 `upload_files()` 填充沙盒。路径必须是绝对路径，内容必须是 `bytes`（字节）类型：
```python
backend.upload_files([
    ("/src/index.py", b"print('Hello')\n"),
    ("/pyproject.toml", b"[project]\nname = 'my-app'\n"),
])
```

### 获取生成的文件 (Retrieving artifacts)
在智能体完成任务后，使用 `download_files()` 从沙盒中检索文件：
```python
results = backend.download_files(["/src/index.py", "/output.txt"])
for result in results:
    if result.content is not None:
        print(f"{result.path}: {result.content.decode()}")
```

---

## 安全注意事项 (Security considerations)

沙盒可以将代码执行与您的宿主系统隔离开来，但它们**无法防范上下文注入**。如果攻击者控制了智能体输入的一部分，就可以指示智能体读取文件、运行命令或从沙盒内部窃取数据。这使得存放在沙盒内的凭证尤其危险。

🚨 **绝对不要将机密信息（Secrets）放入沙盒中。**
通过环境变量、挂载文件或 `secrets` 选项注入沙盒的 API 密钥、令牌、数据库凭证和其他机密，可能会被受到上下文注入攻击的智能体读取并窃取。即使是短期或受限的凭证也是如此。

### 安全处理机密的最佳实践
如果您的智能体需要调用需要认证的 API，请使用以下两种方法之一：
1. **将机密保留在沙盒外部的工具中（推荐）**：在宿主机环境中定义工具并处理认证。智能体通过名称调用这些工具，但永远看不到凭证。
2. **使用注入凭证的网络代理**：某些提供商支持代理拦截沙盒发出的 HTTP 请求，并在转发前附加凭证（如 `Authorization` 头）。智能体只发出普通请求，永远看不到机密。

**通用安全建议**：
- 在应用程序中对沙盒输出采取行动之前，先进行审查。
- 不需要时，阻止沙盒的网络访问。
- 使用中间件过滤或编辑工具输出中的敏感模式。
- 将沙盒内产生的所有内容视为**不受信任的输入**。