package auth

import "time"

// timeoutCh returns a channel that fires after a generous bound, used so a
// broken loopback listener fails the test with a clear message instead of
// hanging the test run indefinitely.
func timeoutCh() <-chan time.Time {
	return time.After(5 * time.Second)
}
