package app

import (
	"sync"
	"testing"
	"time"
)

// serialMainThread models production's serialized UI callbacks. Tests must
// use do for model reads or writes that can overlap work posted by an action.
type serialMainThread struct {
	mu sync.Mutex
}

func (s *serialMainThread) post(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

func (s *serialMainThread) do(fn func()) {
	s.post(fn)
}

func waitForSerialMainThread(
	t *testing.T,
	main *serialMainThread,
	timeout time.Duration,
	condition func() bool,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var done bool
		main.do(func() {
			done = condition()
		})
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("serial main-thread condition did not become true")
}
