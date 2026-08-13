package service

import (
	"context"

	memoryv1 "kratos-demo/api/memory/v1"
	"kratos-demo/app/memory/internal/biz"
)

type MemoryService struct {
	memoryv1.UnimplementedMemoryServiceServer
	reader biz.PromptContextReader
}

func NewMemoryService(reader biz.PromptContextReader) *MemoryService {
	return &MemoryService{reader: reader}
}

func (s *MemoryService) GetPromptContext(ctx context.Context, request *memoryv1.GetPromptContextRequest) (*memoryv1.GetPromptContextReply, error) {
	return &memoryv1.GetPromptContextReply{Content: s.reader.PromptContext(ctx, request.GetUserId(), request.GetSessionId())}, nil
}
