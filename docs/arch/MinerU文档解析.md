# MinerU 文档解析

项目通过 MinerU v4 异步 API 解析工作区中的 PDF：工具先请求预签名上传 URL，上传文件，创建解析任务，轮询完成状态，下载结果 ZIP，并将 Markdown 与图片资源保存到 `.myagent/mineru/`。密钥只从 Git 忽略的项目根 `.env` 读取。

## 配置

在 `.env` 填写：

```bash
MINERU_API_TOKEN=your-token
# 可选：自托管或区域 API 地址，默认 https://mineru.net
MINERU_BASE_URL=https://mineru.net
```

可选运行参数：`MINERU_TIMEOUT_SECONDS`（默认 60）、`MINERU_POLL_INTERVAL_SECONDS`（默认 5）、`MINERU_MAX_WAIT_SECONDS`（默认 900）、`MINERU_OUTPUT_DIR`（默认 `.myagent/mineru`）。

启动 Agent 前加载变量：

```bash
set -a
source .env
set +a
make cli
```

随后可让 Agent 调用 `mineru_parse_document`，例如：

```json
{"path":"Sublinear.pdf","language":"en"}
```

输入路径必须相对工作区，且文件扩展名必须为 `.pdf`。输出会返回 MinerU task ID、结果目录和提取 Markdown 的路径。

## 测试

本地 mock 测试不需要密钥：

```bash
GOCACHE=/private/tmp/go-build-cache go test ./internal/data/tool -run 'TestMinerU(ParseDocumentUploadsPollsAndExtractsMarkdown|RejectsArchivePathTraversal)$'
```

在已配置真实密钥后，使用项目资源 `Sublinear.pdf` 运行真实集成测试：

```bash
set -a
source .env
set +a
KRATOS_RUN_MINERU_INTEGRATION=1 GOCACHE=/private/tmp/go-build-cache \
  go test ./internal/data/tool -run '^TestMinerUSublinearPDFIntegration$' -v
```

真实集成测试会调用 MinerU API，可能产生用量。
