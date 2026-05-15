package actor

import (
	"sync"
	"testing"
	"time"

	"liteframe/pkg/util"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegisterActor(t *testing.T) {
	root = newRootSystem()
	Convey("Given RegisterActor", t, func() {
		f := &fakeActor{
			id:   1,
			name: "fake",
		}
		f.pid = NewPID(f.id, f.name)

		Convey("When duplicate register", func() {
			err := RegisterActor(f, 10)
			So(err, ShouldBeNil)

			err = RegisterActor(f, 100)
			So(err, ShouldNotBeNil)
		})

		Convey("When get by pid", func() {
			Convey("When by register pid", func() {
				gf, ok := root.get(f.PID())
				So(gf, ShouldNotBeNil)
				So(ok, ShouldBeTrue)
			})

			Convey("When by pid value", func() {
				gf, ok := root.get(NewPID(f.id, f.name))
				So(gf, ShouldNotBeNil)
				So(ok, ShouldBeTrue)
			})
			Convey("When get non register pid", func() {
				gf, ok := root.get(NewPID(2, "2"))
				So(gf, ShouldBeNil)
				So(ok, ShouldBeFalse)
			})
		})

	})
}

func TestSyncRequest(t *testing.T) {
	root = newRootSystem()
	f1 := &fakeActor{id: 1, name: "name"}
	f1.pid = NewPID(f1.id, f1.name)
	RegisterActor(f1, 10)

	f2 := &fakeResponse{}
	RegisterActor(f2, 10)

	fTimeout := &fakeTimeout{}
	RegisterActor(fTimeout, 10)

	fErr := &fakeResponseErr{}
	RegisterActor(fErr, 10)

	Convey("Given SyncRequest", t, func() {

		Convey("When sync request self", func() {

			resp := SyncRequest(f1.PID(), f1.PID(), &Message{})
			So(errors.Is(resp.Err, ErrSyncRequestSelf), ShouldBeTrue)
		})

		Convey("When sync request normal response", func() {
			resp := SyncRequest(f1.PID(), f2.PID(), &Message{})
			So(resp.Err, ShouldBeNil)
		})

		Convey("When sync request time out", func() {
			resp := SyncRequest(f1.PID(), fTimeout.PID(), &Message{})
			So(errors.Is(resp.Err, ErrTimeOut), ShouldBeTrue)
		})

		Convey("When sync response error", func() {
			resp := SyncRequest(f1.PID(), fErr.PID(), &Message{})
			So(resp.Err, ShouldNotBeNil)
		})
		Convey("When sync request non register", func() {
			pid := NewPID(3, "non register")
			resp := SyncRequest(f1.PID(), pid, &Message{})
			So(resp.Err, ShouldNotBeNil)
		})
	})
}

func TestAsyncRequest(t *testing.T) {
	root = newRootSystem()
	f1 := &fakeActor{id: 1, name: "name"}
	f1.pid = NewPID(f1.id, f1.name)
	RegisterActor(f1, 10)

	f2 := &fakeResponse{}
	RegisterActor(f2, 10)

	fTimeout := &fakeTimeout{}
	RegisterActor(fTimeout, 10)

	fErr := &fakeResponseErr{}
	RegisterActor(fErr, 10)

	Convey("Given AsyncRequest", t, func() {

		Convey("When async request self", func(c C) {

			AsyncRequest(f1.PID(), f1.PID(), &Message{}, func(msg RespMessage) {
				c.So(msg.Err, ShouldBeNil)
			})

		})

		Convey("When async request normal response", func(c C) {
			AsyncRequest(f1.PID(), f2.PID(), &Message{}, func(msg RespMessage) {
				c.So(msg.Err, ShouldBeNil)
			})

		})

		Convey("When async request time out", func(c C) {
			AsyncRequest(f1.PID(), fTimeout.PID(), &Message{}, func(msg RespMessage) {
				c.So(msg.Err, ShouldNotBeNil)
			})
		})

		Convey("When async response error", func(c C) {
			AsyncRequest(f1.PID(), fErr.PID(), &Message{}, func(msg RespMessage) {
				c.So(msg.Err, ShouldNotBeNil)
			})
		})
		Convey("When async request non register", func(c C) {
			pid := NewPID(3, "non register")
			AsyncRequest(f1.PID(), pid, &Message{}, func(msg RespMessage) {
				c.So(msg.Err, ShouldNotBeNil)
			})
		})
	})

}

func TestStop(t *testing.T) {
	root = newRootSystem()
	f1 := &fakeActor{id: 1, name: "name"}
	f1.pid = NewPID(f1.id, f1.name)
	RegisterActor(f1, 10)

	f2 := &fakeResponse{}
	RegisterActor(f2, 10)

	fTimeout := &fakeTimeout{}
	RegisterActor(fTimeout, 10)

	fErr := &fakeResponseErr{}
	RegisterActor(fErr, 10)

	Convey("Given StopActor", t, func() {

		Convey("When Stop exist", func() {
			StopActor(f1.PID())
			fg, ok := root.get(f1.PID())
			So(fg, ShouldBeNil)
			So(ok, ShouldBeFalse)

			Convey("When register stoped", func() {
				RegisterActor(f1, 10)
				fg, ok := root.get(f1.PID())
				So(fg, ShouldNotBeNil)
				So(ok, ShouldBeTrue)
			})

		})

		Convey("When stop not exist", func() {
			pid := NewPID(4, "not register")
			StopActor(pid)
			fg, ok := root.get(f1.PID())
			So(fg, ShouldNotBeNil)
			So(ok, ShouldBeTrue)

			fg, ok = root.get(pid)
			So(fg, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkAsyncRequest(b *testing.B) {
	root = newRootSystem()
	req := newFakeActor(200000)
	worker := newFakeActor(200001)

	RegisterActor(req, 10000000)
	RegisterActor(worker, 10000000)
	wt := sync.WaitGroup{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wt.Add(1)
		AsyncRequest(req.PID(), worker.PID(), &Message{}, func(msg RespMessage) {
			wt.Done()
		})
	}

	wt.Wait()
}

func BenchmarkChannel(b *testing.B) {
	n := 10000000
	ch := make(chan *Message, n)
	rcv := make(chan *Message, n)
	wt := sync.WaitGroup{}
	go func() {
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					close(rcv)
					return
				}
				rcv <- v
			}
		}
	}()

	go func() {
		for {
			select {
			case _, ok := <-rcv:
				if !ok {
					return
				}
				wt.Done()
			}
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wt.Add(1)
		ch <- &Message{}
	}

	wt.Wait()
}

func TestAsyncRequestB(t *testing.T) {
	root = newRootSystem()
	req := newFakeActor(200000)
	worker := newFakeActor(200001)

	RegisterActor(req, 100000)
	RegisterActor(worker, 100000)
	wt := sync.WaitGroup{}
	wt.Add(1000000)

	beg := time.Now()
	for i := 0; i < 1000000; i++ {
		AsyncRequest(req.PID(), worker.PID(), &Message{}, func(msg RespMessage) {
			wt.Done()
		})
	}

	wt.Wait()

	t.Log(time.Since(beg))
}

func TestTransactionCommit(t *testing.T) {
	root = newRootSystem()
	req1 := NewFakeTxnActorReq(6000000)
	RegisterActor(req1, 10)
	resp1 := NewFakeTxnActorResponse(6000001)
	RegisterActor(resp1, 10)

	defPid := NewPID(1, "default")
	beg := time.Now()
	for i := 0; i < 20000; i++ {
		SyncRequest(defPid, req1.pid, &Message{Data: resp1.pid, Id: failTxnMsgId})
	}

	t.Log(time.Since(beg))
}

func BenchmarkTransaction(b *testing.B) {
	root = newRootSystem()
	req1 := NewFakeTxnActorReq(6000000)
	RegisterActor(req1, 10)
	resp1 := NewFakeTxnActorResponse(6000001)
	RegisterActor(resp1, 10)

	defPid := NewPID(1, "default")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SyncRequest(defPid, req1.pid, &Message{Data: resp1.pid, Id: failTxnMsgId})
	}

}

func TestTransaction(t *testing.T) {
	root = newRootSystem()
	req1 := NewFakeTxnActorReq(6000000)
	RegisterActor(req1, 10)
	resp1 := NewFakeTxnActorResponse(6000001)
	RegisterActor(resp1, 10)

	initMap := map[string]int{
		"init1": 1,
		"init2": 2,
	}
	defPid := NewPID(1, "default")

	Convey("Given Transaction", t, func() {
		Convey("When timeout", func() {

			util.DeepCopy(&req1.m, initMap)
			SyncRequest(defPid, req1.pid, &Message{Id: timeoutTxnMsgId, Data: resp1.PID()})
			So(req1.m, ShouldResemble, initMap)

		})
		Convey("When txn failed", func() {

			util.DeepCopy(&req1.m, initMap)
			SyncRequest(defPid, req1.pid, &Message{Id: failTxnMsgId, Data: resp1.PID()})
			So(req1.m, ShouldResemble, initMap)

		})
		Convey("When txn success", func() {

			util.DeepCopy(&req1.m, initMap)
			SyncRequest(defPid, req1.pid, &Message{Id: successTxnMsgId, Data: resp1.PID()})
			So(req1.m, ShouldNotResemble, initMap)

		})
	})

}
