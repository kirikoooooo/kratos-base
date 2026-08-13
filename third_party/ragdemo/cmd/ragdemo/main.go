package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms/openai"
	"kratos-demo/third_party/ragdemo"
)

func main() {
	var (
		filePath         = flag.String("file", "", "入库单个 .txt/.md 文件")
		dirPath          = flag.String("dir", "", "递归入库目录")
		query            = flag.String("query", "", "检索问题（必填）")
		store            = flag.String("store", "milvus", "存储后端: milvus | memory")
		milvusDB         = flag.String("milvus-db", ".myagent/milvus.db", "Milvus Lite 本地 db 路径")
		milvusCollection = flag.String("milvus-collection", "ragdemo", "Milvus collection 名称")
		milvusDropOld    = flag.Bool("milvus-drop-old", false, "启动时删除并重建 collection")
		embedder         = flag.String("embedder", "hash", "嵌入来源: hash | openai | ollama")
		chunk       = flag.String("chunk", "recursive", "切块策略: recursive | token | markdown")
		chunkSize   = flag.Int("chunk-size", 512, "块大小")
		overlap     = flag.Int("overlap", 100, "块重叠")
		topK        = flag.Int("topk", 4, "召回条数")
		threshold   = flag.Float64("threshold", 0.5, "相似度阈值（threshold 模式）")
		retrieval   = flag.String("retrieval", "similarity", "召回模式: similarity | threshold")
		ollamaModel = flag.String("ollama-model", "nomic-embed-text", "Ollama embedding 模型")
		ask         = flag.Bool("ask", false, "启用 LLM 问答（需 OPENAI_API_KEY）")
	)
	flag.Parse()

	if strings.TrimSpace(*query) == "" {
		fmt.Fprintln(os.Stderr, "usage: ragdemo --query <question> [--file path | --dir path]")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if *filePath == "" && *dirPath == "" {
		*filePath = "third_party/ragdemo/testdata/sample.md"
	}

	cfg := ragdemo.Config{
		Store:            ragdemo.StoreBackend(*store),
		MilvusDBPath:     *milvusDB,
		MilvusCollection: *milvusCollection,
		MilvusDropOld:    *milvusDropOld,
		Embedder:         ragdemo.EmbedderProvider(*embedder),
		ChunkStrategy:  ragdemo.ChunkStrategy(*chunk),
		ChunkSize:      *chunkSize,
		ChunkOverlap:   *overlap,
		TopK:           *topK,
		RetrievalMode:  ragdemo.RetrievalMode(*retrieval),
		ScoreThreshold: float32(*threshold),
		OllamaModel:    *ollamaModel,
	}

	rag, err := ragdemo.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rag.Close()

	ctx := context.Background()
	switch {
	case *filePath != "":
		n, err := rag.IngestFile(ctx, *filePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "ingested %d chunks from %s\n", n, *filePath)
	case *dirPath != "":
		n, err := rag.IngestDir(ctx, *dirPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "ingested %d chunks from %s\n", n, *dirPath)
	}

	if *ask {
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "OPENAI_API_KEY is required for --ask")
			os.Exit(1)
		}
		llm, err := openai.New(openai.WithToken(apiKey))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		answer, sources, err := rag.Ask(ctx, llm, *query)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("=== Answer ===")
		fmt.Println(answer)
		fmt.Println("\n=== Sources ===")
		fmt.Println(ragdemo.FormatSources(sources))
		return
	}

	docs, err := rag.Retrieve(ctx, *query)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(ragdemo.FormatSources(docs))
}
