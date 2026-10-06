package heavywork

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOnlyOneHolderAtATime(t *testing.T) {
	release, err := Acquire(context.Background(), "captions")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got := Holder(); got != "captions" {
		t.Fatalf("Holder() = %q, want captions", got)
	}

	acquired := make(chan func())
	go func() {
		r, err := Acquire(context.Background(), "summaries")
		if err != nil {
			t.Errorf("second Acquire: %v", err)
		}
		acquired <- r
	}()

	select {
	case <-acquired:
		t.Fatal("second Acquire succeeded while the lock was held")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case r := <-acquired:
		if got := Holder(); got != "summaries" {
			t.Errorf("Holder() after hand-over = %q, want summaries", got)
		}
		r()
	case <-time.After(time.Second):
		t.Fatal("waiter never got the lock after it was released")
	}
	if got := Holder(); got != "" {
		t.Errorf("Holder() when free = %q, want empty", got)
	}
}

func TestCancelledWaiterDoesNotAcquire(t *testing.T) {
	release, _ := Acquire(context.Background(), "captions")
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := Acquire(ctx, "summaries")
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Acquire err = %v, want context.Canceled", err)
	}
	if got := Holder(); got != "captions" {
		t.Fatalf("Holder() = %q; a cancelled waiter must not take the lock", got)
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	release, _ := Acquire(context.Background(), "captions")
	release()
	release() // a second call must not free a lock someone else now holds
	r2, _ := Acquire(context.Background(), "summaries")
	release()
	if got := Holder(); got != "summaries" {
		t.Fatalf("Holder() = %q; a stale release freed the new holder's lock", got)
	}
	r2()
}
