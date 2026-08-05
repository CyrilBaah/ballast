package drive

import "testing"

// TestChunkSizePolicyOnSuccessGrowsEveryThirdConsecutiveSuccess covers
// FR-003/FR-004/FR-008: growth only happens on the GrowthThreshold-th
// consecutive success, doubles the size, and resets the streak either way.
func TestChunkSizePolicyOnSuccessGrowsEveryThirdConsecutiveSuccess(t *testing.T) {
	p := NewChunkSizePolicy(BaselineChunkSize)

	// First two successes: no growth yet, streak just accumulates.
	p.OnSuccess()
	if p.Size != BaselineChunkSize {
		t.Fatalf("after 1st success: Size = %d, want unchanged %d", p.Size, BaselineChunkSize)
	}
	if p.ConsecutiveSuccesses != 1 {
		t.Fatalf("after 1st success: ConsecutiveSuccesses = %d, want 1", p.ConsecutiveSuccesses)
	}
	p.OnSuccess()
	if p.Size != BaselineChunkSize {
		t.Fatalf("after 2nd success: Size = %d, want unchanged %d", p.Size, BaselineChunkSize)
	}

	// 3rd consecutive success: doubles and resets the streak.
	p.OnSuccess()
	if p.Size != BaselineChunkSize*2 {
		t.Fatalf("after 3rd success: Size = %d, want %d", p.Size, BaselineChunkSize*2)
	}
	if p.ConsecutiveSuccesses != 0 {
		t.Fatalf("after growth: ConsecutiveSuccesses = %d, want reset to 0", p.ConsecutiveSuccesses)
	}
}

// TestChunkSizePolicyOnSuccessCapsAtMaxChunkSize covers FR-004: growth
// never exceeds the ceiling, no matter how many further successes occur.
func TestChunkSizePolicyOnSuccessCapsAtMaxChunkSize(t *testing.T) {
	p := NewChunkSizePolicy(MaxChunkSize)

	for i := 0; i < 30; i++ {
		p.OnSuccess()
		if p.Size > MaxChunkSize {
			t.Fatalf("after %d successes: Size = %d, exceeded ceiling %d", i+1, p.Size, MaxChunkSize)
		}
	}
	if p.Size != MaxChunkSize {
		t.Fatalf("Size = %d, want to stay at ceiling %d", p.Size, MaxChunkSize)
	}
}

// TestChunkSizePolicyOnSuccessReachesCeilingFromBaseline walks the full
// growth sequence from baseline to confirm every intermediate step is
// correct and a 256 KiB-multiple (FR-007), not just the endpoints.
func TestChunkSizePolicyOnSuccessReachesCeilingFromBaseline(t *testing.T) {
	p := NewChunkSizePolicy(BaselineChunkSize)
	wantSizesAfterEachGrowth := []int64{16 * 1024 * 1024, 32 * 1024 * 1024, 64 * 1024 * 1024}

	for _, want := range wantSizesAfterEachGrowth {
		for i := 0; i < GrowthThreshold; i++ {
			p.OnSuccess()
		}
		if p.Size != want {
			t.Fatalf("Size = %d, want %d", p.Size, want)
		}
		if p.Size%(256*1024) != 0 {
			t.Fatalf("Size = %d is not a 256 KiB multiple", p.Size)
		}
	}
}

// TestChunkSizePolicyOnFailureHalvesImmediately covers FR-005: any
// retried failure shrinks the size right away, regardless of how far the
// success streak had progressed, and resets the streak (FR-008).
func TestChunkSizePolicyOnFailureHalvesImmediately(t *testing.T) {
	p := NewChunkSizePolicy(32 * 1024 * 1024)
	p.ConsecutiveSuccesses = 2 // one success away from the next growth

	p.OnFailure()

	if p.Size != 16*1024*1024 {
		t.Fatalf("Size = %d, want %d", p.Size, 16*1024*1024)
	}
	if p.ConsecutiveSuccesses != 0 {
		t.Fatalf("ConsecutiveSuccesses = %d, want reset to 0", p.ConsecutiveSuccesses)
	}
}

// TestChunkSizePolicyOnFailureFloorsAtMinChunkSize covers FR-006: repeated
// consecutive failures never drop the size below the floor.
func TestChunkSizePolicyOnFailureFloorsAtMinChunkSize(t *testing.T) {
	p := NewChunkSizePolicy(MinChunkSize)

	for i := 0; i < 10; i++ {
		p.OnFailure()
		if p.Size < MinChunkSize {
			t.Fatalf("after %d failures: Size = %d, dropped below floor %d", i+1, p.Size, MinChunkSize)
		}
	}
	if p.Size != MinChunkSize {
		t.Fatalf("Size = %d, want to stay at floor %d", p.Size, MinChunkSize)
	}
}

// TestChunkSizePolicyGrowthResumesGraduallyAfterShrink covers Acceptance
// Scenario 3 of User Story 2: a shrink doesn't reset growth back to an
// immediate jump -- the same GrowthThreshold consecutive successes are
// required again before the next doubling.
func TestChunkSizePolicyGrowthResumesGraduallyAfterShrink(t *testing.T) {
	p := NewChunkSizePolicy(32 * 1024 * 1024)
	p.OnFailure() // -> 16 MiB, streak reset

	p.OnSuccess()
	p.OnSuccess()
	if p.Size != 16*1024*1024 {
		t.Fatalf("Size = %d after only 2 post-failure successes, want unchanged %d (not an immediate jump back to 32 MiB)", p.Size, 16*1024*1024)
	}
	p.OnSuccess() // 3rd consecutive success since the shrink
	if p.Size != 32*1024*1024 {
		t.Fatalf("Size = %d after 3 post-failure successes, want %d", p.Size, 32*1024*1024)
	}
}
