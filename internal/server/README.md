# Server 说明

## 当前项目结构

- `http.go`：创建并配置 Kratos HTTP Server
- `grpc.go`：创建并配置 Kratos gRPC Server
- `server.go`：共享的应用装配示例

在这个 demo 里，HTTP 和 gRPC 都在 `internal/server` 下创建，然后传给：

```go
kratos.New(
	kratos.Server(gs, hs),
)
```

这表示 `gs` 和 `hs` 都是由 `kratos.App` 管理生命周期的 Kratos transport server。

## Kratos 默认支持哪些 Server

根据官方 transport 文档，Kratos 默认内建的服务端传输层主要是：

- HTTP
- gRPC

在当前项目中分别对应：

- `github.com/go-kratos/kratos/v2/transport/http`
- `github.com/go-kratos/kratos/v2/transport/grpc`

## 如果你要新增一个 Server

核心要求是：你的 server 需要满足 Kratos 的 transport server 接口。

```go
type Server interface {
	Start(context.Context) error
	Stop(context.Context) error
}
```

如果你的自定义 server 还希望像内建 HTTP/gRPC 一样参与 transport 上下文、元信息透传等能力，就还需要遵循 Kratos transport 的相关抽象，例如 `Transporter`。

## 新增 Server 的常见方式

### 1. 包装一个已有的服务实现

这是最常见的做法。

例如：

- 包装原生 `net/http` server
- 包装 websocket server
- 包装 MQTT server
- 包装自定义 TCP server

你保留底层实现，只把生命周期管理适配到 Kratos。

### 2. 自己实现一个新的 transport 包

如果你希望行为更接近 Kratos 内建的 HTTP/gRPC，可以单独实现一个 transport 包，并让它：

- 实现 `transport.Server`
- 按需暴露 transport 元信息
- 按需对齐 Kratos 的中间件与上下文约定

## 最小自定义 Server 示例

```go
package server

import (
	"context"
	"net"

	"github.com/go-kratos/kratos/v2/log"
)

type TCPServer struct {
	addr string
	lis  net.Listener
	log  *log.Helper
}

func NewTCPServer(logger log.Logger) *TCPServer {
	return &TCPServer{
		addr: ":9100",
		log:  log.NewHelper(logger),
	}
}

func (s *TCPServer) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.lis = lis
	s.log.Infof("tcp server listening on %s", s.addr)

	go func() {
		<-ctx.Done()
		_ = s.Stop(context.Background())
	}()

	return nil
}

func (s *TCPServer) Stop(context.Context) error {
	if s.lis != nil {
		return s.lis.Close()
	}
	return nil
}
```

然后把它注册进应用：

```go
app := kratos.New(
	kratos.Name("kratos-demo"),
	kratos.Server(gs, hs, tcpSrv),
)
```

## 代码应该放在哪

在这个项目里，建议这样放：

- `internal/server/http.go`：内建 HTTP server
- `internal/server/grpc.go`：内建 gRPC server
- `internal/server/tcp.go`：自定义 TCP server
- `internal/server/ws.go`：自定义 WebSocket server

如果自定义协议越来越复杂，也可以单独拆成自己的包，再在 `internal/server` 里只保留装配代码。

## 如何切换 Server

常见有两种方式：

### 1. 运行时组合

保留多个 server，在 `kratos.Server(...)` 里决定注册哪些。

```go
kratos.Server(gs, hs)
kratos.Server(gs, hs, tcpSrv)
kratos.Server(hs)
```

### 2. 通过 Wire 装配

把新的构造函数加入 provider graph，然后让 `newApp(...)` 接收它，并传给 `kratos.Server(...)`。

当前 demo 里的 `gs`、`hs` 就是这样接进去的。

## 实际分层建议

- 协议适配放在 `internal/server`
- 业务规则放在 `internal/biz`
- 数据访问和外部依赖实现放在 `internal/data`

所以如果你新增一个 server，这个文件里通常只负责：

- 监听
- 解析协议请求
- 调用 service 或 usecase
- 编码响应

不要在这里塞重业务逻辑。

## 参考链接

- Kratos transport 总览：https://go-kratos.dev/docs/component/transport/overview/
- Kratos HTTP transport：https://go-kratos.dev/docs/component/transport/http/
- Kratos gRPC transport：https://go-kratos.dev/docs/component/transport/grpc/
- Kratos `transport.Server` 包文档：https://pkg.go.dev/github.com/go-kratos/kratos/v2/transport
