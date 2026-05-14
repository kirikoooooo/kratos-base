package service

import (
	"context"

	v1 "kratos-demo/api/helloworld/v1"
	"kratos-demo/internal/biz"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewGreeterService)

type GreeterService struct {
	v1.UnimplementedGreeterServer
	uc *biz.GreeterUsecase
}

func NewGreeterService(uc *biz.GreeterUsecase) *GreeterService {
	return &GreeterService{uc: uc}
}

func (s *GreeterService) SayHello(_ context.Context, req *v1.HelloRequest) (*v1.HelloReply, error) {
	name := req.GetName()
	if name == "" {
		name = "world"
	}
	return &v1.HelloReply{Message: s.uc.SayHello(name)}, nil
}
