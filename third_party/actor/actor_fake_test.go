package actor

import (
	"fmt"
	"time"

	"liteframe/pkg/util"
)

const (
	beforeTxnMsgId  = 5000000
	failTxnMsgId    = 5000001
	successTxnMsgId = 5000002
	timeoutTxnMsgId = 5000003
)

type fakeActor struct {
	id   uint64
	name string
	pid  PID
}

func newFakeActor(id uint64) *fakeActor {
	f := &fakeActor{
		id:   id,
		name: "fake",
	}
	f.pid = NewPID(f.id, f.name)
	return f
}

func (f *fakeActor) PID() PID {
	return f.pid
}

func (f *fakeActor) Process(msg *Message) {
	msg.Response(RespMessage{})
}

func (f *fakeActor) OnStop() {

}

type fakeResponse struct{}

func (f *fakeResponse) PID() PID {
	return NewPID(100000, "fake response")
}

func (f *fakeResponse) Process(msg *Message) {
	msg.Response(RespMessage{})
}

func (f *fakeResponse) OnStop() {
}

type fakeTimeout struct{}

func (f *fakeTimeout) PID() PID {
	return NewPID(2000000, "time out")
}

func (f *fakeTimeout) Process(msg *Message) {
	time.Sleep(3100 * time.Millisecond)
	msg.Response(RespMessage{})
}

func (f *fakeTimeout) OnStop() {
}

type fakeResponseErr struct{}

func (fakeResponseErr) PID() PID {
	return NewPID(30000, "response error")
}

func (fakeResponseErr) Process(msg *Message) {

	msg.Response(RespMessage{Err: fmt.Errorf("error response")})
}

func (fakeResponseErr) OnStop() {
}

type ObjData struct {
	M        map[string]*ObjData
	Arr      []*ObjData
	Str      string
	FixedArr [5]int
}

type fakeTxnActorReq struct {
	pid PID
	m   map[string]int
}

func NewFakeTxnActorReq(id uint64) *fakeTxnActorReq {
	return &fakeTxnActorReq{
		pid: NewPID(id, "txnReq"),
		m:   make(map[string]int),
	}
}

func (f fakeTxnActorReq) PID() PID {
	return f.pid
}

func (f *fakeTxnActorReq) Process(msg *Message) {
	if msg.Id == beforeTxnMsgId {
		m := make(map[string]int)
		util.DeepCopy(m, f.m)
		msg.Response(RespMessage{
			Err:  nil,
			Data: m,
		})
		return
	}

	defer msg.Response(RespMessage{})

	pid := msg.Data.(PID)
	if msg.Id == failTxnMsgId {
		txn := NewTransaction(time.Second)
		txn.SavePoint(&f.m)
		f.m["modify_req"] = 55555

		SyncRequest(f.pid, pid, &Message{
			Id:  failTxnMsgId,
			Uid: 0,
			Txn: txn,
		})

		txn.Abort()
		return
	}

	if msg.Id == timeoutTxnMsgId {
		txn := NewTransaction(time.Second)
		txn.SavePoint(&f.m)
		f.m["modify_req"] = 55555

		SyncRequest(f.pid, pid, &Message{
			Id:  timeoutTxnMsgId,
			Uid: 0,
			Txn: txn,
		})
		txn.Commit()
		return
	}

	if msg.Id == successTxnMsgId {
		txn := NewTransaction(time.Second)
		txn.SavePoint(&f.m)
		f.m["txn_success"] = 1111

		SyncRequest(f.pid, pid, &Message{
			Id:  msg.Id,
			Uid: 0,
			Txn: txn,
		})
		txn.Commit()
		return
	}

}

func (f *fakeTxnActorReq) OnStop() {
}

type fakeTxnActorResponse struct {
	pid PID
	obj *ObjData
}

func NewFakeTxnActorResponse(id uint64) *fakeTxnActorResponse {
	return &fakeTxnActorResponse{
		pid: NewPID(id, "txnResp"),
		obj: &ObjData{
			M: map[string]*ObjData{
				"test": &ObjData{
					M: map[string]*ObjData{
						"M2": nil,
					},
					Arr: []*ObjData{&ObjData{
						Str:      "deep2",
						FixedArr: [5]int{222, 222, 222, 2222, 222},
					}},
					Str:      "deep1",
					FixedArr: [5]int{111, 111, 111, 111, 111},
				},
			},
			Arr: []*ObjData{
				&ObjData{
					Str:      "arr12",
					FixedArr: [5]int{12, 12, 12, 12, 12},
				}},
			Str:      "arr1",
			FixedArr: [5]int{1, 1, 1, 1, 1},
		},
	}
}

func (f fakeTxnActorResponse) PID() PID {
	return f.pid
}

func (f *fakeTxnActorResponse) Process(msg *Message) {

	if msg.Id == beforeTxnMsgId {
		d := &ObjData{}
		util.DeepCopy(d, f.obj)
		msg.Response(RespMessage{
			Err:  nil,
			Data: d,
		})
		return
	}
	txn := msg.Txn
	if txn == nil {
		msg.Response(RespMessage{})
		return
	}
	defer func() {
		txn.WaitCommit()
	}()
	txn.SavePoint(f.obj)
	defer msg.Response(RespMessage{})

	if msg.Id == failTxnMsgId {
		f.obj.FixedArr[3] = 123545677
		f.obj.Str = "modify_str"
		f.obj.M["modify_map"] = &ObjData{}

		return
	}

	if msg.Id == successTxnMsgId {
		f.obj.FixedArr[3] = 123545677
		f.obj.Str = "modify_str"
		f.obj.M["modify_map"] = &ObjData{}
		return
	}

	if msg.Id == timeoutTxnMsgId {
		f.obj.FixedArr[3] = 123545677
		f.obj.Str = "modify_str"
		f.obj.M["modify_map"] = &ObjData{}

		time.Sleep(time.Millisecond * 1100)
		return
	}

}

func (fakeTxnActorResponse) OnStop() {
}
