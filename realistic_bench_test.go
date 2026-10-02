package canvas_test

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

// Realistic workloads. Expectations before run:
//   - Layout+component path slower than toy list; may allocate
//   - Nested foreach allocates or is slower
//   - Struct data uses reflect on dynamic path → more ns and possibly allocs
//   - RenderTo "0 alloc" may break on complex templates

func BenchmarkRealistic_LayoutComponent(b *testing.B) {
	dir := b.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(
		`<html><title>@yield('title')</title><body>@include('partials.nav')@yield('content')</body></html>`,
	), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "partials", "nav.html"), []byte(`<nav>{{ $brand }}</nav>`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "components", "card.html"), []byte(
		`<div class="card"><h2>{{ $title }}</h2>{{ $slot }}</div>`,
	), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`@extends('layouts.app')
@section('title')Dash@endsection
@section('content')
@component('card', ['title' => $heading])
<p>{{ $body }}</p>
@endcomponent
@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach
@endsection
`), 0o644)
	eng := canvas.New(dir)
	items := make([]map[string]any, 20)
	for i := 0; i < 20; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{
		"brand": "Z", "heading": "H", "body": "B", "items": items,
	}
	if _, err := eng.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		if err := eng.RenderTo(w, "page", data); err != nil {
			b.Fatal(err)
		}
		rt.ReleaseWriter(w)
	}
}

func BenchmarkRealistic_NestedForeach10x10(b *testing.B) {
	dir := b.TempDir()
	src := `@foreach($rows as $row)@foreach($row.cols as $col)<td>{{ $col.v }}</td>@endforeach@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	eng := canvas.New(dir)
	rows := make([]map[string]any, 10)
	for i := 0; i < 10; i++ {
		cols := make([]map[string]any, 10)
		for j := 0; j < 10; j++ {
			cols[j] = map[string]any{"v": fmt.Sprintf("%d-%d", i, j)}
		}
		rows[i] = map[string]any{"cols": cols}
	}
	data := map[string]any{"rows": rows}
	if _, err := eng.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		if err := eng.RenderTo(w, "page", data); err != nil {
			b.Fatal(err)
		}
		rt.ReleaseWriter(w)
	}
}

type realisticNested struct {
	Label string
}
type realisticItem struct {
	Name  string
	N     int
	Score float64
	OK    bool
	When  time.Time
	Meta  realisticNested
}
type realisticPage struct {
	Title string
	Items []realisticItem
}

func BenchmarkRealistic_CanvasStructData(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $Title }}</h1>@foreach($Items as $item)<li>{{ $item.Name }} {{ $item.N }} {{ $item.Score }} {{ $item.OK }} {{ $item.Meta.Label }}</li>@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	eng := canvas.New(dir)
	items := make([]realisticItem, 50)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for i := range items {
		items[i] = realisticItem{
			Name: fmt.Sprintf("Item-%d", i), N: i, Score: float64(i) * 0.1,
			OK: i%2 == 0, When: now, Meta: realisticNested{Label: "L"},
		}
	}
	// Engine data model is map[string]any — wrap struct fields as map values of typed slices/structs.
	data := map[string]any{"Title": "Bench", "Items": items}
	if _, err := eng.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		if err := eng.RenderTo(w, "page", data); err != nil {
			b.Fatal(err)
		}
		rt.ReleaseWriter(w)
	}
}

func BenchmarkRealistic_HTMLTemplateStruct(b *testing.B) {
	tmpl := template.Must(template.New("page").Parse(
		`<h1>{{.Title}}</h1>{{range .Items}}<li>{{.Name}} {{.N}} {{.Score}} {{.OK}} {{.Meta.Label}}</li>{{end}}`,
	))
	items := make([]realisticItem, 50)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for i := range items {
		items[i] = realisticItem{
			Name: fmt.Sprintf("Item-%d", i), N: i, Score: float64(i) * 0.1,
			OK: i%2 == 0, When: now, Meta: realisticNested{Label: "L"},
		}
	}
	data := realisticPage{Title: "Bench", Items: items}
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tmpl.Execute(&buf, data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRealistic_MixedMapTypes(b *testing.B) {
	dir := b.TempDir()
	src := `{{ $s }} {{ $n }} {{ $f }} {{ $ok }} @foreach($items as $item){{ $item }}@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	eng := canvas.New(dir)
	data := map[string]any{
		"s": "hi", "n": 42, "f": 3.14, "ok": true,
		"items": []any{"a", 1, 2.5, false},
	}
	if _, err := eng.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		if err := eng.RenderTo(w, "page", data); err != nil {
			b.Fatal(err)
		}
		rt.ReleaseWriter(w)
	}
}

func BenchmarkRealistic_CompileCold(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	data := map[string]any{
		"title": "T",
		"items": []map[string]any{{"name": "a"}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng := canvas.New(dir)
		eng.EnableCache(false)
		if _, err := eng.Render("page", data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRealistic_CompileCached(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	eng := canvas.New(dir)
	data := map[string]any{
		"title": "T",
		"items": []map[string]any{{"name": "a"}},
	}
	if _, err := eng.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := eng.Render("page", data); err != nil {
			b.Fatal(err)
		}
	}
}
