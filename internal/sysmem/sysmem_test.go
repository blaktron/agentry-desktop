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

// A virtual machine gets 2 GB and 2 GB a run, at most three quarters of the
// computer's memory; what it fits is read back from what the engine reports.
func TestTheVMIsSizedForTheRuns(t *testing.T) {
	for _, c := range []struct {
		n     int
		total uint64
		want  int
	}{
		{1, 16 * GiB, 4},
		{2, 16 * GiB, 6},
		{6, 32 * GiB, 14},
		{20, 128 * GiB, 42},
		{20, 65 * GiB, 42},
		{6, 17 * GiB, 12},
		{2, 0, 6},
	} {
		if got := VMMemoryGiB(c.n, c.total); got != c.want {
			t.Errorf("VMMemoryGiB(%d, %d GiB) = %d, want %d", c.n, c.total/GiB, got, c.want)
		}
	}
	for _, c := range []struct {
		bytes uint64
		want  int
	}{
		{0, 1},
		{2 * GiB, 1},
		{4*GiB - 300<<20, 1},
		{6*GiB - 200<<20, 2},
		{14 * GiB, 6},
		{42 * GiB, 20},
	} {
		if got := RunsFit(c.bytes); got != c.want {
			t.Errorf("RunsFit(%d MiB) = %d, want %d", c.bytes>>20, got, c.want)
		}
	}
	for _, c := range []struct{ n, cpus, want int }{{1, 8, 2}, {2, 8, 3}, {6, 8, 7}, {20, 8, 7}, {6, 2, 2}} {
		if got := VMCPUs(c.n, c.cpus); got != c.want {
			t.Errorf("VMCPUs(%d, %d) = %d, want %d", c.n, c.cpus, got, c.want)
		}
	}
}
