# RAG 流程和评估

## 状态总览

> 最后核对：2026-07-27。本文以仓库当前代码和配置为准；`[x]` 表示已有实现或已配置并可由代码路径使用，`[ ]` 表示尚未实现，不能作为产品能力宣称。


| 能力                     | 状态  | 当前证据 / 说明                                                                                                                                     |
| ---------------------- | --- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| 标准 MCP stdio client    | [x] | `internal/data/mcp/client.go` 使用 `github.com/mark3labs/mcp-go` 启动、初始化、发现并调用 MCP 工具。                                                           |
| Milvus MCP 服务配置        | [x] | `internal/conf/default_config.yaml` 配置了本地 Milvus Lite MCP server；Agent 会把发现的工具加入 tool calling 集。                                              |
| 手动创建 collection、写入和检索  | [x] | 官方 Milvus MCP server 提供 `milvus_create_collection`、`milvus_insert_data`、`milvus_text_search`、`milvus_vector_search` 和 `milvus_hybrid_search`。 |
| RAG Skill 使用指引         | [x] | `skills/rag-search/SKILL.md` 约束模型先发现工具，再按相同 embedding 的维度进行写入和检索。                                                                             |
| 文档类型识别（PDF / TXT / MD） | [ ] | 未实现入库器或文件类型路由。                                                                                                                                |
| MinerU PDF 解析          | [ ] | 仓库未引入 MinerU，也没有本地 Pipeline 调用。                                                                                                               |
| Markdown / 文本切块        | [ ] | 未实现 chunker；更没有按标题层级构造父子块。                                                                                                                    |
| 父子索引和 Redis 映射         | [ ] | 未配置 Redis、没有 parent/child ID 模型或 repository。                                                                                                  |
| BM25 倒排索引              | [ ] | 未实现；现有 Milvus 文本/混合检索不可等同于项目自建 Redis BM25。                                                                                                    |
| FAISS 存储后端             | [ ] | 当前默认向量库为 Milvus Lite，不是 FAISS。                                                                                                                |
| RAG 上下文拼接              | [ ] | Agent 可自行调用 MCP 工具，但没有固定的“检索子块 -> 取父块 -> 注入上下文”运行时流程。                                                                                         |
| RAGAS LLM-as-a-judge   | [x] | `third_party/ragas-eval/` — 已引入基于 ragas 风格的工具调用能力评估框架，包含 4 类数据集（50 个用例）、Go trace 导出器和 Python 评估 CLI。                                          |
| 入库 / 评估 CLI            | [ ] | `cmd/` 仅有主服务入口；没有 `ingest` 或 `evaluate-rag` 子命令。                                                                                              |


原始需求中出现的 API Key 已移除。密钥只能从环境变量或被 `.gitignore` 忽略的本地配置读取，严禁写入本文、源码、示例配置、日志或评测产物。

## 当前可用能力

当前项目提供的是“Agent 通过 MCP 操作 Milvus”的检索基础，而不是完整 RAG 平台。启用 `runtime.mcp_servers` 中的 `milvus` 服务后，运行时会执行 MCP `initialize`、`tools/list`，并将发现的工具映射为 LLM function-calling 工具。Agent 可在对话中调用这些工具管理 collection 或执行文本、向量与混合检索。

```text
Agent tool calling
  -> internal/data/mcp.Client
  -> mcp-go stdio client
  -> remote_service/mcp-server-milvus
  -> Milvus Lite (.myagent/milvus.db) 或远端 Milvus
```

使用 `rag-search` skill 时，必须先根据实际 `tools/list` 结果选择工具；不要假设集合名、字段名或 embedding 维度。不得把 `.env`、凭证、私钥或敏感原文写入 collection。

## 目标入库流程 `[ ]`

以下是待实现的目标设计，不代表当前可用能力。

