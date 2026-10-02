package rt_test

import (
	"testing"

	"github.com/zatrano/canvas/rt"
)

func BenchmarkReleaseWriter(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		w.WriteString("abcdefghijklmnopqrstuvwxyz0123456789")
		rt.ReleaseWriter(w)
	}
}
