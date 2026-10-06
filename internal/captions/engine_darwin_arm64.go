//go:build darwin && arm64

package captions

// platformSupported is true only on Apple-silicon Macs, the one platform
// captions ship on (research.md §12).
const platformSupported = true
