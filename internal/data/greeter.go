package data

import "kratos-demo/internal/biz"

import "github.com/go-kratos/kratos/v2/log"

type greeterRepo struct {
	data *Data
	log  *log.Helper
}

func NewGreeterRepo(data *Data, logger log.Logger) biz.GreeterRepo {
	return &greeterRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *greeterRepo) DefaultGreeting() string {
	r.log.Infof("loading greeting from %s datasource", r.data.db.GetDriver())
	return "hello"
}
