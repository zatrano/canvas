package canvas_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

// TestEscape_GoldenDocsAndLayouts: Strict ≡ Legacy for safe docs/layout examples.
// Truly unsafe patterns may diverge (listed when they do).
func TestEscape_GoldenDocsAndLayouts(t *testing.T) {
	type file struct {
		rel  string
		body string
	}
	cases := []struct {
		name  string
		files []file
		view  string
		data  map[string]any
		// unsafeDiff: if true, Strict may differ / error vs Legacy (document why)
		unsafeDiff bool
		diffReason string
	}{
		{
			name: "readme_hello",
			files: []file{{"hello.html", `<h1>{{ $title }}</h1>
@foreach($items as $item)
<li>{{ $item.name }}</li>
@endforeach
`}},
			view: "hello",
			data: map[string]any{
				"title": "Hello",
				"items": []map[string]any{{"name": "a"}, {"name": "b"}},
			},
		},
		{
			name: "layout_extends_yield_include",
			files: []file{
				{"layouts/app.html", `<html><title>@yield('title', 'Default')</title><body>@include('partials.brand')@yield('content')</body></html>`},
				{"partials/brand.html", `<div>ZATRANO</div>`},
				{"home.html", `@extends('layouts.app')
@section('title', 'Home')
@section('content')
<p>{{ $message }}</p>
@endsection
`},
			},
			view: "home",
			data: map[string]any{"message": "Hello"},
		},
		{
			name: "component_slot",
			files: []file{
				{"components/box.html", `<div class="box">{{ $slot }}</div>`},
				{"page.html", `@component('box')safe text@endcomponent`},
			},
			view: "page",
			data: nil,
		},
		{
			name: "quoted_attr_safe",
			files: []file{
				{"page.html", `<a title="{{ $t }}" href="/ok">x</a>`},
			},
			view: "page",
			data: map[string]any{"t": "hello"},
		},
		{
			name:       "url_javascript_differs",
			files:      []file{{"page.html", `<a href="{{ $u }}">x</a>`}},
			view:       "page",
			data:       map[string]any{"u": "javascript:alert(1)"},
			unsafeDiff: true,
			diffReason: "Strict URL filter → #unsafe; Legacy HTML-escapes only",
		},
		{
			name:       "onclick_forbid_differs",
			files:      []file{{"page.html", `<a onclick="{{ $x }}">x</a>`}},
			view:       "page",
			data:       map[string]any{"x": "alert(1)"},
			unsafeDiff: true,
			diffReason: "Strict compile error; Legacy HTML-escapes",
		},
	}

	var differed []string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				p := filepath.Join(dir, filepath.FromSlash(f.rel))
				_ = os.MkdirAll(filepath.Dir(p), 0o755)
				if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			engS := canvas.New(dir)
			engS.SetEscapeMode(rt.EscapeStrict)
			outS, errS := engS.Render(tc.view, tc.data)

			engL := canvas.New(dir)
			engL.SetEscapeMode(rt.EscapeLegacy)
			outL, errL := engL.Render(tc.view, tc.data)

			if tc.unsafeDiff {
				if errS == nil && errL == nil && outS == outL {
					t.Fatalf("expected Strict≠Legacy for unsafe case (%s)", tc.diffReason)
				}
				differed = append(differed, tc.name+": "+tc.diffReason)
				return
			}
			if errS != nil {
				t.Fatalf("Strict: %v", errS)
			}
			if errL != nil {
				t.Fatalf("Legacy: %v", errL)
			}
			if outS != outL {
				t.Fatalf("Strict≡Legacy failed:\nStrict=%q\nLegacy=%q", outS, outL)
			}
		})
	}
	t.Logf("intentional Strict≠Legacy cases: %s", strings.Join(differed, "; "))
}
