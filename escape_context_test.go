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

type escPath string

const (
	escAOT    escPath = "aot"
	escHTML   escPath = "html"
	escLayout escPath = "layout"
)

func newEscEngine(dir string, p escPath) *canvas.Engine {
	eng := canvas.New(dir)
	if p == escHTML {
		eng.PreferHTMLPath(true)
	}
	return eng
}

func escWrite(t *testing.T, dir string, p escPath, body string) string {
	t.Helper()
	switch p {
	case escAOT:
		name := "p_aot"
		_ = os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0o644)
		return name
	case escHTML:
		name := "p_html"
		_ = os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0o644)
		return name
	case escLayout:
		_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`@yield('content')`), 0o644)
		name := "p_layout"
		src := "@extends('layouts.app')\n@section('content')\n" + body + "\n@endsection\n"
		_ = os.WriteFile(filepath.Join(dir, name+".html"), []byte(src), 0o644)
		return name
	}
	t.Fatalf("bad path %s", p)
	return ""
}

func TestEscapeContext_Matrix(t *testing.T) {
	type tc struct {
		name    string
		src     string
		data    map[string]any
		wantSub []string // must contain
		forbid  []string // must not contain
		wantErr bool
	}
	cases := []tc{
		{
			name:    "body_script",
			src:     `<p>{{ $x }}</p>`,
			data:    map[string]any{"x": `<script>alert(1)</script>`},
			wantSub: []string{`&lt;script&gt;`},
			forbid:  []string{`<script>alert`},
		},
		{
			name:    "quoted_attr",
			src:     `<a title="{{ $x }}">`,
			data:    map[string]any{"x": `" onmouseover="alert(1)`},
			wantSub: []string{`&#34;`},
			forbid:  []string{`onmouseover="alert`},
		},
		{
			name:    "unquoted_attr",
			src:     `<a title={{ $x }}>`,
			data:    map[string]any{"x": `x onmouseover=alert(1)`},
			wantSub: []string{`&#32;`},
			forbid:  []string{`title=x onmouseover`},
		},
		{
			name:    "url_javascript",
			src:     `<a href="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:alert(1)`},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`javascript:alert`},
		},
		{
			name:    "url_javascript_ws",
			src:     `<a href="{{ $x }}">`,
			data:    map[string]any{"x": " java\nscript:alert(1)"},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`script:alert`},
		},
		{
			name:    "url_data",
			src:     `<a href="{{ $x }}">`,
			data:    map[string]any{"x": `data:text/html,<script>alert(1)</script>`},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`data:text/html`},
		},
		{
			name:    "url_vbscript",
			src:     `<img src="{{ $x }}">`,
			data:    map[string]any{"x": `vbscript:msgbox(1)`},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`vbscript:`},
		},
		{
			name:    "form_action",
			src:     `<form action="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:alert(1)`},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`javascript:`},
		},
		{
			name:    "onclick_forbid",
			src:     `<a onclick="{{ $x }}">`,
			data:    map[string]any{"x": `alert(1)`},
			wantErr: true,
		},
		{
			name:    "script_forbid",
			src:     `<script>var a = "{{ $x }}";</script>`,
			data:    map[string]any{"x": `";alert(1);//`},
			wantErr: true,
		},
		{
			name:    "style_forbid",
			src:     `<style>color: {{ $x }}</style>`,
			data:    map[string]any{"x": `red`},
			wantErr: true,
		},
		{
			name:    "json_script",
			src:     `<script>var a = @json($x);</script>`,
			data:    map[string]any{"x": `</script><script>alert(1)</script>`},
			wantSub: []string{`\u003c`},
			forbid:  []string{`"</script>`},
		},
		{
			name:    "json_attr_dq",
			src:     `<div data-x="@json($x)">`,
			data:    map[string]any{"x": `a"b`},
			wantSub: []string{`&#34;`},
		},
		{
			name:    "json_attr_sq",
			src:     `<div data-x='@json($x)'>`,
			data:    map[string]any{"x": `a"b`},
			wantSub: []string{`&#34;`},
		},
		{
			name:    "url_static_prefix",
			src:     `<a href="/p/{{ $id }}">`,
			data:    map[string]any{"id": `javascript:alert(1)`},
			wantSub: []string{`/p/`},
			forbid:  []string{`#unsafe`},
		},
		{
			name:    "js_in_onclick",
			src:     `<a onclick="@js($x)">`,
			data:    map[string]any{"x": `alert(1)`},
			wantErr: true, // on* forbids {{ }}, @json, and @js alike
		},
	}

	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				name := escWrite(t, dir, p, tc.src)
				eng := newEscEngine(dir, p)
				out, err := eng.Render(name, tc.data)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected compile/render error, got %q", out)
					}
					return
				}
				if err != nil {
					t.Fatalf("err=%v", err)
				}
				for _, w := range tc.wantSub {
					if !strings.Contains(out, w) {
						t.Fatalf("want substring %q in %q", w, out)
					}
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

func TestEscapeContext_GoldenSafeTemplatesUnchanged(t *testing.T) {
	// Correct templates: body + quoted attr with safe text — Strict must match Legacy.
	dir := t.TempDir()
	src := `<h1>{{ $title }}</h1><a title="{{ $t }}">x</a>`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	data := map[string]any{"title": "Hello", "t": "ok"}

	engS := canvas.New(dir)
	engS.SetEscapeMode(rt.EscapeStrict)
	outS, err := engS.Render("page", data)
	if err != nil {
		t.Fatal(err)
	}
	engL := canvas.New(dir)
	engL.SetEscapeMode(rt.EscapeLegacy)
	outL, err := engL.Render("page", data)
	if err != nil {
		t.Fatal(err)
	}
	if outS != outL {
		t.Fatalf("Strict changed safe template output:\nStrict=%q\nLegacy=%q", outS, outL)
	}
	if outS != `<h1>Hello</h1><a title="ok">x</a>` {
		t.Fatalf("unexpected golden %q", outS)
	}
}

func BenchmarkEscapeStrictVsLegacyListPage(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644)
	items := make([]map[string]any, 50)
	for i := 0; i < 50; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{"title": "Bench", "items": items}

	run := func(mode rt.EscapeMode) func(*testing.B) {
		return func(b *testing.B) {
			eng := canvas.New(dir)
			eng.SetEscapeMode(mode)
			_, _ = eng.Render("page", data) // warm
			w := rt.AcquireWriter()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.Reset()
				if err := eng.RenderTo(w, "page", data); err != nil {
					b.Fatal(err)
				}
			}
			rt.ReleaseWriter(w)
		}
	}
	b.Run("Strict", run(rt.EscapeStrict))
	b.Run("Legacy", run(rt.EscapeLegacy))
}
