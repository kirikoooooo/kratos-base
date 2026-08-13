package ragdemo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/schema"
	"kratos-demo/third_party/ragdemo"
)

func TestHybridRetrieverRuleAndRRF(t *testing.T) {
	t.Parallel()

	idx := ragdemo.NewLexicalIndex()
	doc := schema.Document{
		PageContent: "向量召回常用余弦相似度 Top-K 融合排序",
		Metadata:    map[string]any{"chunk_id": "c1", "source": "test"},
	}
	idx.Add(doc)

	rule := ragdemo.NewRuleRetriever(idx)
	hits, err := rule.Retrieve(context.Background(), "余弦相似度 Top-K", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected rule hits")
	}

	fused := ragdemo.FuseRRF([][]ragdemo.Hit{hits}, ragdemo.RRFConfig{K: 60})
	if len(fused) != 1 || fused[0].Method != ragdemo.MethodRRF {
		t.Fatalf("unexpected fuse result: %+v", fused)
	}
}

func TestOverlapReranker(t *testing.T) {
	t.Parallel()
	reranker := ragdemo.OverlapReranker{}
	hits := []ragdemo.Hit{
		{Document: schema.Document{PageContent: "alpha beta", Metadata: map[string]any{"chunk_id": "a"}}, Score: 0.5, Method: ragdemo.MethodRRF, Rank: 2},
		{Document: schema.Document{PageContent: "beta gamma delta", Metadata: map[string]any{"chunk_id": "b"}}, Score: 0.4, Method: ragdemo.MethodRRF, Rank: 1},
	}
	out, err := reranker.Rerank(context.Background(), "beta", hits, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(out))
	}
	if out[0].Method != ragdemo.MethodRerank {
		t.Fatalf("method = %s", out[0].Method)
	}
}

func TestRAGIngestAndRetrieve(t *testing.T) {
	t.Parallel()

	rag, err := ragdemo.New(ragdemo.Config{
		Store:         ragdemo.StoreMemory,
		Embedder:      ragdemo.EmbedderHash,
		ChunkStrategy: ragdemo.ChunkRecursive,
		ChunkSize:     200,
		ChunkOverlap:  20,
		TopK:          2,
	})
	if err != nil {
		t.Fatalf("new rag: %v", err)
	}

	ctx := context.Background()
	content := strings.Join([]string{
		"LangChainGo 是 LangChain 的 Go 语言实现。",
		"RAG 流程包含文档加载、切块、向量化、检索和生成。",
		"向量召回常用余弦相似度 Top-K。",
		"Markdown 切块适合技术文档。",
	}, "\n")
	n, err := rag.IngestText(ctx, content, map[string]any{"source": "test"})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n == 0 {
		t.Fatal("expected chunks")
	}

	hits, err := rag.RetrieveHits(ctx, "向量召回 Top-K")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected retrieved docs")
	}
	found := false
	for _, hit := range hits {
		if strings.Contains(hit.Document.PageContent, "余弦相似度") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected relevant chunk, got: %+v", hits)
	}
}

func TestFormatSources(t *testing.T) {
	t.Parallel()
	out := ragdemo.FormatSources([]schema.Document{
		{PageContent: "hello", Score: 0.9, Metadata: map[string]any{"source": "a.md"}},
	})
	if !strings.Contains(out, "hello") || !strings.Contains(out, "a.md") {
		t.Fatalf("unexpected format: %q", out)
	}
}
