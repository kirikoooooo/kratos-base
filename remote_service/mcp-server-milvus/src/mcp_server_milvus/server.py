import argparse
import os
from contextlib import asynccontextmanager
from typing import Any, AsyncIterator, Optional, List
from dotenv import load_dotenv
import json
from mcp.server.fastmcp.server import FastMCP, Context
from pymilvus import (
    MilvusClient,
    DataType,
    AnnSearchRequest,
    RRFRanker,
)


class MilvusConnector:
    def __init__(
        self, uri: str, token: Optional[str] = None, db_name: Optional[str] = "default"
    ):
        self.uri = uri
        self.token = token
        self.client = MilvusClient(uri=uri, token=token, db_name=db_name)

    async def list_collections(self) -> list[str]:
        """列出当前数据库中的所有 collection。"""
        try:
            return self.client.list_collections()
        except Exception as e:
            raise ValueError(f"Failed to list collections: {str(e)}")

    async def get_collection_info(self, collection_name: str) -> dict:
        """获取 collection 的详细信息。"""
        try:
            return self.client.describe_collection(collection_name)
        except Exception as e:
            raise ValueError(f"Failed to get collection info: {str(e)}")

    async def search_collection(
        self,
        collection_name: str,
        query_text: str,
        limit: int = 5,
        output_fields: Optional[list[str]] = None,
        drop_ratio: float = 0.2,
    ) -> list[dict]:
        """
        在 collection 中执行全文检索。

        Args:
            collection_name: 要检索的 collection 名称
            query_text: 要检索的文本
            limit: 最大返回结果数
            output_fields: 结果中要返回的字段
            drop_ratio: 忽略低频词的比例（0.0-1.0）
        """
        try:
            search_params = {"params": {"drop_ratio_search": drop_ratio}}

            results = self.client.search(
                collection_name=collection_name,
                data=[query_text],
                anns_field="sparse",
                limit=limit,
                output_fields=output_fields,
                search_params=search_params,
            )
            return results
        except Exception as e:
            raise ValueError(f"Search failed: {str(e)}")

    async def query_collection(
        self,
        collection_name: str,
        filter_expr: str,
        output_fields: Optional[list[str]] = None,
        limit: int = 10,
    ) -> list[dict]:
        """使用过滤表达式查询 collection。"""
        try:
            return self.client.query(
                collection_name=collection_name,
                filter=filter_expr,
                output_fields=output_fields,
                limit=limit,
            )
        except Exception as e:
            raise ValueError(f"Query failed: {str(e)}")

    async def vector_search(
        self,
        collection_name: str,
        vector: list[float],
        vector_field: str,
        limit: int = 5,
        output_fields: Optional[list[str]] = None,
        metric_type: str = "COSINE",
        filter_expr: Optional[str] = None,
        radius: Optional[float] = None,
        range_filter: Optional[float] = None,
    ) -> list[dict]:
        """
        在 collection 中执行向量相似度检索。

        Args:
            collection_name: 要检索的 collection 名称
            vector: 查询向量
            vector_field: 待检索的向量字段
            limit: 最大返回结果数
            output_fields: 结果中要返回的字段
            metric_type: 距离度量（COSINE、L2、IP）
            filter_expr: 可选的过滤表达式
            radius: 范围检索的可选下界
            range_filter: 范围检索的可选上界
        """
        try:
            search_params = {"metric_type": metric_type, "params": {"nprobe": 10}}
            if radius is not None:
                search_params["params"]["radius"] = radius
            if range_filter is not None:
                search_params["params"]["range_filter"] = range_filter

            results = self.client.search(
                collection_name=collection_name,
                data=[vector],
                anns_field=vector_field,
                search_params=search_params,
                limit=limit,
                output_fields=output_fields,
                filter=filter_expr,
            )
            return results
        except Exception as e:
            raise ValueError(f"Vector search failed: {str(e)}")

    async def text_similarity_search(
        self,
        collection_name: str,
        query_text: str,
        anns_field: str,
        limit: int = 5,
        output_fields: Optional[list[str]] = None,
        metric_type: str = "COSINE",
        filter_expr: Optional[str] = None,
        radius: Optional[float] = None,
        range_filter: Optional[float] = None,
    ) -> list[dict]:
        """
        在 collection 中执行文本相似度检索。

        Args:
            collection_name: 要检索的 collection 名称
            query_text: 用于相似度检索的查询文本
            anns_field: 文本检索字段名
            limit: 最大返回结果数
            output_fields: 结果中要返回的字段
            metric_type: 距离度量（COSINE、L2、IP）
            filter_expr: 可选的过滤表达式
            radius: 范围检索的可选下界
            range_filter: 范围检索的可选上界
        """
        try:
            search_params = {"metric_type": metric_type, "params": {"nprobe": 10}}
            if radius is not None:
                search_params["params"]["radius"] = radius
            if range_filter is not None:
                search_params["params"]["range_filter"] = range_filter

            results = self.client.search(
                collection_name=collection_name,
                data=[query_text],
                anns_field=anns_field,
                search_params=search_params,
                limit=limit,
                output_fields=output_fields,
                filter=filter_expr,
            )
            return results
        except Exception as e:
            raise ValueError(f"Text similarity search failed: {str(e)}")

    async def hybrid_search(
        self,
        collection_name: str,
        query_text: str,
        text_field: str,
        vector: list[float],
        vector_field: str,
        limit: int,
        output_fields: Optional[list[str]] = None,
        filter_expr: Optional[str] = None,
        sparse_radius: Optional[float] = None,
        sparse_range_filter: Optional[float] = None,
        dense_radius: Optional[float] = None,
        dense_range_filter: Optional[float] = None,
    ) -> list[dict]:
        """
        执行混合检索：结合 BM25 文本检索、向量检索和 RRF 排序。

        Args:
            collection_name: 要检索的 collection 名称
            query_text: BM25 检索的查询文本
            text_field: 文本检索字段名
            vector: 稠密向量检索的查询向量
            vector_field: 向量检索字段名
            limit: 最大返回结果数
            output_fields: 结果中要返回的字段
            filter_expr: 可选的过滤表达式
            sparse_radius: 稀疏向量范围检索的可选下界
            sparse_range_filter: 稀疏向量范围检索的可选上界
            dense_radius: 稠密向量范围检索的可选下界
            dense_range_filter: 稠密向量范围检索的可选上界
        """
        try:
            sparse_params = {"params": {"nprobe": 10}}
            dense_params = {"params": {"drop_ratio_build": 0.2}}
            if sparse_radius is not None:
                sparse_params["params"]["radius"] = sparse_radius
            if sparse_range_filter is not None:
                sparse_params["params"]["range_filter"] = sparse_range_filter
            if dense_radius is not None:
                dense_params["params"]["radius"] = dense_radius
            if dense_range_filter is not None:
                dense_params["params"]["range_filter"] = dense_range_filter
            # BM25 检索请求
            sparse_request = AnnSearchRequest(
                data=[query_text],
                anns_field=text_field,
                param=sparse_params,
                limit=limit,
            )
            # 稠密向量检索请求
            dense_request = AnnSearchRequest(
                data=[vector],
                anns_field=vector_field,
                param=dense_params,
                limit=limit,
            )
            # 混合检索
            results = self.client.hybrid_search(
                collection_name=collection_name,
                reqs=[sparse_request, dense_request],
                ranker=RRFRanker(60),
                limit=limit,
                output_fields=output_fields,
                filter=filter_expr,
            )

            return results

        except Exception as e:
            raise ValueError(f"Hybrid search failed: {str(e)}")

    async def create_collection(
        self,
        collection_name: str,
        auto_id: bool = True,
        dimension: int = 768,
        primary_field_name: str = "id",
        vector_field_name: str = "vector",
        metric_type: str = "COSINE",
        field_schema: list[dict[str, Any]] = None,
        index_params: list[dict[str, Any]] = None,
        **kwargs: Any
    ) -> bool:
        """
        通过快速配置或自定义 schema 创建 collection。

        Args:
            collection_name: 新 collection 名称
            auto_id: 是否自动生成主键 ID，默认 True
            dimension: 向量维度，默认 768；快速创建时使用，提供 field_schema 时忽略
            primary_field_name: 主键字段名，默认 "id"；提供 field_schema 时忽略
            vector_field_name: 向量字段名，默认 "vector"；提供 field_schema 时忽略
            metric_type: 度量类型，默认 "COSINE"；提供 field_schema 时忽略
            field_schema: 字段 schema 列表；每项包含 name、type、dimension、index_type 等键
            index_params: 索引及参数列表；每项包含 field_name、index_type 和其他索引参数。
                默认 None，即不创建索引。使用 field_schema 时，创建 collection 后需要手动创建索引并加载。
            **kwargs: 创建 collection 的额外参数
        """
        try:
            # 检查 collection 是否已存在
            if collection_name in self.client.list_collections():
                raise ValueError(f"Collection '{collection_name}' already exists")

            schema_kwargs = {
                "auto_id": auto_id,
                "enable_dynamic_field": kwargs.get("enable_dynamic_field", True),
            }
            if "partition_key_isolation" in kwargs:
                schema_kwargs["partition_key_isolation"] = kwargs["partition_key_isolation"]

            if field_schema is not None:
                schema = MilvusClient.create_schema(**schema_kwargs)
                for field_kwargs in field_schema:
                    field_kwargs["datatype"] = getattr(DataType, field_kwargs["datatype"].upper())
                    schema.add_field(**field_kwargs)
            else:
                schema = None

            built_index_params = MilvusClient.prepare_index_params()
            if index_params is not None and len(index_params) > 0:
                for index_kwargs in index_params:
                    built_index_params.add_index(**index_kwargs)

            # 创建 collection
            self.client.create_collection(
                collection_name=collection_name,
                auto_id=auto_id,
                dimension=dimension,
                primary_field_name=primary_field_name,
                vector_field_name=vector_field_name,
                metric_type=metric_type,
                schema=schema,
                index_params=built_index_params if index_params is not None else None,
                **kwargs
            )

            return True
        except Exception as e:
            raise ValueError(f"Failed to create collection: {str(e)}")

    async def insert_data(
        self, collection_name: str, data: list[dict[str, Any]]
    ) -> dict[str, Any]:
        """
        向 collection 写入数据。

        Args:
            collection_name: collection 名称
            data: 记录字典列表
        """
        try:
            result = self.client.insert(collection_name=collection_name, data=data)
            return result
        except Exception as e:
            raise ValueError(f"Insert failed: {str(e)}")

    async def delete_entities(
        self, collection_name: str, filter_expr: str
    ) -> dict[str, Any]:
        """
        根据过滤表达式删除 collection 中的实体。

        Args:
            collection_name: collection 名称
            filter_expr: 用于筛选待删除实体的过滤表达式
        """
        try:
            result = self.client.delete(
                collection_name=collection_name, expr=filter_expr
            )
            return result
        except Exception as e:
            raise ValueError(f"Delete failed: {str(e)}")

    async def get_collection_stats(self, collection_name: str) -> dict[str, Any]:
        """
        获取 collection 的统计信息。

        Args:
            collection_name: collection 名称
        """
        try:
            return self.client.get_collection_stats(collection_name)
        except Exception as e:
            raise ValueError(f"Failed to get collection stats: {str(e)}")

    async def multi_vector_search(
        self,
        collection_name: str,
        vectors: list[list[float]],
        vector_field: str,
        limit: int = 5,
        output_fields: Optional[list[str]] = None,
        metric_type: str = "COSINE",
        filter_expr: Optional[str] = None,
        search_params: Optional[dict[str, Any]] = None,
    ) -> list[list[dict]]:
        """
        使用多个查询向量执行向量相似度检索。

        Args:
            collection_name: 要检索的 collection 名称
            vectors: 查询向量列表
            vector_field: 待检索的向量字段
            limit: 每个查询的最大返回结果数
            output_fields: 结果中要返回的字段
            metric_type: 距离度量（COSINE、L2、IP）
            filter_expr: 可选的过滤表达式
            search_params: 额外的检索参数
        """
        try:
            if search_params is None:
                search_params = {"metric_type": metric_type, "params": {"nprobe": 10}}

            results = self.client.search(
                collection_name=collection_name,
                data=vectors,
                anns_field=vector_field,
                search_params=search_params,
                limit=limit,
                output_fields=output_fields,
                filter=filter_expr,
            )
            return results
        except Exception as e:
            raise ValueError(f"Multi-vector search failed: {str(e)}")

    async def create_index(
        self,
        collection_name: str,
        field_name: str,
        index_type: str = "IVF_FLAT",
        metric_type: str = "COSINE",
        params: Optional[dict[str, Any]] = None,
    ) -> bool:
        """
        为向量字段创建索引。

        Args:
            collection_name: collection 名称
            field_name: 要建立索引的字段
            index_type: 索引类型（IVF_FLAT、HNSW 等）
            metric_type: 距离度量（COSINE、L2、IP）
            params: 额外索引参数
        """
        try:
            if params is None:
                params = {"nlist": 1024}

            index_params = {
                "index_type": index_type,
                "metric_type": metric_type,
                "params": params,
            }

            self.client.create_index(
                collection_name=collection_name,
                field_name=field_name,
                index_params=index_params,
            )
            return True
        except Exception as e:
            raise ValueError(f"Failed to create index: {str(e)}")

    async def bulk_insert(
        self, collection_name: str, data: dict[str, list[Any]], batch_size: int = 1000
    ) -> list[dict[str, Any]]:
        """
        分批写入数据以获得更好的性能。

        Args:
            collection_name: collection 名称
            data: 字段名到值列表的映射
            batch_size: 每批记录数
        """
        try:
            results = []
            field_names = list(data.keys())
            total_records = len(data[field_names[0]])

            for i in range(0, total_records, batch_size):
                batch_data = {
                    field: data[field][i : i + batch_size] for field in field_names
                }

                result = self.client.insert(
                    collection_name=collection_name, data=batch_data
                )
                results.append(result)

            return results
        except Exception as e:
            raise ValueError(f"Bulk insert failed: {str(e)}")

    async def load_collection(
        self, collection_name: str, replica_number: int = 1
    ) -> bool:
        """
        将 collection 加载到内存中，以支持检索和查询。

        Args:
            collection_name: 要加载的 collection 名称
            replica_number: 副本数量
        """
        try:
            self.client.load_collection(
                collection_name=collection_name, replica_number=replica_number
            )
            return True
        except Exception as e:
            raise ValueError(f"Failed to load collection: {str(e)}")

    async def release_collection(self, collection_name: str) -> bool:
        """
        从内存中释放 collection。

        Args:
            collection_name: 要释放的 collection 名称
        """
        try:
            self.client.release_collection(collection_name=collection_name)
            return True
        except Exception as e:
            raise ValueError(f"Failed to release collection: {str(e)}")

    async def get_query_segment_info(self, collection_name: str) -> dict[str, Any]:
        """
        获取查询分片的信息。

        Args:
            collection_name: collection 名称
        """
        try:
            return self.client.get_query_segment_info(collection_name)
        except Exception as e:
            raise ValueError(f"Failed to get query segment info: {str(e)}")

    async def upsert_data(
        self, collection_name: str, data: dict[str, list[Any]]
    ) -> dict[str, Any]:
        """
        向 collection 执行 upsert（插入或更新已存在的数据）。

        Args:
            collection_name: collection 名称
            data: 字段名到值列表的映射
        """
        try:
            result = self.client.upsert(collection_name=collection_name, data=data)
            return result
        except Exception as e:
            raise ValueError(f"Upsert failed: {str(e)}")

    async def get_index_info(
        self, collection_name: str, field_name: Optional[str] = None
    ) -> dict[str, Any]:
        """
        获取 collection 中索引的信息。

        Args:
            collection_name: collection 名称
            field_name: 可选；指定要获取索引信息的字段
        """
        try:
            return self.client.describe_index(
                collection_name=collection_name, index_name=field_name
            )
        except Exception as e:
            raise ValueError(f"Failed to get index info: {str(e)}")

    async def get_collection_loading_progress(
        self, collection_name: str
    ) -> dict[str, Any]:
        """
        获取 collection 的加载进度。

        Args:
            collection_name: collection 名称
        """
        try:
            return self.client.get_load_state(collection_name)
        except Exception as e:
            raise ValueError(f"Failed to get loading progress: {str(e)}")

    async def list_databases(self) -> list[str]:
        """列出 Milvus 实例中的所有数据库。"""
        try:
            return self.client.list_databases()
        except Exception as e:
            raise ValueError(f"Failed to list databases: {str(e)}")

    async def use_database(self, db_name: str) -> bool:
        """切换到其他数据库。

        Args:
            db_name: 要使用的数据库名称
        """
        try:
            # 为指定数据库创建新的客户端
            self.client = MilvusClient(uri=self.uri, token=self.token, db_name=db_name)
            return True
        except Exception as e:
            raise ValueError(f"Failed to switch database: {str(e)}")


