package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// BenchTasks is the outcome of the benchmark's concurrent-request scenario:
// how many searches ran at once, how many the backend answered, and how long
// the whole batch took.
type BenchTasks struct {
	Tasks     int   `json:"tasks"`
	Done      int   `json:"done"`
	Failed    int   `json:"failed"`
	Peak      int   `json:"peak"`
	ElapsedMS int64 `json:"elapsedMs"`
}

// BenchConcurrentSearches issues n project searches at once, each on its own
// channel, and waits for every reply. It is the 20-concurrent-tasks measurement:
// what matters is that the manager frames interleaved replies and the backend
// queue stays bounded when the work arrives together. The searches are not shown
// in the sidebar; the visible search is left alone.
func (m *Model) BenchConcurrentSearches(root string, n int) BenchTasks {
	out := BenchTasks{Tasks: n}
	if root == "" || n <= 0 || m.mgr == nil {
		return out
	}
	var done, failed, inflight, peak atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cur := inflight.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			defer inflight.Add(-1)
			ch := m.nextChannel()
			ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
			defer cancel()
			_, err := m.mgr.Invoke(ctx, Window, "search_start", map[string]any{
				"request": ch,
				"query": map[string]any{
					"pattern": "class", "isRegex": false, "caseSensitive": false,
					"wholeWord": false, "include": []string{}, "exclude": []string{},
					"scope": map[string]any{"kind": "project"}, "maxMatches": 100,
				},
				"onEvent": map[string]any{"$channel": ch},
			})
			if err != nil {
				failed.Add(1)
			} else {
				done.Add(1)
			}
		}()
	}
	wg.Wait()
	out.Done = int(done.Load())
	out.Failed = int(failed.Load())
	out.Peak = int(peak.Load())
	out.ElapsedMS = time.Since(start).Milliseconds()
	return out
}
