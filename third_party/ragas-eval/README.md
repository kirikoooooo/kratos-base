# ragas-eval: kratos-base 工具调用能力评估

基于 RAGAS 风格的 LLM-as-a-judge 模式，评估 kratos-base Agent 的工具调用能力。

## 评估维度

| 维度 | 说明 | 指标 |
|------|------|------|
| **工具选择** (tool_selection) | Agent 是否为用户意图选择了正确的工具 | Precision, Recall, F1 |
| **参数提取** (param_extraction) | 工具参数是否正确、完整 | Param Accuracy |
| **多步序列** (multi_step) | 多步工具调用的顺序和完整性 | Multi-step Compliance |
| **错误恢复** (error_recovery) | 工具调用失败后的恢复能力 | Error Recovery Score |
| **调用成功率** | 工具执行的成功率 | Success Rate |

## 目录结构

```
third_party/ragas-eval/
├── pyproject.toml          # Python 项目配置
├── requirements.txt        # Python 依赖
├── README.md               # 本文档
├── ragas_eval/             # 核心评估包
│   ├── __init__.py
│   ├── dataset.py          # 评估数据集管理
│   ├── metrics.py          # 工具调用评估指标
│   ├── judge.py            # LLM-as-a-judge 定性评估
│   ├── evaluator.py        # 主评估运行器
│   ├── reporter.py         # 结果报告生成
│   └── adapters.py         # Go trace 数据适配器
├── datasets/               # 评估数据集
│   ├── tool_selection.jsonl    # 工具选择测试用例
│   ├── tool_params.jsonl       # 参数提取测试用例
│   ├── multi_step.jsonl        # 多步序列测试用例
│   └── error_recovery.jsonl    # 错误恢复测试用例
├── scripts/
│   └── evaluate.py             # CLI 入口
└── internal/                   # Go trace 导出（与 kratos-base 集成）
    └── trace_export.go
```

## 快速开始

### 1. 安装依赖

```bash
cd third_party/ragas-eval
pip install -r requirements.txt
```

### 2. 查看可用数据集

```bash
python scripts/evaluate.py --list-datasets
```

### 3. 运行评估（仅指标）

```bash
python scripts/evaluate.py \
  --dataset datasets/tool_selection.jsonl \
  --traces .myagent/traces/
```

### 4. 运行评估（含 LLM Judge）

```bash
export OPENAI_API_KEY="sk-..."
python scripts/evaluate.py \
  --dataset datasets/tool_selection.jsonl \
  --traces .myagent/traces/ \
  --judge
```

### 5. 评估所有数据集

```bash
python scripts/evaluate.py --all --traces .myagent/traces/
```

## 数据集格式

### 工具选择 (tool_selection)

```jsonl
{"id": "ts-001", "category": "tool_selection", "user_query": "帮我看看 README.md", "language": "zh", "expected_tools": ["read_file"], "forbidden_tools": ["exec_command"], "tags": ["read_file", "zh"]}
```

### 参数提取 (param_extraction)

```jsonl
{"id": "tp-001", "category": "param_extraction", "user_query": "读取 tool_executor.go 的前 50 行", "expected_tools": ["read_file"], "expected_params": {"tool_name": "read_file", "params": {"path": "internal/data/tool/tool_executor.go"}, "params_mode": "subset", "should_succeed": true}}
```

### 多步序列 (multi_step)

```jsonl
{"id": "ms-001", "category": "multi_step", "user_query": "先读 entity.go，再在第 5 行后加注释", "expected_tools": ["read_file", "edit_file"], "multi_step": {"steps": [{"tool_name": "read_file", "params": {"path": "internal/biz/tool/entity.go"}}, {"tool_name": "edit_file", "params": {"path": "internal/biz/tool/entity.go", "operation": "insert_after_line", "line": 5}}], "strict_order": true}}
```

### 错误恢复 (error_recovery)

```jsonl
{"id": "er-001", "category": "error_recovery", "user_query": "读取不存在的文件 /etc/shadow.txt", "expected_tools": ["read_file"], "expected_params": {"tool_name": "read_file", "should_succeed": false, "failure_pattern": "does not exist"}}
```

## 评估指标说明

### 工具选择精度 (Tool Selection Precision)
- **定义**: `|expected ∩ selected| / |selected|`
- **说明**: 选中的工具中有多少是期望的。高精度意味着 Agent 不滥选无关工具。

### 工具选择召回 (Tool Selection Recall)
- **定义**: `|expected ∩ selected| / |expected|`
- **说明**: 期望的工具中有多少被选中了。高召回意味着 Agent 没有遗漏必要工具。

### 参数准确率 (Param Accuracy)
- **定义**: 匹配的参数字段数 / 总期望字段数
- **说明**: 支持 `exact`（完全匹配）、`contains`（子串匹配）、`subset`（子集匹配）三种模式。

### 多步合规性 (Multi-step Compliance)
- **定义**: 最长公共子序列长度 / 期望步骤数（严格顺序模式）
- **说明**: 评估 Agent 是否按正确顺序执行了多步操作。

### 错误恢复评分 (Error Recovery Score)
- **定义**: 综合评分（0-1），考虑：是否避免重复相同错误、是否尝试替代方案、是否最终成功。

## 与 Go Agent 集成

在 kratos-base 的 Agent 运行时中，每次工具调用都会通过 `datatrace.DelegationTraceStore` 记录事件。将这些事件导出为 JSONL 格式后，即可用本评估框架分析。

```go
// 在 agent runtime 中导出 trace
// trace 事件包含 tool_name, tool_input, tool_output, error, exit_code 等字段
```

导出示例（trace JSONL 格式）：
```jsonl
{"task_id": "task-001", "user_query": "读取 README.md", "tool_calls": [{"tool_name": "read_file", "input": "README.md", "output": "path: README.md\n1: # Project...", "error": null}]}
```

## 输出格式

每次评估运行输出到 `.myagent/rag/evaluations/<run-id>/`：

```
.myagent/rag/evaluations/run-20260730-143000/
├── per_case.jsonl       # 每个用例的指标和通过状态
├── judge_results.jsonl  # LLM Judge 的定性评估结果
├── summary.json         # 汇总统计（含配置和版本信息）
├── metrics.csv          # CSV 格式，方便表格分析
└── failures.jsonl       # 仅失败的用例，便于调试
```

## 配置环境变量

| 变量 | 说明 | 必需 |
|------|------|------|
| `OPENAI_API_KEY` | LLM Judge 的 API 密钥 | 仅 `--judge` 时 |
| `OPENAI_BASE_URL` | 自定义 API 端点 | 否 |
| `JUDGE_MODEL` | Judge 模型（默认 `gpt-4o-mini`） | 否 |

## 扩展数据集

在 `datasets/` 目录下创建新的 `.jsonl` 文件，按上述格式编写测试用例。运行时用 `--dataset` 指定路径即可。

每个用例必须有稳定的 `id`，建议使用 `category-prefix-NNN` 格式。重复运行同一数据集应产生一致的结果（前提是 LLM judge 使用 `temperature=0`）。
