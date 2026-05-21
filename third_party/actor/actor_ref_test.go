package actor

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDefaultActorRefRequest(t *testing.T) {
	t.Run("request self", func(t *testing.T) {
		defMail := newDefaultMailBox(10)
		def := NewDefaultActorRef(defMail, NewPID(1, "1"))

		resp := def.Request(&Message{
			from: NewPID(1, "1"),
			to:   NewPID(1, "1"),
		})

		if !errors.Is(resp.Err, ErrSyncRequestSelf) {
			t.Fatalf("expected ErrSyncRequestSelf, got %v", resp.Err)
		}
	})

	t.Run("request normal return", func(t *testing.T) {
		defMail := newDefaultMailBox(10)
		def := NewDefaultActorRef(defMail, NewPID(1, "1"))

		msg := &Message{}
		resp := RespMessage{Data: "test"}
		go func() {
			v := <-defMail.outCh
			pending := v.(*Message)
			pending.Response(resp)
		}()

		got := def.Request(msg)
		if got.Err != nil {
			t.Fatalf("unexpected error: %v", got.Err)
		}
		if got.Data != resp.Data {
			t.Fatalf("response data = %v, want %v", got.Data, resp.Data)
		}
	})

	t.Run("request timeout", func(t *testing.T) {
		defMail := newDefaultMailBox(10)
		def := NewDefaultActorRef(defMail, NewPID(1, "1"))

		msg := &Message{}
		resp := RespMessage{Data: "test"}
		var pending *Message
		wt := sync.WaitGroup{}
		wt.Add(1)
		go func() {
			defer wt.Done()
			time.Sleep(3100 * time.Millisecond)
			v := <-defMail.outCh
			pending = v.(*Message)
			pending.Response(resp)
		}()

		got := def.Request(msg)
		if !errors.Is(got.Err, ErrTimeOut) {
			t.Fatalf("expected ErrTimeOut, got %v", got.Err)
		}

		wt.Wait()
		if pending == nil {
			t.Fatal("expected pending message after timeout")
		}
		if len(pending.respCh) != 1 {
			t.Fatalf("pending.respCh length = %d, want 1", len(pending.respCh))
		}
	})

	t.Run("mail full", func(t *testing.T) {
		limitMail := newDefaultMailBox(1)
		ref := NewDefaultActorRef(limitMail, NewPID(2, "2"))
		for i := 0; i < minQueueSize; i++ {
			_ = ref.Send(&Message{})
		}

		if err := ref.Send(&Message{}); !errors.Is(err, ErrMailOverflow) {
			t.Fatalf("Send() error = %v, want ErrMailOverflow", err)
		}

		if resp := ref.Request(&Message{}); !errors.Is(resp.Err, ErrMailOverflow) {
			t.Fatalf("Request() error = %v, want ErrMailOverflow", resp.Err)
		}

		done := make(chan RespMessage, 1)
		ref.AsyncRequest(&Message{}, func(msg RespMessage) {
			done <- msg
		})
		select {
		case msg := <-done:
			if msg.Err == nil {
				t.Fatal("expected AsyncRequest error when mailbox is full")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting AsyncRequest callback")
		}
	})
}
