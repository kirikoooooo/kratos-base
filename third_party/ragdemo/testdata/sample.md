# RAG Demo 示例文档

## 什么是 RAG

RAG（Retrieval-Augmented Generation）在生成回答前先检索相关知识片段，再将其作为上下文交给大模型，从而降低幻觉并支持私有知识库。

## 典型流程

1. **文档加载**：读取 TXT、Markdown 等源文件
2. **切块（Chunk）**：将长文档切为适合 embedding 的小段
3. **向量化（Embed）**：用 embedding 模型转为向量
4. **入库**：写入向量数据库
5. **召回（Retrieval）**：对用户问题做相似度检索
6. **生成**：将召回片段 + 问题送入 LLM

## 切块策略

- **recursive**：按字符递归分隔，适合通用文本
- **token**：按 token 数切分，与 OpenAI 模型对齐
- **markdown**：按标题层级切分，适合技术文档

## 召回方式

- **Top-K 相似度**：返回余弦相似度最高的 K 条
- **阈值过滤**：丢弃相似度低于阈值的片段

## LangChainGo

本 demo 基于 [langchaingo](https://github.com/tmc/langchaingo) 实现，与 Python LangChain 概念一致，便于在 Go 服务中复用。
