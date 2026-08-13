package ragdemo

import (
	"fmt"

	"github.com/tmc/langchaingo/textsplitter"
)

// newTextSplitter 根据配置创建 LangChainGo 文本切块器。
func newTextSplitter(cfg Config) (textsplitter.TextSplitter, error) {
	switch cfg.ChunkStrategy {
	case ChunkRecursive:
		// 递归字符切块：通用默认，分隔符优先级 \n\n > \n > 空格
		return textsplitter.NewRecursiveCharacter(
			textsplitter.WithChunkSize(cfg.ChunkSize),
			textsplitter.WithChunkOverlap(cfg.ChunkOverlap),
			textsplitter.WithSeparators([]string{"\n\n", "\n", " ", ""}),
		), nil

	case ChunkToken:
		// Token 切块：与 GPT 系列 tokenizer 对齐，ChunkSize/Overlap 单位为 token 数
		return textsplitter.NewTokenSplitter(
			textsplitter.WithChunkSize(cfg.ChunkSize),
			textsplitter.WithChunkOverlap(cfg.ChunkOverlap),
			// 可选：textsplitter.WithModelName("gpt-4o") 切换编码表
		), nil

	case ChunkMarkdown:
		// Markdown 切块：先按标题分段，超长段落再用 SecondSplitter 二次切分
		return textsplitter.NewMarkdownTextSplitter(
			textsplitter.WithChunkSize(cfg.ChunkSize),
			textsplitter.WithChunkOverlap(cfg.ChunkOverlap),
			// 可选：保留标题层级到每个 chunk，提升召回相关性
			textsplitter.WithHeadingHierarchy(true),
			// 可选：textsplitter.WithCodeBlocks(true) 保留代码块
			// 可选：textsplitter.WithJoinTableRows(true) 表格按行合并而非逐行切分
		), nil

	default:
		return nil, fmt.Errorf("unknown chunk strategy: %q", cfg.ChunkStrategy)
	}
}
