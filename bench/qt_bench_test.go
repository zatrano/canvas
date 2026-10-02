package bench_test

import (
	"fmt"
	"testing"

	qt "github.com/valyala/quicktemplate"

	"github.com/zatrano/canvas/rt"
)

const benchItems = 50

// --- quicktemplate-class competitor (hand-written = what qtc emits) ---

type qtItem struct{ Name string }

func streamQTPage(qw *qt.Writer, title string, items []qtItem) {
	qw.N().S(`<h1>`)
	qw.E().S(title)
	qw.N().S(`</h1>`)
	for i := 0; i < len(items); i++ {
		qw.N().S(`<li>`)
		qw.E().S(items[i].Name)
		qw.N().S(`</li>`)
	}
}

func BenchmarkQuickTemplate(b *testing.B) {
	items := make([]qtItem, benchItems)
	for i := range items {
		items[i].Name = fmt.Sprintf("Item-%d", i)
	}
	title := "Bench"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := qt.AcquireByteBuffer()
		w := qt.AcquireWriter(buf)
		streamQTPage(w, title, items)
		qt.ReleaseWriter(w)
		_ = buf.B
		qt.ReleaseByteBuffer(buf)
	}
}

// BenchmarkCanvasTyped is Canvas typed stream (canvas gen output class).
func BenchmarkCanvasTyped(b *testing.B) {
	items := make([]rt.ListItem, benchItems)
	for i := range items {
		items[i].Name = fmt.Sprintf("Item-%d", i)
	}
	title := "Bench"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		rt.StreamListPage(w, title, items)
		_ = w.Bytes()
		rt.ReleaseWriter(w)
	}
}

// BenchmarkCanvasTypedNames is the flattest Canvas typed path.
func BenchmarkCanvasTypedNames(b *testing.B) {
	names := make([]string, benchItems)
	for i := range names {
		names[i] = fmt.Sprintf("Item-%d", i)
	}
	title := "Bench"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		rt.StreamListNames(w, title, names)
		_ = w.Bytes()
		rt.ReleaseWriter(w)
	}
}
