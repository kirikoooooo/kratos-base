package actor

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRegisterActor(t *testing.T) {
	root = newRootSystem()
	f := &fakeActor{id: 1, name: "fake"}
	f.pid = NewPID(f.id, f.name)

	if err := RegisterActor(f, 10); err != nil {
		t.Fatalf("first RegisterActor() error = %v", err)
	}
	if err := RegisterActor(f, 100); err == nil {
		t.Fatal("expected duplicate RegisterActor() to fail")
	}

	if got, ok := root.get(f.PID()); got == nil || !ok {
		t.Fatal("expected actor to be available by registered pid")
	}
	if got, ok := root.get(NewPID(f.id, f.name)); got == nil || !ok {
		t.Fatal("expected actor to be available by equivalent pid value")
	}
	if got, ok := root.get(NewPID(2, "2")); got != nil || ok {
		t.Fatal("expected unknown pid lookup to miss")
	}
}

func TestSyncRequest(t *testing.T) {
	root = newRootSystem()
	f1 := &fakeActor{id: 1, name: "name"}
	f1.pid = NewPID(f1.id, f1.name)
	_ = RegisterActor(f1, 10)
	_ = RegisterActor(&fakeResponse{}, 10)
	_ = RegisterActor(&fakeTimeout{}, 10)
	_ = RegisterActor(&fakeResponseErr{}, 10)

	if resp := SyncRequest(f1.PID(), f1.PID(), &Message{}); !errors.Is(resp.Err, ErrSyncRequestSelf) {
		t.Fatalf("expected ErrSyncRequestSelf, got %v", resp.Err)
	}

	if resp := SyncRequest(f1.PID(), NewPID(100000, "fake response"), &Message{}); resp.Err != nil {
		t.Fatalf("expected normal sync request success, got %v", resp.Err)
	}

	if resp := SyncRequest(f1.PID(), NewPID(2000000, "time out"), &Message{}); !errors.Is(resp.Err, ErrTimeOut) {
		t.Fatalf("expected ErrTimeOut, got %v", resp.Err)
	}

	if resp := SyncRequest(f1.PID(), NewPID(30000, "response error"), &Message{}); resp.Err == nil {
		t.Fatal("expected response error from fakeResponseErr")
	}

	if resp := SyncRequest(f1.PID(), NewPID(3, "non register"), &Message{}); resp.Err == nil {
		t.Fatal("expected error for non-registered pid")
	}
}

func TestAsyncRequest(t *testing.T) {
	root = newRootSystem()
	f1 := &fakeActor{id: 1, name: "name"}
	f1.pid = NewPID(f1.id, f1.name)
	_ = RegisterActor(f1, 10)
	_ = RegisterActor(&fakeResponse{}, 10)
	_ = RegisterActor(&fakeTimeout{}, 10)
	_ = RegisterActor(&fakeResponseErr{}, 10)

	tests := []struct {
		name    string
		target  PID
		wantErr bool
	}{
		{name: "self", target: f1.PID(), wantErr: false},
		{name: "normal", target: NewPID(100000, "fake response"), wantErr: false},
		{name: "timeout actor still replies", target: NewPID(2000000, "time out"), wantErr: false},
		{name: "response error", target: NewPID(30000, "response error"), wantErr: true},
		{name: "missing actor", target: NewPID(3, "non register"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan RespMessage, 1)
			AsyncRequest(f1.PID(), tt.target, &Message{}, func(msg RespMessage) {
				done <- msg
			})
			select {
			case msg := <-done:
				if tt.wantErr && msg.Err == nil {
					t.Fatal("expected async request error")
				}
				if !tt.wantErr && msg.Err != nil {
					t.Fatalf("unexpected async request error: %v", msg.Err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting async request callback")
			}
		})
	}
}

func TestStop(t *testing.T) {
	root = newRootSystem()
	f1 := &fakeActor{id: 1, name: "name"}
	f1.pid = NewPID(f1.id, f1.name)
	_ = RegisterActor(f1, 10)

	StopActor(f1.PID())
	if got, ok := root.get(f1.PID()); got != nil || ok {
		t.Fatal("expected stopped actor to be removed")
	}

	if err := RegisterActor(f1, 10); err != nil {
		t.Fatalf("expected stopped actor to be re-registerable: %v", err)
	}
	if got, ok := root.get(f1.PID()); got == nil || !ok {
		t.Fatal("expected actor to be present after re-register")
	}

	missing := NewPID(4, "not register")
	StopActor(missing)
	if got, ok := root.get(f1.PID()); got == nil || !ok {
		t.Fatal("expected existing actor to remain after stopping missing pid")
	}
	if got, ok := root.get(missing); got != nil || ok {
		t.Fatal("expected missing pid to remain absent")
	}
}

func BenchmarkAsyncRequest(b *testing.B) {
	root = newRootSystem()
	req := newFakeActor(200000)
	worker := newFakeActor(200001)

	_ = RegisterActor(req, 10000000)
	_ = RegisterActor(worker, 10000000)
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

	_ = RegisterActor(req, 100000)
	_ = RegisterActor(worker, 100000)
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
	_ = RegisterActor(req1, 10)
	resp1 := NewFakeTxnActorResponse(6000001)
	_ = RegisterActor(resp1, 10)

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
	_ = RegisterActor(req1, 10)
	resp1 := NewFakeTxnActorResponse(6000001)
	_ = RegisterActor(resp1, 10)

	defPid := NewPID(1, "default")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SyncRequest(defPid, req1.pid, &Message{Data: resp1.pid, Id: failTxnMsgId})
	}
}

func TestTransaction(t *testing.T) {
	root = newRootSystem()
	req1 := NewFakeTxnActorReq(6000000)
	_ = RegisterActor(req1, 10)
	resp1 := NewFakeTxnActorResponse(6000001)
	_ = RegisterActor(resp1, 10)

	initMap := map[string]int{
		"init1": 1,
		"init2": 2,
	}
	defPid := NewPID(1, "default")

	t.Run("timeout", func(t *testing.T) {
		mustDeepCopy(&req1.m, initMap)
		SyncRequest(defPid, req1.pid, &Message{Id: timeoutTxnMsgId, Data: resp1.PID()})
		if !mapsEqual(req1.m, initMap) {
			t.Fatalf("map after timeout = %#v, want %#v", req1.m, initMap)
		}
	})

	t.Run("txn failed", func(t *testing.T) {
		mustDeepCopy(&req1.m, initMap)
		SyncRequest(defPid, req1.pid, &Message{Id: failTxnMsgId, Data: resp1.PID()})
		if !mapsEqual(req1.m, initMap) {
			t.Fatalf("map after failure = %#v, want %#v", req1.m, initMap)
		}
	})

	t.Run("txn success", func(t *testing.T) {
		mustDeepCopy(&req1.m, initMap)
		SyncRequest(defPid, req1.pid, &Message{Id: successTxnMsgId, Data: resp1.PID()})
		if mapsEqual(req1.m, initMap) {
			t.Fatalf("map after success = %#v, expected mutation", req1.m)
		}
	})
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
