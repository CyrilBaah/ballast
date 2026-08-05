package drive

// Chunk-size bounds and growth cadence for the AIMD-style adaptive policy
// (research.md §1), adopted as a starting hypothesis from
// Ballast_Project_Problem_Statement.md -- per Constitution Principle III,
// these are pending validation against the network-simulation harness, not
// settled defaults.
const (
	// BaselineChunkSize is the chunk size every new upload starts at (FR-002).
	BaselineChunkSize int64 = 8 * 1024 * 1024
	// MinChunkSize is the floor a shrinking chunk size never drops below (FR-006).
	MinChunkSize int64 = 1 * 1024 * 1024
	// MaxChunkSize is the ceiling a growing chunk size never exceeds (FR-004).
	MaxChunkSize int64 = 64 * 1024 * 1024
	// GrowthThreshold is how many consecutive Drive-acknowledged chunks are
	// required before the chunk size doubles (FR-003).
	GrowthThreshold = 3
)

// ChunkSizePolicy tracks one upload's current chunk size and how many
// consecutive chunks have succeeded since it last changed (the spec's
// Chunk-Size State entity).
type ChunkSizePolicy struct {
	Size                 int64
	ConsecutiveSuccesses int
}

// NewChunkSizePolicy returns a policy starting at size -- BaselineChunkSize
// for a brand-new upload, or a restored size carried in from a prior
// checkpoint for a resumed one (FR-009) -- with no consecutive successes
// recorded yet.
func NewChunkSizePolicy(size int64) *ChunkSizePolicy {
	return &ChunkSizePolicy{Size: size}
}

// OnSuccess records one more Drive-acknowledged chunk. Every
// GrowthThreshold-th consecutive success doubles Size, capped at
// MaxChunkSize, and resets the streak either way (FR-003/FR-004/FR-008).
func (p *ChunkSizePolicy) OnSuccess() {
	p.ConsecutiveSuccesses++
	if p.ConsecutiveSuccesses < GrowthThreshold {
		return
	}
	p.ConsecutiveSuccesses = 0
	next := p.Size * 2
	if next > MaxChunkSize {
		next = MaxChunkSize
	}
	p.Size = next
}

// OnFailure records one retried chunk-send failure. It halves Size
// immediately, floored at MinChunkSize, and resets the consecutive-success
// streak so growth resumes gradually rather than jumping back to the size
// that just failed (FR-005/FR-006/FR-008).
func (p *ChunkSizePolicy) OnFailure() {
	p.ConsecutiveSuccesses = 0
	next := p.Size / 2
	if next < MinChunkSize {
		next = MinChunkSize
	}
	p.Size = next
}
