package biz

import "fmt"

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewGreeterUsecase,
	NewOrderUsecase,
	NewPricingUsecase,
)

type GreeterRepo interface {
	DefaultGreeting() string
}

type GreeterUsecase struct {
	repo GreeterRepo
	log  *log.Helper
}

func NewGreeterUsecase(repo GreeterRepo, logger log.Logger) *GreeterUsecase {
	return &GreeterUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

func (uc *GreeterUsecase) SayHello(name string) string {
	uc.log.Infof("say hello to %s", name)
	return fmt.Sprintf("%s, %s", uc.repo.DefaultGreeting(), name)
}
