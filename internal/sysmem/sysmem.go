// Package sysmem reads this computer's physical memory and turns it into
// how many runs the app offers to run at once.
package sysmem

// GiB is a gibibyte.
const GiB = 1 << 30

// The memory sizes that raise the ceiling. "More than" is strict: a
// computer sold as 16 GB reports 16 GiB or a little less, and is offered the
// smaller ceiling.
const (
	midMemory  = 16 * GiB
	highMemory = 64 * GiB
)

// The ceilings: how many runs may run at once on a computer with 16 GB of
// memory or less, more than 16 GB, and more than 64 GB.
const (
	LowCeiling  = 2
	MidCeiling  = 6
	HighCeiling = 20
)

// Ceiling is how many runs at once a computer with total bytes of physical
// memory is offered. A total that could not be read (0) is offered the
// smallest.
func Ceiling(total uint64) int {
	switch {
	case total > highMemory:
		return HighCeiling
	case total > midMemory:
		return MidCeiling
	}
	return LowCeiling
}

// Total is this computer's physical memory in bytes, as the operating system
// reports it; 0 with the error when it cannot be read.
func Total() (uint64, error) { return total() }

// The memory a virtual machine needs for a number of runs at once: a base
// for the engine itself and a share for each run.
const (
	vmBaseGiB   = 2
	vmPerRunGiB = 2
)

// VMMemoryGiB is the memory, in GiB, a container engine's virtual machine
// is given for n runs at once on a computer with total bytes of memory: the
// base and a share per run, at most three quarters of the computer's
// memory, and never less than the base and one run.
func VMMemoryGiB(n int, total uint64) int {
	want := vmBaseGiB + vmPerRunGiB*max(n, 1)
	if total > 0 {
		most := int(total * 3 / 4 / GiB)
		want = min(want, most)
	}
	return max(want, vmBaseGiB+vmPerRunGiB)
}

// RoundGiB is bytes in whole GiB, to the nearest: an engine reports its
// virtual machine's memory a little under the size it was given.
func RoundGiB(bytes uint64) int {
	return int((bytes + GiB/2) / GiB)
}

// RunsFit is how many runs at once a virtual machine with bytes of memory
// fits, at least one.
func RunsFit(bytes uint64) int {
	return max(1, (RoundGiB(bytes)-vmBaseGiB)/vmPerRunGiB)
}

// VMCPUs is the processors Agentry's own engine is given for n runs at once
// on a computer with cpus processors: one more than the runs, leaving the
// computer one of its own, and at least two.
func VMCPUs(n, cpus int) int {
	return max(2, min(n+1, cpus-1))
}
