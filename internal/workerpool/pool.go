// Package workerpool provides a generic bounded worker pool for running
// a function over a slice of items concurrently.
package workerpool

import (
	"context"
	"sync"
)

// Run executes fn for each item in items using up to workers goroutines.
// It returns the first non-nil error from fn, or nil if all succeed.
// All in-flight goroutines are allowed to finish even if one returns an error.
func Run[T any](ctx context.Context, items []T, workers int, fn func(context.Context, T) error) error {
	if len(items) == 0 {
		return nil
	}
	if workers <= 0 {
		workers = 1
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	var once sync.Once
	var firstErr error

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(it T) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			if err := fn(ctx, it); err != nil {
				once.Do(func() { firstErr = err })
			}
		}(item)
	}

	wg.Wait()
	return firstErr
}
