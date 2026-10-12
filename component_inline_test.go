package canvas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComponentInlineSkipsMergeDict(t *testing.T) {
	dir := t.TempDir()
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

	eng := New(dir)
	resolved, err := eng.resolveView("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resolved, "mergeDict") || strings.Contains(resolved, "safeStr") {
		t.Fatalf("simple component still lowered to html/template:\n%s", resolved)
	}
	out, err := eng.Render("page", map[string]any{
		"brand":   "Z",
		"heading": "H",
		"body":    "B",
		"items": []map[string]any{
			{"name": "Item-0"},
			{"name": "Item-1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<nav>Z</nav>", "<h2>H</h2>", "<p>B</p>", "<li>Item-0</li>", "<li>Item-1</li>", "<title>Dash</title>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestComponentInlineEscapesUnsafeLiteral(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "components", "box.html"), []byte(`<div class="box {{ $type }}">{{ $slot }}</div>`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(
		"@component('box', ['type' => '<b>'])x@endcomponent\n@component('box', ['type' => 'a\"b'])y@endcomponent",
	), 0o644)
	eng := New(dir)
	resolved, err := eng.resolveView("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resolved, "mergeDict") || strings.Contains(resolved, "mergeDefaults") || strings.Contains(resolved, "safeStr") {
		t.Fatalf("literal prop still lowered to html/template:\n%s", resolved)
	}
	out, err := eng.Render("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `<b>`) {
		t.Fatalf("literal prop must stay escaped, got %q", out)
	}
	for _, want := range []string{`box &lt;b&gt;`, `box a&#34;b`, ">x<", ">y<"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestComponentInlineAttributesPropsAware(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "components", "box.html"), []byte(`<div {{ $attributes }} class="box">{{ $slot }}</div>`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "components", "alert.html"), []byte(`
@props(['type' => 'info'])
<div class="alert-{{ $type }}">{{ $slot }}</div>
`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`
@component('box')Hi@endcomponent
@component('alert')Def@endcomponent
@component('alert', ['type' => '<b>'])Over@endcomponent
`), 0o644)
	eng := New(dir)
	resolved, err := eng.resolveView("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resolved, "mergeDict") || strings.Contains(resolved, "mergeDefaults") || strings.Contains(resolved, "$attributes") || strings.Contains(resolved, "@props") {
		t.Fatalf("props/attributes still lowered:\n%s", resolved)
	}
	out, err := eng.Render("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Hi", "alert-info", "alert-&lt;b&gt;", "Def", "Over"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestXTagInlineAttributesAware(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "components", "alert.html"), []byte(`
@props(['type' => 'info'])
<div {{ $attributes }} class="alert-{{ $type }}">{{ $slot }}</div>
`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "components", "badge.html"), []byte(`
@aware(['type'])
<span class="badge-{{ $type }}">{{ $slot }}</span>
`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "components", "panel.html"), []byte(`
@props(['type' => 'info'])
<div class="panel">
<x-badge>inner</x-badge>
</div>
`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`
<x-alert type="danger" class="flash" id="a1">Hello</x-alert>
<x-panel type="success" />
`), 0o644)
	eng := New(dir)
	resolved, err := eng.resolveView("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resolved, "mergeDict") || strings.Contains(resolved, "mergeDefaults") || strings.Contains(resolved, "$attributes") || strings.Contains(resolved, "@aware") || strings.Contains(resolved, "@props") {
		t.Fatalf("x-tag still lowered to html/template:\n%s", resolved)
	}
	out, err := eng.Render("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="flash"`, `id="a1"`, "alert-danger", "Hello", "badge-success"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
