package actor

import (
	"fmt"
	"runtime/debug"
	"sync"
)

const arrLen = 1000

// rootSystem 所有actor 都注册在这里 提供actor间交互的接口
type rootSystem struct {
	actorLock sync.RWMutex
	actors    map[uint64]actorRef // pid >= 1000 TODO change to bucket map
	actorArr  [arrLen]actorRef    // pid < 1000
	closeCh   chan struct{}
	wt        sync.WaitGroup
}

var root = newRootSystem()

func newRootSystem() *rootSystem {
	return &rootSystem{
		actors:  make(map[uint64]actorRef),
		closeCh: make(chan struct{}),
	}
}

func (s *rootSystem) get(pid PID) (actorRef, bool) {
	s.actorLock.RLock()
	defer s.actorLock.RUnlock()

	if pid.Id() < arrLen {
		a := s.actorArr[pid.Id()]
		return a, a != nil
	}

	v, ok := s.actors[pid.Id()]
	return v, ok
}

func (s *rootSystem) newMailBox(pid PID, cap int) MailBox {
	mb := newDefaultMailBox(cap)
	//s.mbLock.Lock()
	//s.mailBoxes[pid] = mb
	//s.mbLock.Unlock()
	return mb
}

func (s *rootSystem) spawn(a Actor, cap int) error {
	if a.PID() == nil {
		return fmt.Errorf("actor not impl pid: stack %s", string(debug.Stack()))
	}
	mb := s.newMailBox(a.PID(), cap)
	ref := NewDefaultActorRef(mb, a.PID())

	id := a.PID().Id()
	s.actorLock.Lock()
	if id < arrLen {
		if s.actorArr[id] != nil {
			s.actorLock.Unlock()
			return fmt.Errorf("actor %s already register", a.PID())
		}
		s.actorArr[id] = ref
	} else {
		if _, ok := s.actors[id]; ok {
			s.actorLock.Unlock()
			return fmt.Errorf("actor %s already register", a.PID())
		}
		s.actors[id] = ref
	}
	s.actorLock.Unlock()

	s.wt.Add(1)
	go func() {
		defer s.wt.Done()
		actorLoop(a, mb, s.closeCh)
		//s.deleteActor(a.PID())
	}()

	return nil
}

func (s *rootSystem) deleteActor(pid PID) {
	s.actorLock.Lock()
	defer s.actorLock.Unlock()
	if pid.Id() < arrLen {
		s.actorArr[pid.Id()] = nil
		return
	}

	delete(s.actors, pid.Id())
}

func (s *rootSystem) stop() {
	// clear actor ref, stop rcv message
	s.actorLock.Lock()
	s.actorArr = [arrLen]actorRef{}
	s.actors = map[uint64]actorRef{}
	s.actorLock.Unlock()

	close(s.closeCh)

	s.wt.Wait()
}
