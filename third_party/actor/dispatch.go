package actor

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	criticalTIme = time.Millisecond * 1000 // TODO 临时设置为1秒 开发完成后需要调整
)

type Handler func(ctx context.Context, data *Message) error

// StatisticFunc 统计函数
type StatisticFunc func(uid uint64, id uint32, ms int64)

type blackRPCInfo struct {
	RPC    uint16
	UidMap map[uint64]struct{}
}

type blackListData struct {
	Data []*blackRPCInfo
}

type Dispatcher struct {
	handlers       map[uint32]Handler
	maxExecuteTime time.Duration
	st             StatisticFunc
	sync.RWMutex
	idBlackMap map[uint16]*blackRPCInfo
}

func NewDispatcher(maxMillSec int, st StatisticFunc) *Dispatcher {
	return &Dispatcher{
		handlers:       make(map[uint32]Handler),
		st:             st,
		maxExecuteTime: time.Duration(maxMillSec) * time.Millisecond,
		idBlackMap:     make(map[uint16]*blackRPCInfo),
	}
}

func (d *Dispatcher) Register(id uint32, cmd Handler) {
	if _, ok := d.handlers[id]; ok {
		panic(fmt.Sprintf("register id %d duplicated!", id))
	}
	d.handlers[id] = cmd
}

// AddBlackId 目前未被调用, 直接采用UpdateBlackRpc来更新黑名单
// uidList为空表示全局
func (d *Dispatcher) AddBlackId(rpcId uint16, uidList []uint64) {
	d.Lock()
	defer d.Unlock()

	d.idBlackMap[rpcId] = &blackRPCInfo{
		RPC:    rpcId,
		UidMap: make(map[uint64]struct{}, len(uidList)),
	}
	for _, uid := range uidList {
		d.idBlackMap[rpcId].UidMap[uid] = struct{}{}
	}
	logger.Info("add rpc id %v uidList %v to black list", rpcId, uidList)

}

func (d *Dispatcher) IsBlackId(id uint16, uid uint64) bool {
	d.RLock()
	defer d.RUnlock()

	if uid == 0 { // only block uid map
		return false
	}
	info, ok := d.idBlackMap[id]
	if !ok {
		return false
	}

	if len(info.UidMap) == 0 {
		return true
	}
	_, ok = info.UidMap[uid]
	return ok
}

func (d *Dispatcher) UpdateBlackRpc(data *blackListData) {
	d.Lock()
	defer d.Unlock()
	d.idBlackMap = make(map[uint16]*blackRPCInfo)

	if data == nil {
		logger.Info("clear all black rpc")
		return
	}

	for _, info := range data.Data {
		d.idBlackMap[info.RPC] = info
	}

	for _, info := range data.Data {
		logger.Info("update black data %+v", info)
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, data *Message) error {
	id := data.Id
	uid := data.Uid
	// in black list
	//if d.IsBlackId(id, uid) {
	//	err := fmt.Errorf("from %v to %v uid: %d id: %d in black list", data.from, data.to, uid, id)
	//	log.Error(err.Error())
	//	return err
	//}

	// unregister id
	h, ok := d.handlers[id]
	if !ok {
		err := fmt.Errorf("from %v to %v uid: %d rcv not registered id: %d", data.from, data.to, uid, id)
		logger.Error(err.Error())
		return err
	}

	// handle
	bt := time.Now()
	err := h(ctx, data)
	et := time.Since(bt)
	if err != nil {
		logger.Error("dispatch failed: from %v to %v uid %v id %v, err: %v", data.from, data.to, uid, id, err)
	}

	if et > d.maxExecuteTime {
		logger.Error("handle slow, from %v to %v uid %v id %v cost %v ms", data.from, data.to, uid, id,
			et.Milliseconds())
	}

	if d.st != nil {
		d.st(uid, id, et.Milliseconds())
	}

	return err
}
