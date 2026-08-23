package concurrency

import (
	"os"
	"runtime"
	"strconv"
	"sync"
)

// DefaultReserveCores left for the event loop / host during indexing.
const DefaultReserveCores = 2

func workerCount(concurrency, itemCount int) int {
	n := concurrency
	if n > itemCount {
		n = itemCount
	}
	if n < 1 {
		return 1
	}
	return n
}

// ResolveWorkerConcurrency returns the worker pool size for CPU-bound index work.
func ResolveWorkerConcurrency() int {
	raw := os.Getenv("MIRU_CONCURRENCY")
	if raw == "" {
		raw = os.Getenv("MIRU_WORKERS")
	}
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 1 {
			return n
		}
	}
	cores := runtime.NumCPU() - DefaultReserveCores
	if cores < 1 {
		return 1
	}
	return cores
}

// MapPool runs worker over items with at most concurrency in-flight workers.
// Results are stored in input order.
func MapPool[T any, R any](items []T, concurrency int, worker func(item T, index int) R) []R {
	results := make([]R, len(items))
	if len(items) == 0 {
		return results
	}
	n := workerCount(concurrency, len(items))
	var mu sync.Mutex
	next := 0
	var wg sync.WaitGroup
	wg.Add(n)
	for w := 0; w < n; w++ {
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				i := next
				if i >= len(items) {
					mu.Unlock()
					return
				}
				next++
				mu.Unlock()
				results[i] = worker(items[i], i)
			}
		}()
	}
	wg.Wait()
	return results
}

// MapPoolErr is like MapPool but workers may return an error; first error wins.
func MapPoolErr[T any, R any](items []T, concurrency int, worker func(item T, index int) (R, error)) ([]R, error) {
	type slot struct {
		v   R
		err error
	}
	slots := MapPool(items, concurrency, func(item T, index int) slot {
		v, err := worker(item, index)
		return slot{v: v, err: err}
	})
	results := make([]R, len(items))
	for i, s := range slots {
		if s.err != nil {
			return nil, s.err
		}
		results[i] = s.v
	}
	return results, nil
}