```text
输入目录
  -> 类型路由（.pdf / .txt / .md）
  -> PDF: MinerU Pipeline 解析；TXT/MD: UTF-8 读取
  -> 规范化为 Markdown 资源目录
  -> 按 Markdown 标题建立 parent chunk
  -> 按 token 长度和 overlap 建立 child chunk
  -> 写入 child 向量、关键词索引和元数据
  -> 保存 child_id -> parent_id 映射
  -> 输出 manifest、失败清单和可复现摘要
```



### 输入与输出契约 `[ ]`

建议未来 CLI 固定以下目录，避免污染源码和评测数据：

```text
.myagent/rag/
  sources/                  # 原始输入，只读
  parsed/<run-id>/          # MinerU 或文本规范化后的 Markdown / 资源
  manifests/<run-id>.jsonl  # 每个源文件、parent/child、哈希和失败原因
  artifacts/<run-id>/       # 解析图片等非文本产物
  evaluations/<run-id>/     # RAGAS 输入、原始结果和汇总
```

每个 child 至少应包含：`child_id`、`parent_id`、`source_id`、`source_path`、`heading_path`、`ordinal`、`content`、`content_hash`、`embedding_model`、`embedding_dimension` 和 `created_at`。`source_id` 应由内容哈希或稳定文档 ID 生成，使重复运行可幂等更新。

### 存储选型


| 数据        | 原方案        | 当前状态       | 建议的实现边界                                              |
| --------- | ---------- | ---------- | ---------------------------------------------------- |
| 子片段向量和元数据 | FAISS      | [ ] 未实现    | 抽象 `VectorStore`；首个实现建议复用已接入的 Milvus，而非并行引入 FAISS。   |
| 父子映射      | Redis      | [ ] 未实现    | 抽象 `ParentStore`；Demo 可先使用本地 SQLite/JSON，生产再接 Redis。 |
| BM25 倒排索引 | Redis      | [ ] 未实现    | 抽象 `LexicalStore`；需要明确分词语言、增量更新和版本策略。                |
| 文本 / 混合检索 | Milvus MCP | [x] 已可手动调用 | 不能替代 parent/child 映射和应用层召回编排。                        |


在没有明确性能或离线部署需求前，不建议同时引入 FAISS、Redis 和 Milvus 三套存储：这会使索引一致性、删除、备份和评估基线显著复杂化。应先完成一个可替换接口下的 Milvus 实现，并用评估指标决定是否增加专用 BM25/Redis。

## 目标召回流程 `[ ]`

```text
用户 query
  -> query 规范化 / 安全过滤
  -> lexical BM25 召回 child candidates
  -> dense vector 召回 child candidates
  -> 融合排序（RRF 或经过离线验证的加权公式）
  -> 读取 child 对应 parent
  -> 去重、预算裁剪、保留来源引用
  -> 将 parent context 作为不可信参考资料注入 LLM
```

必须保留每个命中的 `source_id`、`heading_path`、`child_id`、排名、分数和召回方法，供审计与 RAGAS 分析。检索到的内容是外部数据，不得被视为系统指令。

### 目标接口 `[ ]`

建议在 `internal/biz/rag` 定义稳定的领域接口，而非让 Agent 直接依赖某个数据库 SDK：

```go
type Ingestor interface {
    Ingest(ctx context.Context, request IngestRequest) (IngestResult, error)
}

type Retriever interface {
    Retrieve(ctx context.Context, query string, options RetrieveOptions) (RetrieveResult, error)
}

type Evaluator interface {
    Evaluate(ctx context.Context, dataset EvaluationDataset) (EvaluationReport, error)
}
```

`data` 层分别实现解析、向量/关键词存储与评估适配；`service` 或 CLI 负责文件路径、配置加载和结果输出。MCP 适合作为 Agent 的交互入口，但批量入库与可重复评估不应依赖 LLM 发起工具调用。

## RAGAS 评估 `[x]`

已引入 `third_party/ragas-eval/`，提供以下能力：

