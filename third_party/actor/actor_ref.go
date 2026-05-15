package actor

import (
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var blockChPool = sync.Pool{New: func() interface{} {
	return make(chan RespMessage, 1)
}}

var timerPool = sync.Pool{New: func() interface{} {
	return time.NewTimer(time.Second * 3)
}}

var (
	ErrTimeOut         = errors.New("time out")
	ErrMailOverflow    = errors.New("mail over flow")
	ErrSyncRequestSelf = errors.New("sync request self")
)

// actorRef 暴露访问actor的接口
type actorRef interface {
	Id() PID
	Send(message *Message) error                              // 异步发送无返回
	AsyncRequest(message *Message, cb func(resp RespMessage)) // 异步发送消息
	Request(message *Message) RespMessage                     // 同步发送消息
	Stop()

	response(cb func(pid PID))
}

type defaultActorRef struct {
	mb MailBox
	id PID
}

func NewDefaultActorRef(m MailBox, id PID) *defaultActorRef {
	ref := &defaultActorRef{
		mb: m,
		id: id,
	}

	return ref
}

func (p *defaultActorRef) Id() PID {
	return p.id
}

func (p *defaultActorRef) Send(msg *Message) error {
	if err := p.mb.Enqueue(msg); err != nil {
		return errors.Wrap(err, fmt.Sprintf("id %v from %v to %v", msg.Id, msg.from, msg.to))
	}
	return nil
}

func (p *defaultActorRef) AsyncRequest(message *Message, cb func(resp RespMessage)) {
	message.cb = cb
	if err := p.mb.Enqueue(message); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("async req from %v to %v message %T", message.from, message.to,
			message.Data))
		if cb != nil {
			cb(RespMessage{
				Err: err,
			})
		}
	}
}

func (p *defaultActorRef) Request(message *Message) RespMessage {
	if p.Id().Equal(message.from) { // sync request to self
		return RespMessage{
			Err: errors.Wrap(ErrSyncRequestSelf, fmt.Sprintf("pid %v msg id %d will block", p.Id(), message.Id)),
		}
	}

	v := blockChPool.Get()
	ch := v.(chan RespMessage)
	message.respCh = ch

	if err := p.mb.Enqueue(message); err != nil {
		blockChPool.Put(v)
		err = errors.Wrap(err, fmt.Sprintf("pid %v sync request to %v message %T", p.Id(), message.to, message.Data))
		return RespMessage{
			Err: err,
		}
	}

	t := timerPool.Get()
	timer := t.(*time.Timer)
	timer.Reset(time.Second * 3)
	defer func() {
		timer.Stop()
		timerPool.Put(timer)
	}()

	select {
	case f := <-message.respCh:
		blockChPool.Put(v)
		return f
	case <-timer.C:
		return RespMessage{
			Err: errors.Wrap(ErrTimeOut, fmt.Sprintf("pid %v sync request from %v msg id %d", message.from, message.to,
				message.Id)),
		}
	}
}

func (p *defaultActorRef) response(cb func(pid PID)) {
	if err := p.mb.Enqueue(cb); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("pid %v callback mailbox overflow", p.Id()))
		logger.Error(err.Error())
	}
}

func (p *defaultActorRef) Stop() {
	if err := p.mb.Enqueue(&Message{Id: SysId_Stop}); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("pid %v stop", p.Id()))
		logger.Error(err.Error())
	}
}
