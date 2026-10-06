//go:build windows

package captions

// LowerPriority is a no-op on Windows, where captions don't run in this
// version (research.md §12).
func LowerPriority(int) {}
