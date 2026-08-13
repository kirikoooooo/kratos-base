package ragdemo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
	"github.com/tmc/langchaingo/vectorstores"
)

// RAG 封装切块、入库与混合召回管线。文档解析由 internal/data/agent_runtime/rag 负责。
type RAG struct {
	cfg          Config
	splitter     textsplitter.TextSplitter
	store        vectorstores.VectorStore
	lexical      *LexicalIndex
	hybrid       *HybridRetriever
	schemaRet    schemaRetriever
	milvusServer interface{ Stop() error }
}

// New 构建 RAG 实例。默认 Milvus Lite + 混合召回（rule + dense + RRF + rerank）。
func New(cfg Config) (*RAG, error) {
	return NewWithContext(context.Background(), cfg)
}

// NewWithContext 构建 RAG 实例。
func NewWithContext(ctx context.Context, cfg Config) (*RAG, error) {
	cfg = cfg.withDefaults()

	splitter, err := newTextSplitter(cfg)
	if err != nil {
		return nil, err
	}
	embedder, err := newEmbedder(cfg)
	if err != nil {
		return nil, err
	}

	store, milvusServer, err := newVectorStore(ctx, cfg, embedder)
	if err != nil {
		return nil, err
	}

	lexical := NewLexicalIndex()
	dense := NewDenseRetriever(store, searchOptionsFromCfg(cfg)...)
	rule := NewRuleRetriever(lexical)
	hybrid := NewHybridRetriever(rule, dense, HybridRetrieverConfig{
		FinalTopK: cfg.TopK,
		RuleTopK:  cfg.RuleTopK,
		DenseTopK: cfg.DenseTopK,
		RRF:       RRFConfig{K: cfg.RRFK},
		Reranker:  rerankerFromCfg(cfg),
	})

	return &RAG{
		cfg:          cfg,
		splitter:     splitter,
		store:        store,
		lexical:      lexical,
		hybrid:       hybrid,
		schemaRet:    schemaRetriever{hybrid: hybrid},
		milvusServer: milvusServer,
	}, nil
}

func rerankerFromCfg(cfg Config) Reranker {
	if cfg.DisableRerank {
		return nil
	}
	return OverlapReranker{}
}

// Close 停止 Milvus Lite 子进程。
func (r *RAG) Close() error {
	if r == nil || r.milvusServer == nil {
		return nil
	}
	return r.milvusServer.Stop()
}

func newVectorStore(ctx context.Context, cfg Config, embedder embeddings.Embedder) (vectorstores.VectorStore, interface{ Stop() error }, error) {
	switch cfg.Store {
	case StoreMemory:
		return NewInMemoryStore(embedder), nil, nil
	case StoreMilvusLite:
		store, server, err := newMilvusLiteStore(ctx, cfg, embedder)
		return store, server, err
	default:
		return nil, nil, fmt.Errorf("unknown store backend: %q", cfg.Store)
	}
}

// IngestText 将原始文本切块后写入稠密库与词法索引。
func (r *RAG) IngestText(ctx context.Context, content string, metadata map[string]any) (int, error) {
	if strings.TrimSpace(content) == "" {
		return 0, fmt.Errorf("empty content")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	chunks, err := textsplitter.CreateDocuments(r.splitter, []string{content}, []map[string]any{metadata})
	if err != nil {
		return 0, fmt.Errorf("split text: %w", err)
	}
	return r.indexChunks(ctx, chunks)
}

// IngestFile 加载 .txt / .md 并切块入库（PDF 请先经 runtime DocumentParser 解析）。
func (r *RAG) IngestFile(ctx context.Context, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	loader := documentloaders.NewText(f)
	docs, err := loader.LoadAndSplit(ctx, r.splitter)
	if err != nil {
		return 0, fmt.Errorf("load and split: %w", err)
	}
	for i := range docs {
		if docs[i].Metadata == nil {
			docs[i].Metadata = map[string]any{}
		}
		docs[i].Metadata["source"] = path
	}
	return r.indexChunks(ctx, docs)
}

// IngestDir 递归加载目录下 .txt / .md。
func (r *RAG) IngestDir(ctx context.Context, dir string) (int, error) {
	loader := documentloaders.NewRecursiveDirLoader(
		documentloaders.WithRoot(dir),
		documentloaders.WithAllowExts("txt", "md"),
	)
	docs, err := loader.LoadAndSplit(ctx, r.splitter)
	if err != nil {
		return 0, fmt.Errorf("load directory: %w", err)
	}
	for i := range docs {
		if docs[i].Metadata == nil {
			docs[i].Metadata = map[string]any{}
		}
	}
	return r.indexChunks(ctx, docs)
}

func (r *RAG) indexChunks(ctx context.Context, chunks []schema.Document) (int, error) {
	for i := range chunks {
		ensureChunkID(&chunks[i])
		r.lexical.Add(chunks[i])
	}
	if _, err := r.store.AddDocuments(ctx, chunks); err != nil {
		return 0, fmt.Errorf("add documents: %w", err)
	}
	return len(chunks), nil
}

// Retrieve 混合召回：rule + dense → RRF → rerank。
func (r *RAG) Retrieve(ctx context.Context, query string) ([]schema.Document, error) {
	hits, err := r.RetrieveHits(ctx, query)
	if err != nil {
		return nil, err
	}
	docs := make([]schema.Document, len(hits))
	for i, hit := range hits {
		docs[i] = hit.Document
	}
	return docs, nil
}

// RetrieveHits 返回带召回方法/分数的详细结果。
func (r *RAG) RetrieveHits(ctx context.Context, query string) ([]Hit, error) {
	return r.hybrid.Retrieve(ctx, query)
}

func searchOptionsFromCfg(cfg Config) []vectorstores.Option {
	if cfg.RetrievalMode == RetrievalThreshold {
		return []vectorstores.Option{
			vectorstores.WithScoreThreshold(cfg.ScoreThreshold),
		}
	}
	return nil
}

// Ask 使用 RetrievalQA 链生成答案。
func (r *RAG) Ask(ctx context.Context, llm llms.Model, question string) (string, []schema.Document, error) {
	qa := chains.NewRetrievalQAFromLLM(llm, r.schemaRet)
	qa.ReturnSourceDocuments = true

	result, err := chains.Call(ctx, qa, map[string]any{
		"query": question,
	})
	if err != nil {
		return "", nil, fmt.Errorf("retrieval qa: %w", err)
	}

	var sources []schema.Document
	if raw, ok := result["source_documents"].([]schema.Document); ok {
		sources = raw
	}
	text, _ := result["text"].(string)
	return text, sources, nil
}

// FormatSources 格式化召回片段；metadata 中 recall_methods 记录融合来源。
func FormatSources(docs []schema.Document) string {
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, doc := range docs {
		source, _ := doc.Metadata["source"].(string)
		if source == "" {
			source = filepath.Base(fmt.Sprintf("%v", doc.Metadata["source"]))
		}
		if source == "" || source == "." {
			source = "unknown"
		}
		methods := ""
		if raw, ok := doc.Metadata["recall_methods"].([]string); ok && len(raw) > 0 {
			methods = ", methods=" + strings.Join(raw, "+")
		}
		fmt.Fprintf(&b, "--- 片段 %d (score=%.3f, source=%s%s) ---\n%s\n\n", i+1, doc.Score, source, methods, doc.PageContent)
	}
	return strings.TrimSpace(b.String())
}
