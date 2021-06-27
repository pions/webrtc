package webrtc

import "sync/atomic"

type atomicBool struct {
	val int32
}

func (b *atomicBool) set(value bool) { // nolint: unparam
	var i int32
	if value {
		i = 1
	}

	atomic.StoreInt32(&b.val, i)
}

func (b *atomicBool) get() bool {
	return atomic.LoadInt32(&b.val) != 0
}

func (b *atomicBool) compareAndSwap(old, new bool) (swapped bool) {
	var oldval, newval int32
	if old {
		oldval = 1
	}
	if new {
		newval = 1
	}
	return atomic.CompareAndSwapInt32(&b.val, oldval, newval)
}