class MilvusContext:
    def __init__(self, connector: MilvusConnector):
        self.connector = connector


@asynccontextmanager
async def server_lifespan(server: FastMCP) -> AsyncIterator[MilvusContext]:
    """管理 Milvus 连接器的应用生命周期。"""
    config = server.config

    connector = MilvusConnector(
        uri=config.get("milvus_uri", "http://localhost:19530"),
        token=config.get("milvus_token"),
        db_name=config.get("db_name", "default"),
    )

    try:
        yield MilvusContext(connector)
    finally:
        pass


mcp = FastMCP(name="Milvus", lifespan=server_lifespan)


@mcp.tool()
async def milvus_text_search(
    collection_name: str,
    query_text: str,
    limit: int = 5,
    output_fields: Optional[list[str]] = None,
    drop_ratio: float = 0.2,
    ctx: Context = None,
) -> str:
    """
    在 Milvus collection 中通过全文检索查找文档。

    Args:
        collection_name: 要检索的 collection 名称
        query_text: 要检索的文本
        limit: 最大返回结果数
        output_fields: 结果中要包含的字段
        drop_ratio: 忽略低频词的比例（0.0-1.0）
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        results = await connector.search_collection(
            collection_name=collection_name,
            query_text=query_text,
            limit=limit,
            output_fields=output_fields,
            drop_ratio=drop_ratio,
        )

        output = (
            f"Search results for '{query_text}' in collection '{collection_name}':\n\n"
        )
        for result in results:
            output += f"{result}\n\n"

        return output
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_list_collections(ctx: Context) -> str:
    """列出当前数据库中的所有 collection。"""
    try:
        connector = ctx.request_context.lifespan_context.connector
        collections = await connector.list_collections()
        return f"Collections in database:\n{', '.join(collections)}"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_query(
    collection_name: str,
    filter_expr: str,
    output_fields: Optional[list[str]] = None,
    limit: int = 10,
    ctx: Context = None,
) -> str:
    """
    使用过滤表达式查询 collection。

    Args:
        collection_name: 要查询的 collection 名称
        filter_expr: 过滤表达式（例如 `age > 20`）
        output_fields: 结果中要包含的字段
        limit: 最大返回结果数
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        results = await connector.query_collection(
            collection_name=collection_name,
            filter_expr=filter_expr,
            output_fields=output_fields,
            limit=limit,
        )

        output = (
            f"Query results for '{filter_expr}' in collection '{collection_name}':\n\n"
        )
        for result in results:
            output += f"{result}\n\n"

        return output
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_vector_search(
    collection_name: str,
    vector: list[float],
    vector_field: str = "vector",
    limit: int = 5,
    output_fields: Optional[list[str]] = None,
    metric_type: str = "COSINE",
    filter_expr: Optional[str] = None,
    radius: Optional[float] = None,
    range_filter: Optional[float] = None,
    ctx: Context = None,
) -> str:
    """
    在 collection 中执行向量相似度检索。

    Args:
        collection_name: 要检索的 collection 名称
        vector: 查询向量
        vector_field: 待检索的向量字段
        limit: 最大返回结果数
        output_fields: 结果中要包含的字段
        metric_type: 距离度量（COSINE、L2、IP）
        filter_expr: 可选的过滤表达式
        radius: 范围检索的可选下界
        range_filter: 范围检索的可选上界
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        results = await connector.vector_search(
            collection_name=collection_name,
            vector=vector,
            vector_field=vector_field,
            limit=limit,
            output_fields=output_fields,
            metric_type=metric_type,
            filter_expr=filter_expr,
            radius=radius,
            range_filter=range_filter,
        )

        output = f"Vector search results for '{collection_name}':\n\n"
        for result in results:
            output += f"{result}\n\n"

        return output
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_hybrid_search(
    collection_name: str,
    query_text: str,
    text_field: str,
    vector: list[float],
    vector_field: str,
    limit: int = 5,
    output_fields: Optional[list[str]] = None,
    filter_expr: Optional[str] = None,
    sparse_radius: Optional[float] = None,
    sparse_range_filter: Optional[float] = None,
    dense_radius: Optional[float] = None,
    dense_range_filter: Optional[float] = None,
    ctx: Context = None,
) -> str:
    """
    执行结合文本与向量检索的混合检索。

    Args:
        collection_name: 要检索的 collection 名称
        query_text: BM25 检索的查询文本
        text_field: 文本检索字段名
        vector: 稠密向量检索的查询向量
        vector_field: 向量检索字段名
        limit: 最大返回结果数
        output_fields: 结果中要返回的字段
        filter_expr: 可选的过滤表达式
        sparse_radius: 稀疏向量范围检索的可选下界
        sparse_range_filter: 稀疏向量范围检索的可选上界
        dense_radius: 稠密向量范围检索的可选下界
        dense_range_filter: 稠密向量范围检索的可选上界
    """
    try:
        connector = ctx.request_context.lifespan_context.connector

        results = await connector.hybrid_search(
            collection_name=collection_name,
            query_text=query_text,
            text_field=text_field,
            vector=vector,
            vector_field=vector_field,
            limit=limit,
            output_fields=output_fields,
            filter_expr=filter_expr,
            sparse_radius=sparse_radius,
            sparse_range_filter=sparse_range_filter,
            dense_radius=dense_radius,
            dense_range_filter=dense_range_filter,
        )

        output = (
            f"Hybrid search results for text '{query_text}' in '{collection_name}':\n\n"
        )
        for result in results:
            output += f"{result}\n\n"

        return output
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_text_similarity_search(
    collection_name: str,
    query_text: str,
    anns_field: str,
    limit: int = 5,
    output_fields: Optional[list[str]] = None,
    metric_type: str = "COSINE",
    filter_expr: Optional[str] = None,
    radius: Optional[float] = None,
    range_filter: Optional[float] = None,
    ctx: Context = None,
) -> str:
    """
    在 collection 中执行文本相似度检索。

    Args:
        collection_name: 要检索的 collection 名称
        query_text: 用于相似度检索的查询文本
        anns_field: 文本检索字段名
        limit: 最大返回结果数
        output_fields: 结果中要包含的字段
        metric_type: 距离度量（COSINE、L2、IP）
        filter_expr: 可选的过滤表达式
        radius: 范围检索的可选下界
        range_filter: 范围检索的可选上界
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        results = await connector.text_similarity_search(
            collection_name=collection_name,
            query_text=query_text,
            anns_field=anns_field,
            limit=limit,
            output_fields=output_fields,
            metric_type=metric_type,
            filter_expr=filter_expr,
            radius=radius,
            range_filter=range_filter,
        )

        output = f"Text similarity search results for '{query_text}' in '{collection_name}':\n\n"
        for result in results:
            output += f"{result}\n\n"

        return output
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_create_collection(
    collection_name: str,
    auto_id: bool = True,
    dimension: Optional[int] = 768,
    primary_field_name: Optional[str] = "id",
    vector_field_name: Optional[str] = "vector",
    metric_type: Optional[str] = "COSINE",
    field_schema: Optional[list[dict[str, Any]]] = None,
    index_params: Optional[list[dict[str, Any]]] = None,
    other_kwargs: Optional[dict[str, Any]] = None,
    ctx: Context = None,
) -> str:
    """
    使用指定 schema 创建新的 collection。

    Args:
        collection_name: 新 collection 名称
        auto_id: 是否自动生成主键 ID，默认 True
        dimension: 向量维度，默认 768；提供 field_schema 时忽略
        primary_field_name: 主键字段名，默认 "id"；提供 field_schema 时忽略
        vector_field_name: 向量字段名，默认 "vector"；提供 field_schema 时忽略
        metric_type: 度量类型，默认 "COSINE"；提供 field_schema 时忽略
        field_schema: 字段 schema 列表，每项包含 name、type 等键
        index_params: 可选的索引参数列表，每项包含 field_name、index_type 等键
        other_kwargs: 创建 collection 的额外关键字参数
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        success = await connector.create_collection(
            collection_name=collection_name,
            auto_id=auto_id,
            dimension=dimension,
            primary_field_name=primary_field_name,
            vector_field_name=vector_field_name,
            metric_type=metric_type,
            field_schema=field_schema,
            index_params=index_params,
            **(other_kwargs if other_kwargs is not None else {})
        )

        return f"Collection '{collection_name}' created successfully"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_insert_data(
    collection_name: str, data: list[dict[str, Any]], ctx: Context = None
) -> str:
    """
    向 collection 写入数据。

    Args:
        collection_name: collection 名称
        data: 记录字典列表
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        result = await connector.insert_data(collection_name=collection_name, data=data)

        return f"Data inserted into collection '{collection_name}' with result: {str(result)}"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_delete_entities(
    collection_name: str, filter_expr: str, ctx: Context = None
) -> str:
    """
    根据过滤表达式删除 collection 中的实体。

    Args:
        collection_name: collection 名称
        filter_expr: 用于筛选待删除实体的过滤表达式
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        result = await connector.delete_entities(
            collection_name=collection_name, filter_expr=filter_expr
        )

        return f"Entities deleted from collection '{collection_name}' with result: {str(result)}"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_load_collection(
    collection_name: str, replica_number: int = 1, ctx: Context = None
) -> str:
    """
    将 collection 加载到内存中，以支持检索和查询。

    Args:
        collection_name: 要加载的 collection 名称
        replica_number: 副本数量
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        success = await connector.load_collection(
            collection_name=collection_name, replica_number=replica_number
        )

        return f"Collection '{collection_name}' loaded successfully with {replica_number} replica(s)"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_release_collection(collection_name: str, ctx: Context = None) -> str:
    """
    从内存中释放 collection。

    Args:
        collection_name: 要释放的 collection 名称
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        success = await connector.release_collection(collection_name=collection_name)

        return f"Collection '{collection_name}' released successfully"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_list_databases(ctx: Context = None) -> str:
    """列出 Milvus 实例中的所有数据库。"""
    try:
        connector = ctx.request_context.lifespan_context.connector
        databases = await connector.list_databases()
        return f"Databases in Milvus instance:\n{', '.join(databases)}"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_use_database(db_name: str, ctx: Context = None) -> str:
    """
    切换到其他数据库。

    Args:
        db_name: 要使用的数据库名称
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        success = await connector.use_database(db_name)

        return f"Switched to database '{db_name}' successfully"
    except Exception as e:
        return f"Error: {str(e)}"


