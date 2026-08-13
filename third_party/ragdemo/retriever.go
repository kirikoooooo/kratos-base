package ragdemo

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

// RecallMethod 单路召回或融合阶段标识，便于审计与 RAGAS 分析。
type RecallMethod string

const (
	MethodRule   RecallMethod = "rule"
	MethodDense  RecallMethod = "dense"
	MethodRRF    RecallMethod = "rrf"
	MethodRerank RecallMethod = "rerank"
)

// Hit 单条召回结果，保留融合前的来源与方法。
type Hit struct {
	Document schema.Document
	Score    float32
	Method   RecallMethod
	Rank     int
}

// ChunkRetriever 单路召回器接口。
type ChunkRetriever interface {
	Retrieve(ctx context.Context, query string, topK int) ([]Hit, error)
	Name() RecallMethod
}

// LexicalIndex 规则/词法召回索引（内存 BM25-lite），与 Milvus 稠密索引互补。
type LexicalIndex struct {
	mu      sync.RWMutex
	entries []lexEntry
}

type lexEntry struct {
	id   string
	doc  schema.Document
	tf   map[string]float64
	len  float64
}

// NewLexicalIndex 创建空词法索引。
func NewLexicalIndex() *LexicalIndex {
	return &LexicalIndex{}
}

// Add 追加文档块到词法索引。
func (idx *LexicalIndex) Add(doc schema.Document) {
	id := chunkID(doc)
	tokens := tokenize(doc.PageContent)
	tf := termFreq(tokens)
	var length float64
	for _, v := range tf {
		length += v
	}
	idx.mu.Lock()
	idx.entries = append(idx.entries, lexEntry{id: id, doc: doc, tf: tf, len: length})
	idx.mu.Unlock()
}

// RuleRetriever 基于词法匹配的规则召回（BM25-lite）。
type RuleRetriever struct {
	index *LexicalIndex
}

func NewRuleRetriever(index *LexicalIndex) *RuleRetriever {
	return &RuleRetriever{index: index}
}

func (r *RuleRetriever) Name() RecallMethod { return MethodRule }

func (r *RuleRetriever) Retrieve(ctx context.Context, query string, topK int) ([]Hit, error) {
	_ = ctx
	if topK <= 0 || r.index == nil {
		return nil, nil
	}
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}
	qtf := termFreq(queryTokens)

	r.index.mu.RLock()
	defer r.index.mu.RUnlock()
	if len(r.index.entries) == 0 {
		return nil, nil
	}

	avgLen := 0.0
	for _, e := range r.index.entries {
		avgLen += e.len
	}
	avgLen /= float64(len(r.index.entries))

	type scored struct {
		hit Hit
	}
	scoredHits := make([]scored, 0, len(r.index.entries))
	k1, b := 1.5, 0.75
	for _, entry := range r.index.entries {
		var score float64
		for term, qf := range qtf {
			tf := entry.tf[term]
			if tf == 0 {
				continue
			}
			// 平滑 IDF，小语料下避免负值
			idf := math.Log(1 + (float64(len(r.index.entries)-docFreq(r.index.entries, term))+0.5)/(float64(docFreq(r.index.entries, term))+0.5))
			denom := tf + k1*(1-b+b*entry.len/avgLen)
			score += idf * (tf * (k1 + 1)) / denom * qf
		}
		if score <= 0 {
			continue
		}
		doc := entry.doc
		doc.Score = float32(score)
		scoredHits = append(scoredHits, scored{hit: Hit{Document: doc, Score: float32(score), Method: MethodRule}})
	}
	sort.Slice(scoredHits, func(i, j int) bool {
		return scoredHits[i].hit.Score > scoredHits[j].hit.Score
	})
	if len(scoredHits) > topK {
		scoredHits = scoredHits[:topK]
	}
	out := make([]Hit, len(scoredHits))
	for i, item := range scoredHits {
		item.hit.Rank = i + 1
		out[i] = item.hit
	}
	return out, nil
}

// DenseRetriever 稠密向量召回，包装 langchaingo VectorStore。
type DenseRetriever struct {
	store   vectorstores.VectorStore
	options []vectorstores.Option
}

func NewDenseRetriever(store vectorstores.VectorStore, opts ...vectorstores.Option) *DenseRetriever {
	return &DenseRetriever{store: store, options: opts}
}

