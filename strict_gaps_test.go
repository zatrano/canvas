package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas/rt"
)

// TestStrictGaps_Regression documents and locks the escape holes that existed
// before oracle harden and were closed afterwards.
//
// | ctx | payload | pre-fix out | why exploitable |
// |-----|---------|--------------|-----------------|
// | unquoted URL after /ok/ lock | `x onmouseover=alert(1)` | `href=/ok/x onmouseover=alert(1)` | EscHTML after URL lock skipped space escape → new on* attr |
// | unquoted @json | `x onmouseover=alert(1)` | `title="… onmouseover=…"` (entities + spaces) | JSON spaces split unquoted attrs |
// | style @json | `red;…url(javascript:x)` | style contains url(javascript | @json bypassed style forbidAttrs check |
//
// Gap table: docs/strict-gaps.md.
func TestStrictGaps_Regression(t *testing.T) {
	type tc struct {
		name    string
		src     string
		data    map[string]any
		wantErr bool
		wantSub []string
		forbid  []string
	}
	cases := []tc{
		{
			name:    "unquoted_url_after_static_no_breakout",
			src:     `<div href=/ok/{{ $x }}>`,
			data:    map[string]any{"x": `x onmouseover=alert(1)`},
			wantSub: []string{`&#32;`},
			forbid:  []string{` onmouseover=alert`},
		},
		{
			name:    "unquoted_url_start_scheme_and_spaces",
			src:     `<a href={{ $x }}>`,
			data:    map[string]any{"x": `javascript:alert(1)`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "unquoted_json_forbid",
			src:     `<div title=@json($x)>`,
			data:    map[string]any{"x": `x onmouseover=alert(1)`},
			wantErr: true,
		},
		{
			name:    "style_json_forbid",
			src:     `<div style="@json($x)">`,
			data:    map[string]any{"x": `red;background:url(javascript:x)`},
			wantErr: true,
		},
		{
			name:    "unquoted_js_forbid",
			src:     `<div id=@js($x)>`,
			data:    map[string]any{"x": `1`},
			wantErr: true,
		},
	}
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				name := escWrite(t, dir, p, tc.src)
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(rt.EscapeStrict)
				out, err := eng.Render(name, tc.data)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected error, out=%q", out)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, w := range tc.wantSub {
					if strings.Contains(out, w) {
						continue
					}
					// html/template may re-escape entities (&#32; → &amp;#32;).
					if w == `&#32;` && strings.Contains(out, `&amp;#32;`) {
						continue
					}
					t.Fatalf("want %q in %q", w, out)
				}
				for _, f := range tc.forbid {
					if strings.Contains(out, f) {
						t.Fatalf("forbid %q in %q", f, out)
					}
				}
			})
		}
	}
}

// TestStrict_ExpectedCompileErrors asserts fail-closed contexts.
// Note: HTML-escaping inside an unquoted JS string does NOT prevent XSS for
// payloads like x=alert(1) if the value were ever allowed; that is why {{ }} in
// <script> must remain a compile error (see script_echo_* rows).
func TestStrict_ExpectedCompileErrors(t *testing.T) {
	type row struct {
		name      string
		src       string
		ctxNeedle string // must appear in error (context name)
		allowJS   bool   // if true, @js must succeed
		allowJSON bool   // if true, @json must succeed
		setup     func(dir string)
	}
	rows := []row{
		{name: "script_echo_bare", src: `<script>var a = {{ $x }};</script>`, ctxNeedle: "script"},
		{name: "script_echo_string", src: `<script>var a = "{{ $x }}";</script>`, ctxNeedle: "script"},
		{name: "script_js_allowed", src: `<script>var a = @js($x);</script>`, allowJS: true},
		{name: "script_json_allowed", src: `<script>var a = @json($x);</script>`, allowJSON: true},
		{name: "style_el_echo", src: `<style>{{ $x }}</style>`, ctxNeedle: "style"},
		{name: "style_el_json", src: `<style>@json($x)</style>`, ctxNeedle: "style"},
		{name: "style_el_js", src: `<style>@js($x)</style>`, ctxNeedle: "style"},
		{name: "onclick", src: `<a onclick="{{ $x }}">`, ctxNeedle: "on"},
		{name: "onclick_json", src: `<a onclick="@json($x)">`, ctxNeedle: "on"},
		{name: "onclick_js", src: `<a onclick="@js($x)">`, ctxNeedle: "on"},
		{name: "style_attr_whole", src: `<div style="{{ $x }}">`, ctxNeedle: "style"},
		{name: "style_attr_json", src: `<div style="@json($x)">`, ctxNeedle: "style"},
		{name: "style_attr_js", src: `<div style="@js($x)">`, ctxNeedle: "style"},
		{name: "srcdoc", src: `<iframe srcdoc="{{ $x }}">`, ctxNeedle: "srcdoc"},
		{name: "srcdoc_json", src: `<iframe srcdoc="@json($x)">`, ctxNeedle: "srcdoc"},
		{name: "srcdoc_js", src: `<iframe srcdoc="@js($x)">`, ctxNeedle: "srcdoc"},
		{name: "tag_name", src: `<{{ $t }}>`, ctxNeedle: "tag"},
		{name: "attr_name", src: `<div {{ $attrs }}>`, ctxNeedle: "attribute"},
		{name: "between_attrs", src: `<a href="x" {{ $y }}>`, ctxNeedle: "attribute"},
		{
			name: "include_in_script", src: `<script>@include('partials.x')</script>`, ctxNeedle: "script",
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`1`), 0o644)
			},
		},
		{
			name: "include_in_style", src: `<style>@include('partials.x')</style>`, ctxNeedle: "style",
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`1`), 0o644)
			},
		},
		{
			name: "include_in_tag", src: `<@include('partials.x')>`, ctxNeedle: "tag",
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`div`), 0o644)
			},
		},
		{
			name: "component_in_script", src: `<script>@component('partials.x')</script>`, ctxNeedle: "script",
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`1`), 0o644)
			},
		},
		{
			name: "extends_in_script", src: `<script>@extends('layouts.app')</script>`, ctxNeedle: "script",
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`@yield('content')`), 0o644)
			},
		},
	}
	paths := []escPath{escAOT, escHTML, escLayout}
	data := map[string]any{"x": `alert(1)`, "t": `script`, "attrs": `onclick=x`, "y": `onclick=x`}
	for _, p := range paths {
		for _, r := range rows {
			t.Run(fmt.Sprintf("%s/%s", p, r.name), func(t *testing.T) {
				dir := t.TempDir()
				if r.setup != nil {
					r.setup(dir)
				}
				name := escWrite(t, dir, p, r.src)
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(rt.EscapeStrict)
				out, err := eng.Render(name, data)
				if r.allowJS || r.allowJSON {
					if err != nil {
						t.Fatalf("expected allow, err=%v", err)
					}
					return
				}
				if err == nil {
					t.Fatalf("expected compile error, out=%q", out)
				}
				msg := strings.ToLower(err.Error())
				if !strings.Contains(msg, strings.ToLower(r.ctxNeedle)) {
					t.Fatalf("error missing context %q: %v", r.ctxNeedle, err)
				}
			})
		}
	}
}
