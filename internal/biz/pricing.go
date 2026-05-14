package biz

import (
	"github.com/go-kratos/kratos/v2/log"
)

type PricingUsecase struct {
	log *log.Helper
}

func NewPricingUsecase(logger log.Logger) *PricingUsecase {
	return &PricingUsecase{log: log.NewHelper(logger)}
}

func (uc *PricingUsecase) Quote(amount int64, vip bool) int64 {
	if amount < 0 {
		return 0
	}
	if vip {
		uc.log.Infof("quote vip amount=%d", amount)
		return amount * 90 / 100
	}
	uc.log.Infof("quote regular amount=%d", amount)
	return amount
}
