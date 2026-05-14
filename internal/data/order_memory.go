package data

import (
	"context"
	"fmt"

	"kratos-demo/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type memoryOrderRepo struct {
	log *log.Helper
}

func NewMemoryOrderRepo(logger log.Logger) biz.OrderRepo {
	return &memoryOrderRepo{
		log: log.NewHelper(logger),
	}
}

func (r *memoryOrderRepo) Save(_ context.Context, order *biz.Order) error {
	order.ID = fmt.Sprintf("memory-%s-%d", order.Item, order.Quantity)
	r.log.Infof("save order to in-memory store: id=%s", order.ID)
	return nil
}
