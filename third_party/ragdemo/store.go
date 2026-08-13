package ragdemo

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

// storedDoc 内存向量库中的一条记录。
type storedDoc struct {
	id      string
	doc     schema.Document
	vector  []float32
}

// InMemoryStore 基于内存的余弦相似度向量库，实现 langchaingo vectorstores.VectorStore。
// 适合本地演示与小规模知识库；生产可替换为 Milvus/Chroma/PGVector 等（langchaingo 均有适配）。
type InMemoryStore struct {
	mu       sync.RWMutex
	embedder embeddings.Embedder
	records  []storedDoc
}

var _ vectorstores.VectorStore = (*InMemoryStore)(nil)

// NewInMemoryStore 创建内存向量库。
func NewInMemoryStore(embedder embeddings.Embedder) *InMemoryStore {
	return &InMemoryStore{embedder: embedder}
}

// AddDocuments 将文档切块内容向量化后写入内存索引。
func (s *InMemoryStore) AddDocuments(ctx context.Context, docs []schema.Document, _ ...vectorstores.Option) ([]string, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.PageContent
	}
	vectors, err := s.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed documents: %w", err)
	}
	if len(vectors) != len(docs) {
		return nil, fmt.Errorf("embedder returned %d vectors for %d documents", len(vectors), len(docs))
	}

	ids := make([]string, len(docs))
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, doc := range docs {
		id := uuid.NewString()
		ids[i] = id
		s.records = append(s.records, storedDoc{
			id:     id,
			doc:    doc,
			vector: vectors[i],
		})
	}
	return ids, nil
}

// SimilaritySearch 按查询文本做余弦相似度检索，返回 Top numDocuments 条。
// 可通过 vectorstores.WithScoreThreshold 设置最低相似度（与 chroma/milvus 行为一致，范围 0~1）。
func (s *InMemoryStore) SimilaritySearch(
	ctx context.Context,
	query string,
	numDocuments int,
	options ...vectorstores.Option,
) ([]schema.Document, error) {
	if numDocuments <= 0 {
		return nil, nil
	}
	opts := vectorstores.Options{}
	for _, opt := range options {
		opt(&opts)
	}
	embedder := s.embedder
	if opts.Embedder != nil {
		embedder = opts.Embedder
	}

	queryVec, err := embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.records) == 0 {
		return nil, nil
	}

	type scored struct {
		doc   schema.Document
		score float32
	}
	scoredDocs := make([]scored, 0, len(s.records))
	for _, rec := range s.records {
		score, err := cosineSimilarity(queryVec, rec.vector)
		if err != nil {
			continue
		}
		if opts.ScoreThreshold > 0 && score < opts.ScoreThreshold {
			continue
		}
		out := rec.doc
		out.Score = score
		scoredDocs = append(scoredDocs, scored{doc: out, score: score})
	}

	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})
	if len(scoredDocs) > numDocuments {
		scoredDocs = scoredDocs[:numDocuments]
	}

	result := make([]schema.Document, len(scoredDocs))
	for i, item := range scoredDocs {
		result[i] = item.doc
	}
	return result, nil
}

func cosineSimilarity(left, right []float32) (float32, error) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, fmt.Errorf("vectors must be non-empty and same length")
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l, r := float64(left[i]), float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, fmt.Errorf("zero vector")
	}
	return float32(dot / math.Sqrt(leftNorm*rightNorm)), nil
}
