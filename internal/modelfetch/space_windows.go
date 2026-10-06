//go:build windows

package modelfetch

import "math"

// availableBytes doesn't check on Windows, where models aren't downloaded
// in this version (captions and summaries are Apple-silicon only); the
// download itself fails cleanly if the disk fills.
func availableBytes(string) (uint64, error) {
	return math.MaxUint64, nil
}
