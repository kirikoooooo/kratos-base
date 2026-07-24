# SWE-bench Lite 评测使用说明

## 目标

使用官方 SWE-bench Lite 对 Agent 的代码修复能力进行离线评测。每个实例提供仓库与 GitHub Issue；Agent 生成 patch；官方 Docker harness 应用 patch 并运行测试，以 `resolved` 计算通过率。

官方源码位于 `third_party/SWE-bench`，当前固定在 commit `f7bbbb2`。它是独立 Python/Docker 工具，不参与 Go 主程序构建。

```text
SWE-bench Lite instance (repo + issue)
  -> Agent 在隔离工作区生成 git diff
  -> predictions.jsonl
  -> SWE-bench Docker harness
  -> evaluation_results/<run_id>/results.json
```

## 环境准备

需要 Python 3.8+、Docker 和足够磁盘空间。官方建议 x86_64、至少 120GB 空闲磁盘、16GB RAM、8 CPU；Docker Desktop 要同步提高虚拟磁盘配额。

```bash
cd third_party/SWE-bench
uv venv
uv pip install -e .
docker info
```

Apple Silicon/ARM 上官方 Docker 镜像支持是实验性的。先用单实例和 `--namespace ''` 本地构建验证；大规模 Lite 评测建议 x86_64 Linux 或云端。

## 先验证 Harness

以下命令只验证官方 gold patch 与环境，不测试本 Agent。优先选择当前仍可从上游仓库检出的实例；不要把单个实例的上游分支/commit 失效误判为 Agent 失败：

```bash
cd third_party/SWE-bench
uv run python -m swebench.harness.run_evaluation \
  --dataset_name princeton-nlp/SWE-bench_Lite \
  --predictions_path gold \
  --instance_ids astropy__astropy-12907 \
  --max_workers 1 \
  --run_id validate-gold
```

ARM/macOS 追加 `--namespace ''`。确认 `evaluation_results/validate-gold/` 有结果后，再运行 Agent 评测。

已知验证记录：在本机 Apple Silicon 上，`sympy__sympy-20590` 的环境和 Harness 镜像均可构建，但实例 Dockerfile 尝试从 SymPy 上游 clone 已不存在的 `1.7` 分支，返回 `git clone` 128。因此该实例不能用作当前环境基线；改用其他 Lite 实例，或在 x86_64/官方预构建镜像环境复测。

## Prediction 契约

评测输入必须是 JSONL：每行只对应一个实例，`model_patch` 是从该实例仓库根目录生成的标准 unified git diff。

```json
{"instance_id":"sympy__sympy-20590","model_name_or_path":"kratos-agent/deepseek-v4-flash","model_patch":"diff --git a/path/file.py b/path/file.py\n..."}
```

建议将输出存入项目根目录外或 `.myagent/evaluations/`，例如：

```text
.myagent/evaluations/swebench-lite/predictions.jsonl
```

生成器必须确保：

- 使用 Lite 数据集提供的基线 commit 和仓库；不能在当前 `kratos-base` 工作区直接生成 patch。
- 每个 `instance_id` 最多一行，`model_patch` 可被 `git apply` 成功应用。
- 记录模型、Prompt/Skill/MCP 版本、开始时间和 Agent 原始输出，便于复现实验；不要把 API Key 写入 prediction 文件。
- Agent 失败时仍可写空 patch 作为覆盖率统计，但应与模型错误分开记录。

## 运行 Lite 评测

```bash
cd third_party/SWE-bench
uv run python -m swebench.harness.run_evaluation \
  --dataset_name princeton-nlp/SWE-bench_Lite \
  --predictions_path ../../.myagent/evaluations/swebench-lite/predictions.jsonl \
  --max_workers 1 \
  --run_id kratos-agent-YYYYMMDD
```

先使用 `--max_workers 1` 和少量 `--instance_ids` 做冒烟测试；确认稳定后再提高并发。评测过程生成 Docker 镜像、`logs/build_images/`、`logs/run_evaluation/` 和 `evaluation_results/`，这些均不应提交到本仓库。

## 读取结果

结果目录：

```text
third_party/SWE-bench/evaluation_results/<run_id>/
  results.json
  instance_results.jsonl
  run_logs/
```

核心指标：

- `Instances submitted`：有 prediction 的实例数。
- `Instances completed`：Harness 完成运行的实例数。
- `Instances resolved`：patch 成功修复 Issue 的实例数。
- `Resolution rate`：resolved / submitted；同时报告 completed 比率，避免把环境失败误当模型失败。

## 与 Langfuse 的关系

每个实例应作为一条独立 Agent Trace，使用 `instance_id` 作为可过滤 metadata，记录模型、输入 Issue、生成 patch 摘要和评测结果。不要把多个实例放入一条 Trace；可用统一 run ID 作为实验分组。SWE-bench 的官方判分仍以 Docker Harness 结果为准，Langfuse 用于分析失败路径、延迟和成本。

## 官方资料

- `third_party/SWE-bench/docs/guides/evaluation.md`
- https://www.swebench.com/
- https://huggingface.co/datasets/SWE-bench/SWE-bench_Lite
