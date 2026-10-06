//go:build !windows

package captions

import "syscall"

// LowerPriority makes a helper process yield the CPU to the user's other
// work, uploads included (research.md §9). Best effort: a failure only
// means it runs at normal priority.
func LowerPriority(pid int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, pid, 10)
}
