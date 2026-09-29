package rt_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/rt"
)

func TestCompileForeachEcho(t *testing.T) {
	doc, err := ast.ParseSource(`@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`)
	if err != nil {
		t.Fatal(err)
	}
	prog, ok, err := rt.CompileFunc(doc, "local")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	out, err := prog.Render(map[string]any{
		"items": []map[string]any{{"name": "A"}, {"name": "B"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<li>A</li>") || !strings.Contains(out, "<li>B</li>") {
		t.Fatalf("out=%q", out)
	}
}

func TestCompileIfElse(t *testing.T) {
	doc, err := ast.ParseSource(`@if($on)YES@else NO@endif`)
	if err != nil {
		t.Fatal(err)
	}
	prog, ok, err := rt.CompileFunc(doc, "local")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	out, err := prog.Render(map[string]any{"on": true})
	if err != nil || out != "YES" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	out, err = prog.Render(map[string]any{"on": false})
	if err != nil || out != "NO" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestEscape(t *testing.T) {
	doc, err := ast.ParseSource(`{{ $x }}`)
	if err != nil {
		t.Fatal(err)
	}
	prog, ok, err := rt.CompileFunc(doc, "local")
	if err != nil || !ok {
		t.Fatal(err)
	}
	out, err := prog.Render(map[string]any{"x": `<script>"&'`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<script>") || !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("out=%q", out)
	}
}