func (r *DenseRetriever) Name() RecallMethod { return MethodDense }

func (r *DenseRetriever) Retrieve(ctx context.Context, query string, topK int) ([]Hit, error) {
	if topK <= 0 || r.store == nil {
		return nil, nil
	}
	docs, err := r.store.SimilaritySearch(ctx, query, topK, r.options...)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, len(docs))
	for i, doc := range docs {
		out[i] = Hit{Document: doc, Score: doc.Score, Method: MethodDense, Rank: i + 1}
	}
	return out, nil
}

// RRFConfig Reciprocal Rank Fusion 参数。
// RRFScore(d) = Σ 1/(k + rank_i(d))；k 越大，排名靠后的文档仍有权重（常用 60）。
type RRFConfig struct {
	K int // 默认 60
}

func (c RRFConfig) withDefaults() RRFConfig {
	if c.K <= 0 {
		c.K = 60
	}
	return c
}

// FuseRRF 将多路召回列表融合为统一排序。相同 chunk_id 合并分数。
func FuseRRF(lists [][]Hit, cfg RRFConfig) []Hit {
	cfg = cfg.withDefaults()
	scores := map[string]float64{}
	docs := map[string]schema.Document{}
	methods := map[string]map[RecallMethod]struct{}{}

	for _, list := range lists {
		for i, hit := range list {
			id := chunkID(hit.Document)
			rank := hit.Rank
			if rank <= 0 {
				rank = i + 1
			}
			scores[id] += 1.0 / (float64(cfg.K) + float64(rank))
			if _, ok := docs[id]; !ok {
				docs[id] = hit.Document
			}
			if methods[id] == nil {
				methods[id] = map[RecallMethod]struct{}{}
			}
			methods[id][hit.Method] = struct{}{}
		}
	}

	type scored struct {
		id    string
		score float64
	}
	ordered := make([]scored, 0, len(scores))
	for id, score := range scores {
		ordered = append(ordered, scored{id: id, score: score})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].score > ordered[j].score
	})

	out := make([]Hit, len(ordered))
	for i, item := range ordered {
		doc := docs[item.id]
		doc.Score = float32(item.score)
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		doc.Metadata["recall_methods"] = methodSet(methods[item.id])
		out[i] = Hit{Document: doc, Score: float32(item.score), Method: MethodRRF, Rank: i + 1}
	}
	return out
}

// Reranker 对融合结果精排。可替换为 cross-encoder / LLM reranker。
type Reranker interface {
	Rerank(ctx context.Context, query string, hits []Hit, topK int) ([]Hit, error)
}

// OverlapReranker 轻量精排：RRF 分数 + 查询词覆盖率的线性组合（无需额外模型）。
type OverlapReranker struct {
	RRFWeight    float64 // 默认 0.7
	OverlapWeight float64 // 默认 0.3
}

func (r OverlapReranker) withDefaults() OverlapReranker {
	if r.RRFWeight <= 0 && r.OverlapWeight <= 0 {
		r.RRFWeight = 0.7
		r.OverlapWeight = 0.3
	}
	return r
}

func (r OverlapReranker) Rerank(ctx context.Context, query string, hits []Hit, topK int) ([]Hit, error) {
	_ = ctx
	r = r.withDefaults()
	if len(hits) == 0 {
		return nil, nil
	}
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		if topK > 0 && len(hits) > topK {
			return hits[:topK], nil
		}
		return hits, nil
	}
	qset := map[string]struct{}{}
	for _, t := range queryTokens {
		qset[t] = struct{}{}
	}

	type scored struct {
		hit   Hit
		score float64
	}
	scoredHits := make([]scored, len(hits))
	for i, hit := range hits {
		contentTokens := tokenize(hit.Document.PageContent)
		if len(contentTokens) == 0 {
			scoredHits[i] = scored{hit: hit, score: float64(hit.Score) * r.RRFWeight}
			continue
		}
		match := 0
		for _, t := range contentTokens {
			if _, ok := qset[t]; ok {
				match++
			}
		}
		overlap := float64(match) / float64(len(qset))
		score := float64(hit.Score)*r.RRFWeight + overlap*r.OverlapWeight
		scoredHits[i] = scored{hit: hit, score: score}
	}
	sort.Slice(scoredHits, func(i, j int) bool {
		return scoredHits[i].score > scoredHits[j].score
	})
	if topK > 0 && len(scoredHits) > topK {
		scoredHits = scoredHits[:topK]
	}
	out := make([]Hit, len(scoredHits))
	for i, item := range scoredHits {
		doc := item.hit.Document
		doc.Score = float32(item.score)
		out[i] = Hit{Document: doc, Score: float32(item.score), Method: MethodRerank, Rank: i + 1}
	}
	return out, nil
}

