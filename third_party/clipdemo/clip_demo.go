// Package clipdemo is an isolated LangChainGo CLIP image-text similarity demo.
package clipdemo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/tmc/langchaingo/embeddings"
)

// Config points at the local CLIP demo service. Both endpoints use the same
// CLIP checkpoint, so their vectors are in a shared image-text space.
type Config struct {
	BaseURL string
	APIKey  string // Optional Bearer token for a protected local deployment.
}

type client struct {
	config     Config
	httpClient *http.Client
}

// CompareImageToTexts encodes image and candidates with the same CLIP model.
// Text embedding is exposed through LangChainGo's EmbedderClient adapter.
func CompareImageToTexts(ctx context.Context, cfg Config, imagePath string, candidates []string) ([]float64, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("CLIP_BASE_URL is required")
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("at least one candidate is required")
	}
	c := &client{config: cfg, httpClient: http.DefaultClient}
	embedder, err := embeddings.NewEmbedder(embeddings.EmbedderClientFunc(c.embedTexts))
	if err != nil {
		return nil, fmt.Errorf("create LangChainGo embedder: %w", err)
	}
	textVectors, err := embedder.EmbedDocuments(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("embed CLIP text: %w", err)
	}
	imageVector, err := c.embedImage(ctx, imagePath)
	if err != nil {
		return nil, err
	}
	scores := make([]float64, len(candidates))
	for i, textVector := range textVectors {
		score, err := CosineSimilarity(imageVector, textVector)
		if err != nil {
			return nil, fmt.Errorf("candidate %d: %w", i, err)
		}
		scores[i] = score
	}
	return scores, nil
}

func (c *client) embedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	var response embeddingResponse
	if err := c.post(ctx, "/embed/text", map[string]any{"texts": texts}, &response); err != nil {
		return nil, err
	}
	return response.Embeddings, nil
}

func (c *client) embedImage(ctx context.Context, imagePath string) ([]float32, error) {
	raw, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("read image %s: %w", imagePath, err)
	}
	var response imageEmbeddingResponse
	if err := c.post(ctx, "/embed/image", map[string]string{"image_base64": base64.StdEncoding.EncodeToString(raw)}, &response); err != nil {
		return nil, err
	}
	return response.Embedding, nil
}

func (c *client) post(ctx context.Context, path string, request, response any) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.config.BaseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.config.APIKey); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call CLIP service: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CLIP service %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("decode CLIP service response: %w", err)
	}
	return nil
}

type embeddingResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}
type imageEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

func CosineSimilarity(left, right []float32) (float64, error) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, fmt.Errorf("vectors must be non-empty and have equal dimensions")
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l, r := float64(left[i]), float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, fmt.Errorf("zero vector has no cosine similarity")
	}
	return dot / math.Sqrt(leftNorm*rightNorm), nil
}
