// Package ragdemo 提供基于 LangChainGo 的 RAG（检索增强生成）演示实现。
//
// 典型流程：加载文档 → 切块（chunk）→ 向量化（embed）→ 入库 → 相似度召回 →（可选）LLM 生成答案。
package ragdemo

// ChunkStrategy 文档切块策略。
//
// 可选值说明：
//   - ChunkRecursive：递归字符切块，按 "\n\n" → "\n" → " " 逐级分隔，适合通用纯文本/代码；
//   - ChunkToken：按 tiktoken 计数切块，与 OpenAI 模型 token 对齐，适合严格控制上下文长度；
//   - ChunkMarkdown：按 Markdown 标题层级切块，可保留标题路径，适合技术文档/笔记。
type ChunkStrategy string

const (
	ChunkRecursive ChunkStrategy = "recursive"
	ChunkToken     ChunkStrategy = "token"
	ChunkMarkdown  ChunkStrategy = "markdown"
)

// RetrievalMode 向量召回方式。
//
// 可选值说明：
//   - RetrievalSimilarity：余弦相似度 Top-K，最常用，按相似度降序返回前 K 条；
//   - RetrievalThreshold：Top-K 后再按 ScoreThreshold 过滤低分片段（0~1，越高越严格）。
//
// 生产环境还可扩展：MMR（最大边际相关性，去重多样化）、Hybrid（向量 + BM25 混合）、
// 以及带 metadata 过滤的 filtered search（langchaingo 通过 vectorstores.WithFilters 支持）。
type RetrievalMode string

const (
	RetrievalSimilarity RetrievalMode = "similarity"
	RetrievalThreshold  RetrievalMode = "threshold"
)

// StoreBackend 向量库存储后端。
//
// 可选值说明：
//   - StoreMilvusLite：嵌入式 Milvus Lite，数据持久化到本地 .db 文件（默认，与 MCP milvus 配置路径一致）；
//   - StoreMemory：纯内存余弦检索，仅适合单元测试或无持久化需求的快速验证。
type StoreBackend string

const (
	StoreMilvusLite StoreBackend = "milvus"
	StoreMemory     StoreBackend = "memory"

	// defaultMilvusDBPath 与 internal/conf/default_config.yaml 中 MCP milvus 的 --milvus-uri 保持一致。
	defaultMilvusDBPath = ".myagent/milvus.db"
)

// EmbedderProvider 嵌入模型来源。
//
// 可选值说明：
//   - EmbedderOpenAI：OpenAI text-embedding-* 系列，需 OPENAI_API_KEY；
//   - EmbedderOllama：本地 Ollama embedding 模型，需 OLLAMA_HOST 可访问；
//   - EmbedderHash：确定性哈希向量，仅用于离线演示/单元测试，无语义能力。
type EmbedderProvider string

const (
	EmbedderOpenAI EmbedderProvider = "openai"
	EmbedderOllama EmbedderProvider = "ollama"
	EmbedderHash   EmbedderProvider = "hash"
)

// Config RAG 管线配置。零值字段会在 New 时被替换为合理默认值。
type Config struct {
	// --- 切块（Chunk）选项 ---
	ChunkStrategy ChunkStrategy // 切块策略，默认 ChunkRecursive
	ChunkSize     int           // 单块最大长度（字符或 token，取决于策略），默认 512
	ChunkOverlap  int           // 相邻块重叠长度，避免语义截断，默认 100

	// --- 召回（Retrieval）选项 ---
	RetrievalMode  RetrievalMode // 遗留：仅 dense 单路时使用
	TopK           int           // 最终返回条数，默认 4
	ScoreThreshold float32       // 稠密路相似度下限（0~1）
	RuleTopK       int           // 规则路预召回数，默认 TopK*2
	DenseTopK      int           // 稠密路预召回数，默认 TopK*2
	RRFK           int           // RRF 常数 k，默认 60
	DisableRerank  bool          // 为 true 时跳过 rerank 阶段

	// --- 存储（Vector Store）选项 ---
	Store           StoreBackend // 向量库后端，默认 StoreMilvusLite
	MilvusDBPath    string       // Milvus Lite 本地 db 文件路径，默认 .myagent/milvus.db
	MilvusCollection string      // collection 名称，默认 ragdemo
	MilvusDropOld   bool         // 启动时删除并重建 collection（切换 embedding 维度时需开启）

	// --- 嵌入（Embedding）选项 ---
	Embedder EmbedderProvider // 嵌入提供方，默认 EmbedderHash
	// OpenAIModel 如 text-embedding-3-small / text-embedding-ada-002
	OpenAIModel string
	// OllamaModel 如 nomic-embed-text / mxbai-embed-large
	OllamaModel string
	// OllamaHost 默认 http://127.0.0.1:11434
	OllamaHost string
	// HashDimensions 哈希嵌入维度，仅 EmbedderHash 使用，默认 256
	HashDimensions int
}

func (c *Config) withDefaults() Config {
	out := *c
	if out.ChunkStrategy == "" {
		out.ChunkStrategy = ChunkRecursive
	}
	if out.ChunkSize <= 0 {
		out.ChunkSize = 512
	}
	if out.ChunkOverlap <= 0 {
		out.ChunkOverlap = 100
	}
	if out.RetrievalMode == "" {
		out.RetrievalMode = RetrievalSimilarity
	}
	if out.TopK <= 0 {
		out.TopK = 4
	}
	if out.RuleTopK <= 0 {
		out.RuleTopK = out.TopK * 2
	}
	if out.DenseTopK <= 0 {
		out.DenseTopK = out.TopK * 2
	}
	if out.RRFK <= 0 {
		out.RRFK = 60
	}
	if out.ScoreThreshold <= 0 {
		out.ScoreThreshold = 0.5
	}
	if out.Store == "" {
		out.Store = StoreMilvusLite
	}
	if out.MilvusDBPath == "" {
		out.MilvusDBPath = defaultMilvusDBPath
	}
	if out.MilvusCollection == "" {
		out.MilvusCollection = "ragdemo"
	}
	if out.Embedder == "" {
		out.Embedder = EmbedderHash
	}
	if out.OpenAIModel == "" {
		out.OpenAIModel = "text-embedding-3-small"
	}
	if out.OllamaModel == "" {
		out.OllamaModel = "nomic-embed-text"
	}
	if out.OllamaHost == "" {
		out.OllamaHost = "http://127.0.0.1:11434"
	}
	if out.HashDimensions <= 0 {
		out.HashDimensions = 256
	}
	return out
}
