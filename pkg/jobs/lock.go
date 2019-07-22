package jobs

import "sync/atomic"

//
// The sync/Mutex library only offers blocking Lock/Unlock mechanisms.
//
// We are rolling our own non-blocking lock.
//
// Usage:
//
//  l := Lock()
//
//  if l.TryLock() {
//     fmt.Printf("In the lock")
//  } else {
//     fmt.Printf("Failed to acquire lock")
//  }
//

type Lock struct {
	state int32 // zero state of int32 variables in Go is 0
}

func (l *Lock) TryLock() bool {
	return atomic.CompareAndSwapInt32(&l.state, 0, 1)
}
