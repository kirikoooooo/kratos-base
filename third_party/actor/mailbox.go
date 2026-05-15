package actor

const minQueueSize = 32

type MailBox interface {
	Enqueue(m interface{}) error
	OutCh() <-chan interface{}
}

type defaultMailBox struct {
	outCh chan interface{}
	cap   int
}

func newDefaultMailBox(cap int) *defaultMailBox {
	if cap < minQueueSize {
		cap = minQueueSize
	}
	m := &defaultMailBox{
		outCh: make(chan interface{}, cap),
		cap:   cap,
	}

	return m
}

func (d *defaultMailBox) Enqueue(m interface{}) error {
	select {
	case d.outCh <- m:
		return nil
	default:
		return ErrMailOverflow
	}
}

func (d *defaultMailBox) OutCh() <-chan interface{} {
	return d.outCh
}
