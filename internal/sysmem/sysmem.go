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
