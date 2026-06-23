package jobs

import (
	"sync"
	"testing"
	"time"
)

func TestKeyedMutexSerializesSameKey(t *testing.T) {
	km := newKeyedMutex()
	var mu sync.Mutex
	concurrent, maxConcurrent := 0, 0

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := km.Lock(1) // same key for everyone
			defer unlock()
			mu.Lock()
			concurrent++
			if concurrent > maxConcurrent {
				maxConcurrent = concurrent
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			concurrent--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if maxConcurrent != 1 {
		t.Fatalf("same-key critical sections overlapped: max concurrent = %d", maxConcurrent)
	}
}

func TestKeyedMutexDifferentKeysDoNotBlock(t *testing.T) {
	km := newKeyedMutex()
	unlock1 := km.Lock(1)
	defer unlock1()

	done := make(chan struct{})
	go func() {
		unlock2 := km.Lock(2) // different key — must not wait on key 1
		unlock2()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("locking a different key blocked on an unrelated key")
	}
}
