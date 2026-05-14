package biz

import (
	"context"
	"errors"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
)

type Order struct {
	ID       string
	Item     string
	Quantity int32
}

type OrderRepo interface {
	Save(context.Context, *Order) error
}

type OrderUsecase struct {
	repo OrderRepo
	log  *log.Helper
}

func NewOrderUsecase(repo OrderRepo, logger log.Logger) *OrderUsecase {
	return &OrderUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

func (uc *OrderUsecase) Create(ctx context.Context, item string, quantity int32) (*Order, error) {
	item = strings.TrimSpace(item)
	if item == "" {
		return nil, errors.New("item is required")
	}
	if quantity <= 0 {
		return nil, errors.New("quantity must be positive")
	}
	order := &Order{Item: item, Quantity: quantity}
	uc.log.Infof("create order item=%s quantity=%d", item, quantity)
	if uc.repo != nil {
		if err := uc.repo.Save(ctx, order); err != nil {
			return nil, err
		}
	}
	return order, nil
}
