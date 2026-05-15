package actor

import (
	"runtime/debug"
	"strconv"
)

type PID interface {
	String() string
	Id() uint64
	Equal(pid PID) bool
}

type defaultPID struct {
	pid string
	id  uint64
}

func NewPID(id uint64, name string) PID {
	return &defaultPID{
		pid: name + ":" + strconv.Itoa(int(id)),
		id:  id,
	}
}

func (p *defaultPID) Id() uint64 {
	return p.id
}

func (p *defaultPID) String() string {
	return p.pid
}

func (p *defaultPID) Equal(pid PID) bool {
	if p == nil || pid == nil {
		return p == pid
	}
	return p.String() == pid.String()
}

type Actor interface {
	PID() PID

	// Process 消息处理
	Process(msg *Message)

	OnStop()
}

func actorLoop(a Actor, mb MailBox, closeCh chan struct{}) {
	defer a.OnStop()
	for {
		select {
		case message := <-mb.OutCh():
			exit := dispatchMessage(a, message, len(mb.OutCh()))
			if exit { // direct exit not process left message
				return
			}
		case <-closeCh:
			// process left message, can't send to other actor
			for len(mb.OutCh()) > 0 {
				m := <-mb.OutCh()
				dispatchMessage(a, m, len(mb.OutCh()))
			}
			logger.Info("actor %v exit", a.PID())
			return
		}
	}
}

func dispatchMessage(a Actor, m interface{}, buff int) (exit bool) {
	defer func() {
		if err := recover(); err != nil {
			logger.Fatal("actor %v panic: %v\nstack: %v", a.PID(), err, string(debug.Stack()))
		}
	}()
	switch t := m.(type) {
	case *Message: // process message
		if t.Id < SysId_End { // 系统消息
			pass, exit := processSysMessage(a, t, buff)
			if exit {
				return true
			}
			if !pass {
				return false
			}
		}

		hook := atomicHotfix.Load()
		if hook.MsgHotfix(a, t) { // 拦截消息 判断是否做热修复
			return false
		}
		a.Process(t)

	case func(pid PID): // process cb
		t(a.PID())
	}
	return false
}

// pass ： continue process, m pass to actor; exit：  exit actor process loop
func processSysMessage(a Actor, m *Message, buff int) (pass bool, exit bool) {
	if m.Id == SysId_Stop {
		return false, true
	}

	if m.Id == SysId_Metric {
		m.Response(RespMessage{
			Data: buff,
		})
		return false, false
	}

	if m.Id == SysId_Suspend {
		gs := m.Data.(*GroupSync)
		gs.Done()
		gs.Pause()
		return false, false
	}

	return true, false
}
