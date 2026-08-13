package ragdemo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	milvuslite "github.com/lyyyuna/milvus-lite-go/v2"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/vectorstores"
	milvusv2 "github.com/tmc/langchaingo/vectorstores/milvus/v2"
)

// newMilvusLiteStore 启动 Milvus Lite 子进程并创建 langchaingo Milvus 向量库。
// db 文件默认与项目 MCP 配置一致（.myagent/milvus.db），数据持久化到本地。
func newMilvusLiteStore(ctx context.Context, cfg Config, embedder embeddings.Embedder) (vectorstores.VectorStore, *milvuslite.Server, error) {
	dbPath, err := resolveMilvusDBPath(cfg.MilvusDBPath)
	if err != nil {
		return nil, nil, err
	}

	server, err := milvuslite.Start(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("start milvus lite: %w", err)
	}

	opts := []milvusv2.Option{
		milvusv2.WithEmbedder(embedder),
		milvusv2.WithCollectionName(cfg.MilvusCollection),
		// 余弦相似度：与大多数 embedding 模型匹配；可选 entity.L2 / entity.IP
		milvusv2.WithMetricType(entity.COSINE),
		milvusv2.WithIndex(index.NewAutoIndex(entity.COSINE)),
	}
	if cfg.MilvusDropOld {
		opts = append(opts, milvusv2.WithDropOld())
	}

	store, err := milvusv2.New(ctx, milvusclient.ClientConfig{Address: server.Addr()}, opts...)
	if err != nil {
		_ = server.Stop()
		return nil, nil, fmt.Errorf("create milvus store: %w", err)
	}
	return store, server, nil
}

func resolveMilvusDBPath(path string) (string, error) {
	if path == "" {
		path = defaultMilvusDBPath
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Join(wd, path), nil
}
