package data

import (
	"context"
	"os"
	"strings"

	"kratos-demo/app/memory/internal/biz"
	"kratos-demo/internal/conf"
	rootmemory "kratos-demo/internal/data/memory"

	"github.com/go-kratos/kratos/v2/log"
)

type promptContextReader struct{ memory rootmemory.AgentMemory }

func NewPromptContextReader() (biz.PromptContextReader, func(), error) {
	dir := strings.TrimSpace(os.Getenv("MEMORY_DIR"))
	userID := strings.TrimSpace(os.Getenv("MEMORY_USER_ID"))
	store, err := rootmemory.NewAgentMemoryStore(&conf.Data{AgentMemory: &conf.Data_AgentMemory{Dir: dir, UserId: userID}}, log.NewStdLogger(os.Stdout))
	if err != nil {
		return nil, nil, err
	}
	memory := rootmemory.NewAgentMemoryUsecase(store, rootmemory.NewAgentMemoryConfig(&conf.Data{AgentMemory: &conf.Data_AgentMemory{Dir: dir, UserId: userID}}))
	return &promptContextReader{memory: memory}, func() {}, nil
}

func (r *promptContextReader) PromptContext(ctx context.Context, _ string, sessionID string) string {
	if r == nil || r.memory == nil {
		return ""
	}
	return r.memory.RenderPromptContext(ctx, sessionID)
}
