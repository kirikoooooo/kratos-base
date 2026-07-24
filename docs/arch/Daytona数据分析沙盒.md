# Daytona 数据分析沙盒

## 目标

`daytona_data_analysis` 仅用于数据分析、统计计算和生成 PNG 图表。生成的 Python 代码不会在 Agent 宿主机执行，而是在 Daytona 临时 Sandbox 中运行；图表下载到 `.myagent/artifacts/<task-id>/`。

默认关闭，未启用时工具不会暴露给模型。

## 配置

```yaml
runtime:
  daytona:
    enabled: false
    api_key_env: DAYTONA_API_KEY
    api_url: https://app.daytona.io/api
    target: us              # 可选
    snapshot: daytona-small # 应提供 Python、pandas、matplotlib
    auto_delete_minutes: 0
```

密钥只通过环境变量提供：

```bash
export DAYTONA_API_KEY='...'
```

启用后还必须在 `configs/casbin/policy.csv` 为分析角色添加：

```csv
p, analyst, tool.daytona_data_analysis, *, *
g, local-dev, analyst
```

## 执行链路

```text
数据分析请求
  -> daytona_data_analysis
  -> Casbin 检查 tool.daytona_data_analysis
  -> Daytona Go SDK 创建 NetworkBlockAll 临时 Sandbox
  -> 上传指定工作区数据到 /tmp/input
  -> CodeRun 执行 Python
  -> 下载 /tmp/*.png 到 .myagent/artifacts/<task-id>
  -> 删除 Sandbox，写入 Trace
```

工具参数必须包含 `code` 和 `/tmp/*.png` 的 `output_path`；可选 `files` 是上传到沙盒的工作区相对文件。Sandbox 总会在调用结束后删除，`auto_delete_minutes` 作为远端兜底。

## 依赖

使用 Daytona 官方 Go SDK：`github.com/daytona/clients/sdk-go/pkg/daytona`。当前官方 SDK 要求 Go 1.25.4；本项目构建环境也需至少使用该版本。
