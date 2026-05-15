package actor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDispatcher_AddBlackId(t *testing.T) {
	Convey("Given a NewDispatcher", t, func() {
		d := NewDispatcher(10, func(uid uint64, id uint32, ms int64) {

		})
		Convey("args error", func() {
			d.AddBlackId(5, 1, []uint64{1, 2, 3})
			So(d.IsBlackId(2, 1), ShouldEqual, false)
		})
		Convey("add all uid twice", func() {
			d.AddBlackId(1, 5, []uint64{})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
			d.AddBlackId(1, 5, []uint64{})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
		})
		Convey("add uid list twice", func() {
			d.AddBlackId(1, 5, []uint64{1, 2})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
			So(d.IsBlackId(2, 2), ShouldEqual, true)
			d.AddBlackId(1, 5, []uint64{3, 4})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
			So(d.IsBlackId(2, 2), ShouldEqual, true)
			So(d.IsBlackId(2, 3), ShouldEqual, true)
			So(d.IsBlackId(2, 4), ShouldEqual, true)

			So(d.IsBlackId(0, 4), ShouldEqual, false)
			So(d.IsBlackId(6, 3), ShouldEqual, false)
		})
		Convey("add uid list after add all uid", func() {
			d.AddBlackId(1, 5, []uint64{})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
			d.AddBlackId(1, 5, []uint64{3, 4})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
		})
		Convey("add all uid after add uid list", func() {
			d.AddBlackId(1, 5, []uint64{1, 2})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
			So(d.IsBlackId(2, 2), ShouldEqual, true)
			So(d.IsBlackId(2, 3), ShouldEqual, false)
			d.AddBlackId(1, 5, []uint64{})
			So(d.IsBlackId(2, 1), ShouldEqual, true)
			So(d.IsBlackId(2, 2), ShouldEqual, true)
			So(d.IsBlackId(2, 3), ShouldEqual, true)
		})
	})
}

func TestDispatcher(t *testing.T) {
	//d := NewDispatcher(0, 100, 100)
	//index := 0
	//d.Register(1, func(ctx context.Context,  data Message, retCb func(*Message, error)) {
	//	index++
	//	if index == 2 {
	//		retCb(nil, fmt.Errorf("id 1 return err"))
	//		return
	//	}
	//	t.Logf("handler id %d uid %d", data.Id, data.Uid)
	//	retCb(nil, nil)
	//})
	//
	//d.Register(2, func(ctx context.Context,  data Message, retCb func(*Message, error)) {
	//	t.Logf("handler id %d uid %d", data.Id, data.Uid)
	//})
	//cb := func(message *Message, err error) {
	//	t.Log(message, err)
	//}
	//
	//d.Dispatch(context.TODO(), 1, 1, nil, cb)
	//
	//d.Dispatch(context.TODO(), 1, 1, nil, cb)
	//
	//d.AddBlackId(1, 0)
	//d.Dispatch(context.TODO(), 1, 1, nil, cb)
	//
	//d.DeleteBlackId(1, 0)
	//d.Dispatch(context.TODO(), 1, 1, nil, cb)
	//
	//d.AddBlackId(1, 1)
	//d.Dispatch(context.TODO(), 1, 1, nil, cb)
	//d.Dispatch(context.TODO(), 2, 1, nil, cb)
	//d.Dispatch(context.TODO(), 0, 1, nil, cb)
	//
	//d.DeleteBlackId(1, 1)
	//
	//d.Dispatch(context.TODO(), 1, 1, nil, cb)
	//d.Dispatch(context.TODO(), 2, 1, nil, cb)
}
