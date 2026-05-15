package actor

import "sync/atomic"

type hotfixInterface interface {
	// MsgHotfix 用于对Actor的消息处理进行热修复
	MsgHotfix(a Actor, msg *Message) bool // true拦截消息,false不拦截消息
}

type defaultHotfix struct{}

func (d defaultHotfix) MsgHotfix(a Actor, msg *Message) bool {
	return false // 默认不拦截消息
}

type hotfixValue struct {
	hotfixInterface
}

var atomicHotfix atomic.Pointer[hotfixValue]

func init() {
	atomicHotfix.Store(&hotfixValue{
		hotfixInterface: defaultHotfix{},
	})
}