- **评估数据集**：`tool_selection`（15 例）、`param_extraction`（15 例）、`multi_step`（8 例）、`error_recovery`（12 例），共 50 个用例。
- **指标计算**：工具选择精度/召回/F1、参数准确率、多步合规性、错误恢复评分。
- **LLM Judge**：基于 OpenAI 兼容 API 的定性评估（可选，需 `--judge` 参数）。
- **Go 集成**：`internal/trace_export.go` 将 Agent trace 导出为 JSONL 格式供 Python 评估。
- **CLI**：`python scripts/evaluate.py --dataset datasets/tool_selection.jsonl --traces traces.jsonl`
- **报告输出**：JSONL（逐题）、CSV（表格分析）、JSON（汇总）和失败清单。

RAGAS 仅用于离线评估，不能在在线请求路径中调用。

```json
{
  "id": "stable-case-id",
  "question": "问题",
  "ground_truth": "可选的参考答案",
  "reference_contexts": ["可选的金标准上下文"],
  "tags": ["language:zh", "source:manual"]
}
```

目标评估工作流：

```text
dataset.jsonl
  -> 对每题执行固定版本 Retriever
  -> 记录 retrieved_contexts、答案、耗时、模型与索引版本
  -> RAGAS LLM-as-a-judge（从环境变量读取 provider 凭证）
  -> 输出逐题 JSONL + 汇总 JSON/CSV + 失败原因
```

建议起步指标为：context precision、context recall、faithfulness、answer relevancy，以及检索延迟和空召回率。每次运行必须记录 embedding 模型、LLM judge 模型、Prompt/Skill 版本、索引 manifest hash 和随机种子；否则分数不可比较。

## 建议 CLI 与验收标准 `[ ]`

未来可在现有二进制中增加显式子命令，或新增独立的 `cmd/ragctl`，例如：

```text
ragctl ingest --source .myagent/rag/sources --output .myagent/rag/parsed/run-001
ragctl retrieve --query "..." --top-k 8
ragctl evaluate --dataset datasets/rag-eval.jsonl --output .myagent/rag/evaluations/run-001
```

完成定义：

- [ ] TXT、MD 和 PDF 的成功/失败路径都有自动化测试；PDF 解析依赖以可选集成测试运行。
- [ ] 同一输入重复入库不会产生重复 child，删除/重建能清理映射与索引。
- [ ] 召回结果带 parent context 和来源元数据，并受最大上下文预算限制。
- [ ] 离线评估可在没有在线服务的情况下重放已有检索记录；密钥不进入结果文件。
- [ ] RAGAS 评估的失败、超时和不可评分样例可区分统计。
- [ ] `go test ./...` 通过；可选 Python/MinerU/RAGAS 集成测试被单独标记并有环境前置条件。



## 实施顺序

1. [ ] 先定义 `Ingestor`、`Retriever`、manifest 数据模型及 TXT/MD 单元测试。
2. [ ] 使用当前 Milvus 作为首个 `VectorStore`，完成单一存储基线与幂等入库。
3. [ ] 实现 parent/child、来源引用和上下文预算；先验证纯向量/文本检索的质量。
4. [ ] 再根据评测结果决定是否增加 Redis BM25 和融合排序，而不是预先维护三套索引。
5. [ ] 将 MinerU 作为可选 PDF adapter，隔离 Python 环境和大文件资源消耗。
6. [ ] 最后引入 RAGAS 离线 CLI、数据集版本化与 Langfuse/Trace 关联。



## 结论

当前完成的是 Milvus MCP 检索基础和 Agent 侧工具接入；完整 RAG 入库、父子索引、BM25、固定上下文拼接、RAGAS 评估和 CLI 均未完成。本文保留这些未完成项作为可执行的设计和验收清单，后续实现必须将相应 `[ ]` 更新为 `[x]`，并附上代码位置和测试命令。