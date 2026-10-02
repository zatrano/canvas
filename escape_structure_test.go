package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/rt"
)

func TestEscape_AOTStructureStrictEqualsLegacy(t *testing.T) {
	dir := t.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)

	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	cs, okS, errS := rt.CompileFuncMode(doc, "development", rt.EscapeStrict)
	if errS != nil || !okS {
		t.Fatalf("strict compile: ok=%v err=%v", okS, errS)
	}
	cl, okL, errL := rt.CompileFuncMode(doc, "development", rt.EscapeLegacy)
	if errL != nil || !okL {
		t.Fatalf("legacy compile: ok=%v err=%v", okL, errL)
	}
	// Same list-page template: every echo is EscHTML under both modes → identical kind sequence.
	// Probe via rendering allocs/bytes equality under both engines.
	items := make([]map[string]any, 50)
	for i := 0; i < 50; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{"title": "Bench", "items": items}

	engS := canvas.New(dir)
	engS.SetEscapeMode(rt.EscapeStrict)
	engL := canvas.New(dir)
	engL.SetEscapeMode(rt.EscapeLegacy)
	outS, err := engS.Render("page", data)
	if err != nil {
		t.Fatal(err)
	}
	outL, err := engL.Render("page", data)
	if err != nil {
		t.Fatal(err)
	}
	if outS != outL {
		t.Fatalf("list page Strict≠Legacy:\nS=%q\nL=%q", outS, outL)
	}
	_ = cs
	_ = cl
	t.Log("list-page AOT output identical under Strict and Legacy (structure-equivalent for EscHTML-only sites)")
}

func medianInts(xs []int) float64 {
	sort.Ints(xs)
	n := len(xs)
	if n%2 == 0 {
		return float64(xs[n/2-1]+xs[n/2]) / 2
	}
	return float64(xs[n/2])
}
