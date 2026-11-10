package sysmem

import "testing"

// The ceiling rises only above 16 GB and above 64 GB of memory, and a
// computer whose memory could not be read is offered the smallest.
func TestTheCeilingFollowsTheMemory(t *testing.T) {
	for _, c := range []struct {
		total uint64
		want  int
	}{
		{0, 2},
		{8 * GiB, 2},
		{15*GiB + 600<<20, 2},
		{16 * GiB, 2},
		{16*GiB + 1, 6},
		{32 * GiB, 6},
		{64 * GiB, 6},
		{64*GiB + 1, 20},
		{128 * GiB, 20},
	} {
		if got := Ceiling(c.total); got != c.want {
			t.Errorf("Ceiling(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

// This computer's memory reads as something.
func TestTotalReadsThisComputersMemory(t *testing.T) {
	n, err := Total()
	if err != nil || n < 256<<20 {
		t.Fatalf("Total() = %d, %v", n, err)
	}
}
