package actor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
)

/*
慎用
  问题：
    1. 同步所有相关actor执行
    2. 只能保存处理导出的数据（使用json的序列化）
    3. 容易死锁，只能用于不环形处理的地方
*/

type TxnStatus int

const minTxnTimeout = time.Millisecond

type Transaction struct {
	local sync.Map
	sync.Mutex
	cond   *sync.Cond
	isDone bool
	timer  *time.Timer
}

func NewTransaction(timeOut time.Duration) *Transaction {
	t := &Transaction{
		Mutex:  sync.Mutex{},
		cond:   nil,
		isDone: false,
	}
	if timeOut < minTxnTimeout {
		timeOut = minTxnTimeout
	}

	t.cond = sync.NewCond(t)
	t.timer = time.AfterFunc(timeOut, func() {
		t.Abort()
		fmt.Println("txn time out")
	})

	return t
}

func (t *Transaction) SavePoint(p interface{}) {
	d, err := json.Marshal(p)
	if err != nil {
		logger.Fatal("json.Marshal failed p=%v err=%s", p, err.Error())
		return
	}

	t.Lock()
	if t.isDone {
		t.Unlock()
		return
	}
	t.Unlock()
	t.local.Store(p, d)
}

func (t *Transaction) Commit() {
	t.Lock()
	defer t.Unlock()
	if t.isDone {
		return
	}
	t.isDone = true
	t.timer.Stop()
	t.cond.Broadcast()
}

func (t *Transaction) Abort() {
	t.Lock()
	defer t.Unlock()
	if t.isDone {
		return
	}
	t.rollback()
	t.isDone = true
	t.timer.Stop()
	t.cond.Broadcast()

}

func (t *Transaction) WaitCommit() {
	t.Lock()
	for !t.isDone {
		t.cond.Wait()
	}
	t.Unlock()
}

func (t *Transaction) rollback() {
	t.local.Range(func(p, v any) bool {
		d, _ := v.([]byte)
		rt := reflect.ValueOf(p).Type()
		rv := reflect.New(rt.Elem()).Interface()

		if err := json.Unmarshal(d, rv); err != nil {
			logger.Fatal("unmarshal type %T failed: %v, data %s", v, err.Error(), string(d))
			return true
		}
		pv := reflect.ValueOf(p)
		pv.Elem().Set(reflect.ValueOf(rv).Elem())
		return true
	})
}
