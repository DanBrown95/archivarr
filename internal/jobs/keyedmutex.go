package jobs

import "sync"

// keyedMutex provides one mutex per integer key, created on demand. Used to
// serialize writes to the same physical destination drive.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[int64]*sync.Mutex)}
}

// Lock acquires the mutex for key and returns its unlock function.
func (k *keyedMutex) Lock(key int64) func() {
	k.mu.Lock()
	l, ok := k.locks[key]
	if !ok {
		l = &sync.Mutex{}
		k.locks[key] = l
	}
	k.mu.Unlock()

	l.Lock()
	return l.Unlock
}
