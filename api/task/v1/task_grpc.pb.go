// gRPC bindings for api/task/v1/task.proto.
//
// This file follows protoc-gen-go-grpc output so services can adopt the
// contract before the local generator is installed. Regenerate it with
// `make proto` once protoc-gen-go-grpc is available.

package v1

import (
	context "context"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

const (
	InternalTaskService_CreateTask_FullMethodName = "/api.task.v1.InternalTaskService/CreateTask"
	InternalTaskService_GetTask_FullMethodName    = "/api.task.v1.InternalTaskService/GetTask"
)

// InternalTaskServiceClient is the service-to-service task client.
type InternalTaskServiceClient interface {
	CreateTask(ctx context.Context, in *CreateTaskRequest, opts ...grpc.CallOption) (*TaskReply, error)
	GetTask(ctx context.Context, in *GetTaskRequest, opts ...grpc.CallOption) (*TaskReply, error)
}

type internalTaskServiceClient struct{ cc grpc.ClientConnInterface }

func NewInternalTaskServiceClient(cc grpc.ClientConnInterface) InternalTaskServiceClient {
	return &internalTaskServiceClient{cc: cc}
}

func (c *internalTaskServiceClient) CreateTask(ctx context.Context, in *CreateTaskRequest, opts ...grpc.CallOption) (*TaskReply, error) {
	out := new(TaskReply)
	err := c.cc.Invoke(ctx, InternalTaskService_CreateTask_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *internalTaskServiceClient) GetTask(ctx context.Context, in *GetTaskRequest, opts ...grpc.CallOption) (*TaskReply, error) {
	out := new(TaskReply)
	err := c.cc.Invoke(ctx, InternalTaskService_GetTask_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// InternalTaskServiceServer is the server-side task contract.
type InternalTaskServiceServer interface {
	CreateTask(context.Context, *CreateTaskRequest) (*TaskReply, error)
	GetTask(context.Context, *GetTaskRequest) (*TaskReply, error)
	mustEmbedUnimplementedInternalTaskServiceServer()
}

// UnimplementedInternalTaskServiceServer provides forward-compatible defaults.
type UnimplementedInternalTaskServiceServer struct{}

func (UnimplementedInternalTaskServiceServer) CreateTask(context.Context, *CreateTaskRequest) (*TaskReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateTask not implemented")
}

func (UnimplementedInternalTaskServiceServer) GetTask(context.Context, *GetTaskRequest) (*TaskReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTask not implemented")
}

func (UnimplementedInternalTaskServiceServer) mustEmbedUnimplementedInternalTaskServiceServer() {}

func RegisterInternalTaskServiceServer(s grpc.ServiceRegistrar, srv InternalTaskServiceServer) {
	s.RegisterService(&InternalTaskService_ServiceDesc, srv)
}

func _InternalTaskService_CreateTask_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateTaskRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InternalTaskServiceServer).CreateTask(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: InternalTaskService_CreateTask_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InternalTaskServiceServer).CreateTask(ctx, req.(*CreateTaskRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _InternalTaskService_GetTask_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTaskRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InternalTaskServiceServer).GetTask(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: InternalTaskService_GetTask_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InternalTaskServiceServer).GetTask(ctx, req.(*GetTaskRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// InternalTaskService_ServiceDesc is the grpc.ServiceDesc for InternalTaskService.
var InternalTaskService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "api.task.v1.InternalTaskService",
	HandlerType: (*InternalTaskServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "CreateTask", Handler: _InternalTaskService_CreateTask_Handler},
		{MethodName: "GetTask", Handler: _InternalTaskService_GetTask_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "api/task/v1/task.proto",
}
