package data

import (
	"context"
	"fmt"

	"kratos-demo/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type databaseOrderRepo struct {
	data *Data
	log  *log.Helper
}

func NewDatabaseOrderRepo(data *Data, logger log.Logger) biz.OrderRepo {
	return &databaseOrderRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

// 运行时切换，一种fallback机制
func NewOrderRepo(data *Data, logger log.Logger) biz.OrderRepo {
	switch data.db.GetDriver() {
	case "memory":
		return NewMemoryOrderRepo(logger)
	default:
		return NewDatabaseOrderRepo(data, logger)
	}
}

func (r *databaseOrderRepo) Save(_ context.Context, order *biz.Order) error {
	order.ID = fmt.Sprintf("demo-%s-%d", order.Item, order.Quantity)
	r.log.Infof("save order to %s datasource: id=%s", r.data.db.GetDriver(), order.ID)
	return nil
}
