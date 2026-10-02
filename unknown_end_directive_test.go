package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

func TestUnknownClosingDirective_Strict(t *testing.T) {
	type tc struct {
		name    string
		src     string
		wantErr bool
		needle  string // substring of error (directive name)
	}
	cases := []tc{
		{name: "endfoo", src: `@endfoo`, wantErr: true, needle: "@endfoo"},
		{name: "endwidget", src: `@endwidget`, wantErr: true, needle: "@endwidget"},
		{name: "mismatched_pair", src: `@foo(x)
body
@endfoo`, wantErr: true, needle: "@endfoo"},
		// Negatives — must not error in Strict.
		{name: "email", src: `Contact user@endpoint.com please`, wantErr: false},
		{name: "escaped_at", src: `@@endfoo`, wantErr: false},
		{name: "verbatim", src: `@verbatim
@endfoo
@endverbatim`, wantErr: false},
		{name: "comment", src: `{{-- @endfoo --}}`, wantErr: false},
		{name: "css_media", src: `<style>@media screen { .x{color:red} }</style>`, wantErr: false},
		{name: "known_endif", src: `@if(true)ok@endif`, wantErr: false},
		{name: "known_endforeach", src: `@foreach($items as $i){{ $i }}@endforeach`, wantErr: false},
		{name: "known_endsection_layout", src: `@extends('layouts.app')
@section('content')
x
@endsection`, wantErr: false},
	}
	paths := []escPath{escAOT, escHTML, escLayout}
	data := map[string]any{"items": []any{"a"}}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				if strings.Contains(tc.src, "layouts.app") || p == escLayout {
					_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
					_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`@yield('content')`), 0o644)
				}
				var name string
				if strings.Contains(tc.src, "@extends") {
					_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(tc.src), 0o644)
					name = "p"
					eng := newEscEngine(dir, p)
					eng.SetEscapeMode(rt.EscapeStrict)
					_, err := eng.Render(name, data)
					if tc.wantErr {
						if err == nil {
							t.Fatal("expected compile error")
						}
						if !strings.Contains(err.Error(), tc.needle) {
							t.Fatalf("error missing %q: %v", tc.needle, err)
						}
						if !strings.Contains(err.Error(), "unknown closing directive") {
							t.Fatalf("error missing phrase: %v", err)
						}
						if !strings.Contains(err.Error(), "docs/directives.md") {
							t.Fatalf("error missing docs link: %v", err)
						}
						if !strings.Contains(err.Error(), "canvas template [") {
							t.Fatalf("error missing template name: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				name = escWrite(t, dir, p, tc.src)
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(rt.EscapeStrict)
				out, err := eng.Render(name, data)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected compile error, out=%q", out)
					}
					msg := err.Error()
					if !strings.Contains(msg, tc.needle) {
						t.Fatalf("error missing %q: %v", tc.needle, err)
					}
					if !strings.Contains(msg, "unknown closing directive") {
						t.Fatalf("error missing phrase: %v", err)
					}
					if !strings.Contains(msg, "docs/directives.md") {
						t.Fatalf("error missing docs link: %v", err)
					}
					if !strings.Contains(msg, "canvas template [") {
						t.Fatalf("error missing template name: %v", err)
					}
					// line:column present (e.g. at 1:1)
					if !strings.Contains(msg, " at ") {
						t.Fatalf("error missing line:col: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected err: %v out=%q", err, out)
				}
			})
		}
	}
}

func TestUnknownClosingDirective_LegacyLiteral(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(`@endfoo`), 0o644)
	eng := canvas.New(dir)
	eng.SetEscapeMode(rt.EscapeLegacy)
	out, err := eng.Render("p", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "@endfoo") {
		t.Fatalf("Legacy must leave unknown closer as literal text, got %q", out)
	}
}
