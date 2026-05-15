package actor

import (
	"sync"
	"time"
)

type GroupSync struct {
	waitGroup sync.WaitGroup
	lock      *sync.Mutex
	timer     *time.Timer
	cond      *sync.Cond
	end       bool
	ch        chan bool
}

func NewGroupSync(timeOut time.Duration) *GroupSync {
	gs := &GroupSync{
		ch:   make(chan bool, 1),
		lock: &sync.Mutex{},
	}
	gs.cond = sync.NewCond(gs.lock)
	if timeOut < minTxnTimeout {
		timeOut = minTxnTimeout
	}
	gs.timer = time.AfterFunc(timeOut, func() {
		gs.exit()
		gs.ch <- false
	})
	return gs
}

func (gs *GroupSync) Pause() {
	gs.lock.Lock()
	for !gs.end {
		gs.cond.Wait()
	}
	gs.lock.Unlock()
}

func (gs *GroupSync) Add(delta int) {
	gs.waitGroup.Add(delta)
}

func (gs *GroupSync) Done() {
	gs.waitGroup.Done()
}

func (gs *GroupSync) Wait(action func(bool)) {
	go func() {
		gs.waitGroup.Wait()
		gs.ch <- true
	}()

	ret := <-gs.ch
	action(ret)
	gs.exit()
}

func (gs *GroupSync) exit() {
	gs.lock.Lock()
	defer gs.lock.Unlock()
	if gs.end {
		return
	}

	gs.end = true
	gs.cond.Broadcast()
	gs.timer.Stop()
}
