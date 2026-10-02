package bench_test

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	qt "github.com/valyala/quicktemplate"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

// CI performance contract (v0.1.0): Canvas must beat quicktemplate on typed
// and dynamic list-page shapes, and crush the legacy html/template+dataGet path.
//
//	go test -run 'Gate' -v
func TestGateCanvasBeatsQuickTemplateTyped(t *testing.T) {
	const n = 50
	items := make([]rt.ListItem, n)
	qtItems := make([]qtItem, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("Item-%d", i)
		items[i].Name = name
		qtItems[i].Name = name
	}
	title := "Bench"

	canvasNs := medianNs(t, 7, func() {
		w := rt.AcquireWriter()
		rt.StreamListPage(w, title, items)
		_ = w.Bytes()
		rt.ReleaseWriter(w)
	})
	qtNs := medianNs(t, 7, func() {
		buf := qt.AcquireByteBuffer()
		w := qt.AcquireWriter(buf)
		streamQTPage(w, title, qtItems)
		qt.ReleaseWriter(w)
		_ = buf.B
		qt.ReleaseByteBuffer(buf)
	})
	if canvasNs < 1 || qtNs < 1 {
		t.Fatalf("timing failed: canvas=%.0f qt=%.0f", canvasNs, qtNs)
	}
	ratio := qtNs / canvasNs
	t.Logf("CanvasTyped=%.0fns QuickTemplate=%.0fns ratio=%.2fx", canvasNs, qtNs, ratio)
	if ratio < 2.0 {
		t.Fatalf("typed Canvas must be ≥2× faster than quicktemplate; got %.2fx (%.0f vs %.0f ns)", ratio, qtNs, canvasNs)
	}
}

func TestGateCanvasDynamicBeatsQuickTemplate(t *testing.T) {
	dir := t.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := canvas.New(dir)
	data := gateBenchData(50)
	wWarm := rt.AcquireWriter()
	if err := eng.RenderTo(wWarm, "page", data); err != nil {
		t.Fatal(err)
	}
	rt.ReleaseWriter(wWarm)

	const n = 50
	qtItems := make([]qtItem, n)
	for i := 0; i < n; i++ {
		qtItems[i].Name = fmt.Sprintf("Item-%d", i)
	}

	canvasNs := medianNs(t, 7, func() {
		w := rt.AcquireWriter()
		_ = eng.RenderTo(w, "page", data)
		rt.ReleaseWriter(w)
	})
	qtNs := medianNs(t, 7, func() {
		buf := qt.AcquireByteBuffer()
		w := qt.AcquireWriter(buf)
		streamQTPage(w, "Bench", qtItems)
		qt.ReleaseWriter(w)
		_ = buf.B
		qt.ReleaseByteBuffer(buf)
	})
	if canvasNs < 1 || qtNs < 1 {
		t.Fatalf("timing failed: canvas=%.0f qt=%.0f", canvasNs, qtNs)
	}
	ratio := qtNs / canvasNs
	t.Logf("CanvasRTTo=%.0fns QuickTemplate=%.0fns ratio=%.2fx", canvasNs, qtNs, ratio)
	if ratio < 1.05 {
		t.Fatalf("dynamic Canvas RenderTo must beat quicktemplate; got %.2fx (%.0f vs %.0f ns)", ratio, qtNs, canvasNs)
	}
}

func TestGateCanvasBeatsLegacyDataGet(t *testing.T) {
	dir := t.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := canvas.New(dir)
	data := gateBenchData(50)
	w0 := rt.AcquireWriter()
	if err := eng.RenderTo(w0, "page", data); err != nil {
		t.Fatal(err)
	}
	rt.ReleaseWriter(w0)

	funcMap := template.FuncMap{
		"dataGet": func(data any, path string) any {
			parts := strings.Split(path, ".")
			cur := data
			for _, p := range parts {
				mm, ok := cur.(map[string]any)
				if !ok {
					return nil
				}
				cur = mm[p]
			}
			return cur
		},
	}
	tmpl := template.Must(template.New("page").Funcs(funcMap).Parse(
		`<h1>{{ dataGet . "title" }}</h1>{{ range $i, $item := dataGet . "items" }}<li>{{ dataGet $item "name" }}</li>{{ end }}`,
	))

	canvasNs := medianNs(t, 5, func() {
		w := rt.AcquireWriter()
		_ = eng.RenderTo(w, "page", data)
		rt.ReleaseWriter(w)
	})
	legacyNs := medianNs(t, 5, func() {
		var buf strings.Builder
		_ = tmpl.Execute(&buf, data)
	})
	ratio := legacyNs / canvasNs
	t.Logf("CanvasRTTo=%.0fns legacyDataGet=%.0fns ratio=%.1fx", canvasNs, legacyNs, ratio)
	if ratio < 20 {
		t.Fatalf("Canvas must be ≥20× faster than html/template+dataGet; got %.1fx", ratio)
	}
}

func TestGateRenderToZeroAllocs(t *testing.T) {
	dir := t.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := canvas.New(dir)
	data := gateBenchData(50)
	w := rt.AcquireWriter()
	if err := eng.RenderTo(w, "page", data); err != nil {
		t.Fatal(err)
	}
	rt.ReleaseWriter(w)

	allocs := testing.AllocsPerRun(100, func() {
		w := rt.AcquireWriter()
		_ = eng.RenderTo(w, "page", data)
		rt.ReleaseWriter(w)
	})
	t.Logf("RenderTo allocs/op=%.2f", allocs)
	if allocs > 0.5 {
		t.Fatalf("RenderTo must be ~0 allocs/op; got %.2f", allocs)
	}
}

func gateBenchData(n int) map[string]any {
	items := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i)}
	}
	return map[string]any{"title": "Bench", "items": items, "_token": "tok"}
}

func medianNs(t *testing.T, rounds int, fn func()) float64 {
	t.Helper()
	samples := make([]float64, rounds)
	for i := 0; i < rounds; i++ {
		samples[i] = float64(benchOne(fn))
	}
	for i := 0; i < len(samples); i++ {
		for j := i + 1; j < len(samples); j++ {
			if samples[j] < samples[i] {
				samples[i], samples[j] = samples[j], samples[i]
			}
		}
	}
	return samples[len(samples)/2]
}

func benchOne(fn func()) int64 {
	const target = 50 * time.Millisecond
	for i := 0; i < 50; i++ {
		fn()
	}
	start := time.Now()
	n := 0
	for time.Since(start) < target {
		fn()
		n++
	}
	elapsed := time.Since(start)
	if n == 0 {
		return 0
	}
	return elapsed.Nanoseconds() / int64(n)
}
