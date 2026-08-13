// gRPC bindings for api/memory/v1/memory.proto.

package v1

import (
	context "context"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

const MemoryService_GetPromptContext_FullMethodName = "/api.memory.v1.MemoryService/GetPromptContext"

type MemoryServiceClient interface {
	GetPromptContext(context.Context, *GetPromptContextRequest, ...grpc.CallOption) (*GetPromptContextReply, error)
}

type memoryServiceClient struct{ cc grpc.ClientConnInterface }

func NewMemoryServiceClient(cc grpc.ClientConnInterface) MemoryServiceClient {
	return &memoryServiceClient{cc: cc}
}

func (c *memoryServiceClient) GetPromptContext(ctx context.Context, in *GetPromptContextRequest, opts ...grpc.CallOption) (*GetPromptContextReply, error) {
	out := new(GetPromptContextReply)
	err := c.cc.Invoke(ctx, MemoryService_GetPromptContext_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type MemoryServiceServer interface {
	GetPromptContext(context.Context, *GetPromptContextRequest) (*GetPromptContextReply, error)
	mustEmbedUnimplementedMemoryServiceServer()
}

type UnimplementedMemoryServiceServer struct{}

func (UnimplementedMemoryServiceServer) GetPromptContext(context.Context, *GetPromptContextRequest) (*GetPromptContextReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetPromptContext not implemented")
}

func (UnimplementedMemoryServiceServer) mustEmbedUnimplementedMemoryServiceServer() {}

func RegisterMemoryServiceServer(s grpc.ServiceRegistrar, srv MemoryServiceServer) {
	s.RegisterService(&MemoryService_ServiceDesc, srv)
}

func _MemoryService_GetPromptContext_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPromptContextRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MemoryServiceServer).GetPromptContext(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: MemoryService_GetPromptContext_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MemoryServiceServer).GetPromptContext(ctx, req.(*GetPromptContextRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var MemoryService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "api.memory.v1.MemoryService",
	HandlerType: (*MemoryServiceServer)(nil),
	Methods:     []grpc.MethodDesc{{MethodName: "GetPromptContext", Handler: _MemoryService_GetPromptContext_Handler}},
	Streams:     []grpc.StreamDesc{},
	Metadata:    "api/memory/v1/memory.proto",
}
