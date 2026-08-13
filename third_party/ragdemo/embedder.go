package ragdemo

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"strings"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// newEmbedder 创建嵌入模型。生产环境请使用 OpenAI 或 Ollama；Hash 仅供离线测试。
func newEmbedder(cfg Config) (embeddings.Embedder, error) {
	switch cfg.Embedder {
	case EmbedderOpenAI:
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for openai embedder")
		}
		llm, err := openai.New(
			openai.WithToken(apiKey),
			openai.WithEmbeddingModel(cfg.OpenAIModel),
		)
		if err != nil {
			return nil, fmt.Errorf("create openai embedder: %w", err)
		}
		return embeddings.NewEmbedder(llm)

	case EmbedderOllama:
		llm, err := ollama.New(
			ollama.WithModel(cfg.OllamaModel),
			ollama.WithServerURL(cfg.OllamaHost),
		)
		if err != nil {
			return nil, fmt.Errorf("create ollama embedder: %w", err)
		}
		return embeddings.NewEmbedder(llm)

	case EmbedderHash:
		return newHashEmbedder(cfg.HashDimensions), nil

	default:
		return nil, fmt.Errorf("unknown embedder provider: %q", cfg.Embedder)
	}
}

// hashEmbedder 将文本映射到固定维度的确定性向量，便于无 API Key 的单元测试。
// 注意：不具备真实语义相似度，不可用于生产召回质量评估。
type hashEmbedder struct {
	dim int
}

func newHashEmbedder(dim int) embeddings.Embedder {
	client := embeddings.EmbedderClientFunc(func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i, text := range texts {
			out[i] = hashTextToVector(text, dim)
		}
		return out, nil
	})
	emb, _ := embeddings.NewEmbedder(client)
	return emb
}

func hashTextToVector(text string, dim int) []float32 {
	vec := make([]float32, dim)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum64() % uint64(dim))
		vec[idx] += 1
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm == 0 {
		return vec
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec
}
