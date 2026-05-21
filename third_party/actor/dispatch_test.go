package actor

import "testing"

func TestDispatcherAddBlackId(t *testing.T) {
	t.Run("unknown rpc stays false", func(t *testing.T) {
		d := NewDispatcher(10, func(uid uint64, id uint32, ms int64) {})
		d.AddBlackId(5, []uint64{1, 2, 3})
		if d.IsBlackId(2, 1) {
			t.Fatal("expected rpc 2 uid 1 not to be blacklisted")
		}
	})

	t.Run("add all uid twice", func(t *testing.T) {
		d := NewDispatcher(10, func(uid uint64, id uint32, ms int64) {})
		d.AddBlackId(1, []uint64{})
		if !d.IsBlackId(1, 1) {
			t.Fatal("expected rpc 1 uid 1 to be blacklisted")
		}
		d.AddBlackId(1, []uint64{})
		if !d.IsBlackId(1, 1) {
			t.Fatal("expected rpc 1 uid 1 to remain blacklisted")
		}
	})

	t.Run("replace uid list", func(t *testing.T) {
		d := NewDispatcher(10, func(uid uint64, id uint32, ms int64) {})
		d.AddBlackId(1, []uint64{1, 2})
		if !d.IsBlackId(1, 1) || !d.IsBlackId(1, 2) {
			t.Fatal("expected initial uid list to be blacklisted")
		}

		d.AddBlackId(1, []uint64{3, 4})
		if d.IsBlackId(1, 1) || d.IsBlackId(1, 2) {
			t.Fatal("expected old uid list to be replaced")
		}
		if !d.IsBlackId(1, 3) || !d.IsBlackId(1, 4) {
			t.Fatal("expected new uid list to be blacklisted")
		}
		if d.IsBlackId(0, 4) || d.IsBlackId(6, 3) {
			t.Fatal("expected unrelated rpc ids not to be blacklisted")
		}
	})

	t.Run("uid zero is never blocked", func(t *testing.T) {
		d := NewDispatcher(10, func(uid uint64, id uint32, ms int64) {})
		d.AddBlackId(1, []uint64{})
		if d.IsBlackId(1, 0) {
			t.Fatal("expected uid 0 not to be blocked")
		}
	})
}