@mcp.tool()
async def milvus_get_collection_info(collection_name: str, ctx: Context = None) -> str:
    """
    列出指定 collection 的详细信息。

    Args:
        collection_name: 要查看的 collection 名称
    """
    try:
        connector = ctx.request_context.lifespan_context.connector
        collection_info = await connector.get_collection_info(collection_name)
        info_str = json.dumps(collection_info, indent=2, default=list)
        return f"Collection information:\n{info_str}"
    except Exception as e:
        return f"Error: {str(e)}"


def parse_arguments():
    parser = argparse.ArgumentParser(description="Milvus MCP 服务")
    parser.add_argument(
        "--milvus-uri",
        type=str,
        default="http://localhost:19530",
        help="Milvus 服务 URI",
    )
    parser.add_argument(
        "--milvus-token", type=str, default=None, help="Milvus 认证令牌"
    )
    parser.add_argument(
        "--milvus-db", type=str, default="default", help="Milvus 数据库名称"
    )
    parser.add_argument("--sse", action="store_true", help="启用 SSE 模式")
    parser.add_argument(
        "--streamable-http",
        dest="streamable_http",
        action="store_true",
        help="启用 Streamable HTTP 传输（推荐用于生产环境）"
    )
    parser.add_argument(
        "--stateless",
        action="store_true",
        help="以无状态模式运行（不持久化会话，仅适用于 Streamable HTTP）"
    )
    parser.add_argument(
        "--port", type=int, default=8000, help="SSE/Streamable HTTP 服务端口"
    )
    return parser.parse_args()


def main():
    load_dotenv()
    args = parse_arguments()

    mcp.config = {
        "milvus_uri": os.environ.get("MILVUS_URI", args.milvus_uri),
        "milvus_token": os.environ.get("MILVUS_TOKEN", args.milvus_token),
        "db_name": os.environ.get("MILVUS_DB", args.milvus_db),
    }
    if args.sse:
        mcp.settings.port = args.port
        mcp.settings.host = "localhost"
        mcp.run(transport="sse")
    elif args.streamable_http:
        mcp.settings.port = args.port
        mcp.settings.host = "localhost"
        if args.stateless:
            mcp.settings.stateless_http = True
            mcp.settings.json_response = True
        mcp.run(transport="streamable-http")
    else:
        mcp.run()


if __name__ == "__main__":
    main()
