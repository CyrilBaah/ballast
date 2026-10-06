// Package heavywork is the one-model-at-a-time lock shared by captions
// (Feature 005) and summaries (Feature 006). On an 8 GB Mac the speech
// model and a summary model together would push the machine into heavy
// swapping and slow everything, uploads included, so whichever one is
// running holds this lock and the other waits.
package heavywork

import (
	"context"
	"sync"
)

var (
	slot   = make(chan struct{}, 1)
	mu     sync.Mutex
	holder string
)

// Acquire waits until the lock is free, takes it on behalf of owner, and
// returns a release func. It returns ctx.Err() without taking the lock if
// ctx ends first. Calling release more than once is harmless.
func Acquire(ctx context.Context, owner string) (release func(), err error) {
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	mu.Lock()
	holder = owner
	mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			holder = ""
			mu.Unlock()
			<-slot
		})
	}, nil
}

// Holder names who holds the lock, or "" when it is free.
func Holder() string {
	mu.Lock()
	defer mu.Unlock()
	return holder
}
