package rt_test

import (
	"sort"
	"testing"

	"github.com/zatrano/canvas/rt"
)

func TestReleaseWriter_BenchOverheadDefault(t *testing.T) {
	const rounds = 10
	times := make([]int64, 0, rounds)
	for r := 0; r < rounds; r++ {
		n := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				w := rt.AcquireWriter()
				w.WriteString("abcdefghijklmnopqrstuvwxyz0123456789")
				rt.ReleaseWriter(w)
			}
		})
		times = append(times, n.NsPerOp())
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	med := times[len(times)/2]
	t.Logf("ReleaseWriter ns/op min=%d median=%d max=%d (count=%d)", times[0], med, times[len(times)-1], rounds)
}
