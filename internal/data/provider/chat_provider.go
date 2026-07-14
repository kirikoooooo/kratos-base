package provider

import (
	bizchat "kratos-demo/internal/biz/chat"
	"kratos-demo/internal/conf"
	errconst "kratos-demo/internal/consts/error"
	datachat "kratos-demo/internal/data/agent_runtime/chat"

	"github.com/go-kratos/kratos/v2/log"
)

// NewChatClientProvider 创建一个 ChatClientProvider 实例。
// 默认使用 LangChain 作为后端，后续可通过配置切换为 OpenAI 原生 SDK。
func NewChatClientProvider(aiConf *conf.AI, logger *log.Helper) (bizchat.ChatClientProvider, error) {
	return newLangChainChatProvider(aiConf, logger)
}

// newLangChainChatProvider 基于 langchaingo LLMProvider 的实现。
func newLangChainChatProvider(aiConf *conf.AI, logger *log.Helper) (bizchat.ChatClientProvider, error) {
	pvd, err := NewProvider(aiConf, logger)
	if err != nil {
		return nil, err
	}
	return &langChainChatProvider{pvd: pvd}, nil
}

type langChainChatProvider struct {
	pvd LLMProvider
}

func (p *langChainChatProvider) CreateChatClient(functionCalling bool) (bizchat.ChatClient, error) {
	if p.pvd == nil {
		return nil, errconst.ErrChatClientNotAvailable
	}
	var opts []ModelOption
	if functionCalling {
		opts = append(opts, WithFunctionCalling())
	}
	model, err := p.pvd.CreateModel(opts...)
	if err != nil {
		return nil, err
	}
	return datachat.NewLangChainClient(model), nil
}
