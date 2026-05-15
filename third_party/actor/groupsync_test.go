package actor

import (
	"testing"
	"time"
)

func TestGroupSyncTimeout(t *testing.T) {
	gs := NewGroupSync(time.Millisecond * 100)
	gs.Add(2)
	go func() {
		time.Sleep(time.Millisecond * 10)
		gs.Done()
		gs.Pause()
	}()

	go func() {
		time.Sleep(time.Millisecond * 200)
		gs.Done()
		gs.Pause()
	}()

	gs.Wait(func(b bool) {
		if b != false {
			t.Fatal("group sync time out return error")
		}
	})
}

func TestGroupSyncSuccess(t *testing.T) {
	gs := NewGroupSync(time.Millisecond * 100)
	gs.Add(2)
	go func() {
		time.Sleep(time.Millisecond * 10)
		gs.Done()
		gs.Pause()
	}()

	go func() {
		time.Sleep(time.Millisecond * 30)
		gs.Done()
		gs.Pause()
	}()

	gs.Wait(func(b bool) {
		if b != true {
			t.Fatal("group sync return error")
		}
	})
}
