# LangChainGo RAG Demo

基于 [langchaingo](https://github.com/tmc/langchaingo) 的 RAG（检索增强生成）演示包，位于 `third_party/ragdemo`，与主服务解耦，可独立运行验证切块、嵌入、召回流程。

当前主项目 **没有** 完整的入库 RAG 平台（见 `docs/arch/RAG流程和评估.md`）；本包提供可复用的 Go 侧参考实现。

## 能力概览

| 阶段 | 实现 | 关键选项 |
|------|------|----------|
| 加载 | `.txt` / `.md` 单文件或目录 | `documentloaders` |
| 切块 (Chunk) | `recursive` / `token` / `markdown` | `ChunkSize`, `ChunkOverlap` |
| 嵌入 (Embed) | `hash` / `openai` / `ollama` | 模型名、API 地址 |
| 存储 | **Milvus Lite** 持久化（默认 `.myagent/milvus.db`） | `Store`, `MilvusDBPath`, `MilvusCollection` |
| 召回 (Retrieval) | **rule + dense → RRF → rerank** | `RuleTopK`, `DenseTopK`, `RRFK`, `DisableRerank` |
| 生成 (可选) | `chains.RetrievalQA` | 任意 langchaingo LLM |

## 快速开始（Milvus Lite 持久化，无需 API Key）

默认写入 `.myagent/milvus.db`（与 MCP milvus 配置路径一致）：

```bash
go run ./third_party/ragdemo/cmd/ragdemo \
  --file third_party/ragdemo/testdata/sample.md \
  --query "RAG 有哪些切块策略"
```

纯内存模式（不落盘，适合快速验证）：

```bash
go run ./third_party/ragdemo/cmd/ragdemo \
  --store memory \
  --file third_party/ragdemo/testdata/sample.md \
  --query "RAG 有哪些切块策略"
```

## 使用 OpenAI 嵌入 + 问答

```bash
export OPENAI_API_KEY=sk-...
go run ./third_party/ragdemo/cmd/ragdemo \
  --embedder openai \
  --chunk markdown \
  --file docs/arch/RAG流程和评估.md \
  --query "当前项目 RAG 缺什么" \
  --ask
```

## 使用 Ollama 本地嵌入

```bash
# 先拉取 embedding 模型：ollama pull nomic-embed-text
go run ./third_party/ragdemo/cmd/ragdemo \
  --embedder ollama \
  --ollama-model nomic-embed-text \
  --dir skills/rag-search \
  --query "Milvus MCP 怎么用"
```

## 切块策略说明

- **recursive**：通用文本，按段落/行/空格递归切分
- **token**：按 tiktoken 计数，适合与 GPT 上下文长度对齐
- **markdown**：按 `#` 标题层级切分，可保留标题路径（`WithHeadingHierarchy`）

## 召回管线

默认混合召回（见 `retriever.go`）：

1. **rule**：内存 BM25-lite 词法匹配
2. **dense**：Milvus 向量 Top-K
3. **RRF**：Reciprocal Rank Fusion 融合（`RRFK` 默认 60）
4. **rerank**：OverlapReranker 精排（查询词覆盖率 + RRF 分数）

可用 `RetrieveHits()` 获取每条的 `Method` 与 `recall_methods` 元数据。

## 文档解析（runtime 层）

PDF/TXT/MD 解析不在 ragdemo，而在 `internal/data/agent_runtime/rag`：

- `.pdf` → MinerU（`internal/data/mineru`）
- `.txt` / `.md` → 直接读取

Runtime 通过 `ParseDocument(ctx, relPath)` 暴露解析能力。

## 召回方式说明

- **similarity / threshold**：仅作用于 dense 路
- **DisableRerank**：跳过 rerank，仅 RRF 融合

生产环境常见扩展：cross-encoder rerank、Milvus hybrid search、MMR 去重。

## 与主项目集成

- 默认 Milvus Lite db 路径与 `internal/conf/default_config.yaml` 中 MCP `--milvus-uri` 一致
- 切换 embedding 模型维度时，可加 `--milvus-drop-old` 重建 collection
- 评估可配合 `third_party/ragas-eval/`
- Agent 侧已有 `skills/rag-search` 指引 MCP 检索
