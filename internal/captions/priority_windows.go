//go:build windows

package captions

// lowerPriority is a no-op on Windows, where captions don't run in this
// version (research.md §12).
func lowerPriority(int) {}
