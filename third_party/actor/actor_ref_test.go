package actor

import (
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultActorRef_Request(t *testing.T) {

	Convey("Given DefaultActorRef_Request", t, func() {
		defMail := newDefaultMailBox(10)
		def := NewDefaultActorRef(defMail, NewPID(1, "1"))
		Convey("When Request self", func() {
			resp := def.Request(&Message{
				from: NewPID(1, "1"),
				to:   NewPID(1, "1"),
			})

			So(errors.Is(resp.Err, ErrSyncRequestSelf), ShouldBeTrue)
		})

		Convey("When Request normal return", func() {
			msg := &Message{}
			resp := RespMessage{
				Data: "test",
			}
			go func() {
				v := <-defMail.outCh
				msg := v.(*Message)
				msg.Response(resp)
			}()
			rsp2 := def.Request(msg)
			So(rsp2.Data, ShouldEqual, resp.Data)
			So(rsp2.Err, ShouldBeNil)
		})

		Convey("When Request Timeout", func() {
			msg := &Message{}
			resp := RespMessage{
				Data: "test",
			}
			wt := sync.WaitGroup{}
			wt.Add(1)
			go func() {
				defer wt.Done()
				time.Sleep(3100 * time.Millisecond)
				v := <-defMail.outCh
				msg = v.(*Message)
				msg.Response(resp)
			}()
			rsp2 := def.Request(msg)
			So(errors.Is(rsp2.Err, ErrTimeOut), ShouldBeTrue)

			wt.Wait()
			So(len(msg.respCh), ShouldEqual, 1)
		})

		limitMail := newDefaultMailBox(1)
		a := NewDefaultActorRef(limitMail, NewPID(2, "2"))
		for i := 0; i < minQueueSize; i++ {
			a.Send(&Message{})
		}
		Convey("When mail full", func() {
			Convey("When Send", func() {
				err := a.Send(&Message{})
				So(errors.Is(err, ErrMailOverflow), ShouldBeTrue)
			})
			Convey("When Request", func() {
				resp := a.Request(&Message{})
				So(errors.Is(resp.Err, ErrMailOverflow), ShouldBeTrue)
			})
			Convey("When AsyncRequest", func() {
				a.AsyncRequest(&Message{}, func(msg RespMessage) {
					So(msg.Err, ShouldNotBeNil)
				})
			})
		})
	})

}