// HybridRetrieverConfig 混合召回管线配置。
type HybridRetrieverConfig struct {
	FinalTopK int // 最终返回条数
	RuleTopK  int // 规则路召回数（融合前），默认 FinalTopK*2
	DenseTopK int // 稠密路召回数（融合前），默认 FinalTopK*2
	RRF       RRFConfig
	Reranker  Reranker // 默认 OverlapReranker
}

func (c HybridRetrieverConfig) withDefaults(finalTopK int) HybridRetrieverConfig {
	if c.FinalTopK <= 0 {
		c.FinalTopK = finalTopK
	}
	if c.RuleTopK <= 0 {
		c.RuleTopK = c.FinalTopK * 2
	}
	if c.DenseTopK <= 0 {
		c.DenseTopK = c.FinalTopK * 2
	}
	c.RRF = c.RRF.withDefaults()
	if c.Reranker == nil {
		c.Reranker = OverlapReranker{}
	}
	return c
}

// HybridRetriever 混合召回：rule + dense → RRF 融合 → rerank。
type HybridRetriever struct {
	rule  ChunkRetriever
	dense ChunkRetriever
	cfg   HybridRetrieverConfig
}

// NewHybridRetriever 创建混合召回器。
func NewHybridRetriever(rule *RuleRetriever, dense *DenseRetriever, cfg HybridRetrieverConfig) *HybridRetriever {
	return &HybridRetriever{rule: rule, dense: dense, cfg: cfg}
}

// Retrieve 执行完整混合召回管线。
func (h *HybridRetriever) Retrieve(ctx context.Context, query string) ([]Hit, error) {
	cfg := h.cfg.withDefaults(4)
	var lists [][]Hit

	if h.rule != nil {
		ruleHits, err := h.rule.Retrieve(ctx, query, cfg.RuleTopK)
		if err != nil {
			return nil, err
		}
		lists = append(lists, ruleHits)
	}
	if h.dense != nil {
		denseHits, err := h.dense.Retrieve(ctx, query, cfg.DenseTopK)
		if err != nil {
			return nil, err
		}
		lists = append(lists, denseHits)
	}
	if len(lists) == 0 {
		return nil, nil
	}

	fused := FuseRRF(lists, cfg.RRF)
	if cfg.Reranker == nil {
		if len(fused) > cfg.FinalTopK {
			fused = fused[:cfg.FinalTopK]
		}
		return fused, nil
	}
	return cfg.Reranker.Rerank(ctx, query, fused, cfg.FinalTopK)
}

// schemaRetriever 适配 langchaingo chains.RetrievalQA。
type schemaRetriever struct {
	hybrid *HybridRetriever
}

func (r schemaRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	hits, err := r.hybrid.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}
	docs := make([]schema.Document, len(hits))
	for i, hit := range hits {
		docs[i] = hit.Document
	}
	return docs, nil
}

func chunkID(doc schema.Document) string {
	if doc.Metadata != nil {
		if id, ok := doc.Metadata["chunk_id"].(string); ok && id != "" {
			return id
		}
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(doc.PageContent)).String()
}

func ensureChunkID(doc *schema.Document) {
	if doc.Metadata == nil {
		doc.Metadata = map[string]any{}
	}
	if id, ok := doc.Metadata["chunk_id"].(string); !ok || id == "" {
		doc.Metadata["chunk_id"] = uuid.NewString()
	}
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			current.WriteRune(r)
		case unicode.Is(unicode.Han, r):
			flush()
			tokens = append(tokens, string(r))
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func termFreq(tokens []string) map[string]float64 {
	tf := map[string]float64{}
	for _, t := range tokens {
		tf[t]++
	}
	return tf
}

func docFreq(entries []lexEntry, term string) int {
	n := 0
	for _, e := range entries {
		if e.tf[term] > 0 {
			n++
		}
	}
	return n
}

func methodSet(m map[RecallMethod]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}
