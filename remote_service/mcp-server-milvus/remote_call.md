# Milvus MCP 接口文档

MCP server 启动后会注册以下 14 个工具，外部可通过 MCP 协议的 `tools/call` 调用。

---

## 集合管理

### 1. milvus_list_collections

列出当前数据库中的所有集合。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| — | — | — | 无参数 |

返回：集合名称列表。

---

### 2. milvus_get_collection_info

查看指定集合的详细结构信息（字段、索引等）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| `collection_name` | `string` | ✓ | 集合名称 |

返回：JSON 格式的集合描述。

---

### 3. milvus_create_collection

创建新集合，支持快速创建和自定义 schema 两种模式。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 新集合名称 |
| `auto_id` | `bool` | | `True` | 是否自动生成主键 |
| `dimension` | `int` | | `768` | 向量维度；有 `field_schema` 时忽略 |
| `primary_field_name` | `string` | | `"id"` | 主键字段名；有 `field_schema` 时忽略 |
| `vector_field_name` | `string` | | `"vector"` | 向量字段名；有 `field_schema` 时忽略 |
| `metric_type` | `string` | | `"COSINE"` | 距离度量（COSINE/L2/IP）；有 `field_schema` 时忽略 |
| `field_schema` | `list[dict]` | | `None` | 自定义字段 schema（name/type/dimension 等） |
| `index_params` | `list[dict]` | | `None` | 索引参数（field_name/index_type/其他） |
| `other_kwargs` | `dict` | | `None` | 其他创建参数（enable_dynamic_field 等） |

> 自定义 schema 创建后需手动建索引并 `milvus_load_collection`。

---

### 4. milvus_load_collection

将集合加载到内存中，使其可被搜索。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 集合名称 |
| `replica_number` | `int` | | `1` | 副本数 |

---

### 5. milvus_release_collection

将集合从内存中释放（停止可被搜索）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| `collection_name` | `string` | ✓ | 集合名称 |

---

## 搜索

### 6. milvus_text_search

全文搜索（BM25），通过稀疏向量做关键词匹配。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 集合名称 |
| `query_text` | `string` | ✓ | — | 搜索文本 |
| `limit` | `int` | | `5` | 最大返回条数 |
| `output_fields` | `list[str]` | | `None` | 要返回的字段 |
| `drop_ratio` | `float` | | `0.2` | 低频词忽略比例（0.0–1.0） |

---

### 7. milvus_vector_search

向量相似度搜索（稠密向量）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 集合名称 |
| `vector` | `list[float]` | ✓ | — | 查询向量 |
| `vector_field` | `string` | | `"vector"` | 向量字段名 |
| `limit` | `int` | | `5` | 最大返回条数 |
| `output_fields` | `list[str]` | | `None` | 要返回的字段 |
| `metric_type` | `string` | | `"COSINE"` | 距离度量（COSINE/L2/IP） |
| `filter_expr` | `string` | | `None` | 过滤表达式 |
| `radius` | `float` | | `None` | 范围搜索下界 |
| `range_filter` | `float` | | `None` | 范围搜索上界 |

---

### 8. milvus_text_similarity_search

文本相似度搜索（对文本字段进行语义相似度搜索）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 集合名称 |
| `query_text` | `string` | ✓ | — | 查询文本 |
| `anns_field` | `string` | ✓ | — | 文本搜索字段名 |
| `limit` | `int` | | `5` | 最大返回条数 |
| `output_fields` | `list[str]` | | `None` | 要返回的字段 |
| `metric_type` | `string` | | `"COSINE"` | 距离度量（COSINE/L2/IP） |
| `filter_expr` | `string` | | `None` | 过滤表达式 |
| `radius` | `float` | | `None` | 范围搜索下界 |
| `range_filter` | `float` | | `None` | 范围搜索上界 |

---

### 9. milvus_hybrid_search

混合搜索：BM25 文本 + 稠密向量，RRF 融合排序。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 集合名称 |
| `query_text` | `string` | ✓ | — | BM25 文本查询 |
| `text_field` | `string` | ✓ | — | 文本字段名 |
| `vector` | `list[float]` | ✓ | — | 稠密向量 |
| `vector_field` | `string` | ✓ | — | 向量字段名 |
| `limit` | `int` | | `5` | 最大返回条数 |
| `output_fields` | `list[str]` | | `None` | 要返回的字段 |
| `filter_expr` | `string` | | `None` | 过滤表达式 |
| `sparse_radius` | `float` | | `None` | 稀疏搜索下界 |
| `sparse_range_filter` | `float` | | `None` | 稀疏搜索上界 |
| `dense_radius` | `float` | | `None` | 稠密搜索下界 |
| `dense_range_filter` | `float` | | `None` | 稠密搜索上界 |

---

### 10. milvus_query

按过滤表达式查询集合（标量过滤，不涉及向量）。

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|:--:|--------|------|
| `collection_name` | `string` | ✓ | — | 集合名称 |
| `filter_expr` | `string` | ✓ | — | 过滤表达式（如 `age > 20`） |
| `output_fields` | `list[str]` | | `None` | 要返回的字段 |
| `limit` | `int` | | `10` | 最大返回条数 |

---

## 数据操作

### 11. milvus_insert_data

插入数据。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| `collection_name` | `string` | ✓ | 集合名称 |
| `data` | `list[dict]` | ✓ | 数据列表，每个 dict 是一条记录 |

---

### 12. milvus_delete_entities

按过滤条件删除实体。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| `collection_name` | `string` | ✓ | 集合名称 |
| `filter_expr` | `string` | ✓ | 过滤表达式（匹配到的全部删除） |

---

## 数据库管理

### 13. milvus_list_databases

列出 Milvus 实例中所有数据库。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| — | — | — | 无参数 |

---

### 14. milvus_use_database

切换到指定数据库（后续操作在该库下进行）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|:--:|------|
| `db_name` | `string` | ✓ | 目标数据库名 |

---

## MCP 调用示例

假设 MCP server 已注册，调用方式如下（具体通过 `tools/call`）：

```json
{
  "method": "tools/call",
  "params": {
    "name": "milvus_text_search",
    "arguments": {
      "collection_name": "my_docs",
      "query_text": "kratos microservice",
      "limit": 3
    }
  }
}
```

```json
{
  "method": "tools/call",
  "params": {
    "name": "milvus_create_collection",
    "arguments": {
      "collection_name": "my_collection",
      "dimension": 768,
      "metric_type": "COSINE"
    }
  }
}
```

---

## 启动方式

```bash
cd remote_service/mcp-server-milvus
python -m mcp_server_milvus.server \
  --milvus-uri http://localhost:19530 \
  --milvus-token <optional>
```

或通过环境变量：

```bash
export MILVUS_URI="http://localhost:19530"
export MILVUS_TOKEN="<token>"
export MILVUS_DB="default"
python -m mcp_server_milvus.server
```

传输模式：
- **stdio**（默认）：标准输入/输出
- **sse**：`--sse --port 8000`
- **streamable-http**：`--streamable-http --port 8000`
