//go:build !(darwin && arm64)

package captions

// platformSupported is false everywhere but Apple-silicon Macs: Intel Macs,
// Windows and Linux report "Captions aren't available on this system yet"
// (FR-017, research.md §12).
const platformSupported = false
