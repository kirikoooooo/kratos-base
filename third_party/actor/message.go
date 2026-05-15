package actor

// actor system's msg id 0-100
const (
	SysId_Stop         = 1
	SysId_Metric       = 2
	SysId_NewActor     = 3
	SysId_DestroyActor = 4
	SysId_Suspend      = 5 // 挂起Actor

	SysId_End = 100
)

type Message struct {
	Id     uint32
	Uid    uint64
	Txn    *Transaction
	from   PID
	to     PID
	respCh chan RespMessage
	cb     func(message RespMessage)
	Data   interface{}
	SaveDB bool //针对离线玩家，导致玩家数据写入的，需要存储
}

func (m *Message) From() PID {
	return m.from
}

func (m *Message) To() PID {
	return m.to
}

type RespMessage struct {
	Err  error
	Data interface{}
}

func (m *Message) Response(resp RespMessage) {
	if m.respCh != nil {
		m.respCh <- resp
		return
	}
	if m.cb == nil {
		return
	}

	f := func(pid PID) {
		if pid.Id() != m.from.Id() {
			logger.Error("async cb should be exec in send goroutine %v,now in %v", m.from.Id(), pid.Id())
		}
		m.cb(resp)
	}
	t, ok := root.get(m.from)
	if !ok {
		logger.Error("response actor %v nil, msg id %v", m.from.String(), m.Id)
		return
	}
	t.response(f)
}
