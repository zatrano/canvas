package canvas_test

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/rt"
)

// renderPath names the three compilation pipelines.
type renderPath string

const (
	pathAOT    renderPath = "aot"
	pathHTML   renderPath = "html"   // regex compileView → html/template
	pathLayout renderPath = "layout" // extends/include/component → html/template
)

func writePathTemplate(t *testing.T, dir string, p renderPath, body string) string {
	t.Helper()
	switch p {
	case pathAOT:
		name := "aot_page"
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return name
	case pathHTML:
		// PreferHTMLPath on the engine forces html/template (see callers).
		name := "html_page"
		src := body
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return name
	case pathLayout:
		_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(
			`<html>@yield('content')</html>`,
		), 0o644)
		name := "layout_page"
		src := `@extends('layouts.app')
@section('content')
` + body + `
@endsection
`
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return name
	default:
		t.Fatalf("unknown path %s", p)
		return ""
	}
}

func assertAOT(t *testing.T, dir, name string, wantAOT bool) {
	t.Helper()
	eng := canvas.New(dir)
	eng.EnableCache(false)
	// Probe: compile via Render of empty-ish — check CanASTLower on resolved body.
	raw, err := os.ReadFile(filepath.Join(dir, name+".html"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ast.ParseSource(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	got := doc.CanASTLower()
	if got != wantAOT {
		// layout page still has @extends in file; after resolve it may lower — only check aot/html files.
		if name == "aot_page" || name == "html_page" {
			t.Fatalf("%s CanASTLower=%v want %v", name, got, wantAOT)
		}
	}
}

func TestRenderPath_Consistency(t *testing.T) {
	paths := []renderPath{pathAOT, pathHTML, pathLayout}

	t.Run("xss_escape", func(t *testing.T) {
		dir := t.TempDir()
		body := `{{ $x }}`
		outs := map[renderPath]string{}
		for _, p := range paths {
			name := writePathTemplate(t, dir, p, body)
			if p == pathAOT {
				assertAOT(t, dir, name, true)
			}
			eng := canvas.New(dir)
			if p == pathHTML {
				eng.PreferHTMLPath(true)
			}
			out, err := eng.Render(name, map[string]any{"x": `<script>"'&`})
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			outs[p] = out
		}
		// Strip layout wrapper for comparison of the escaped payload presence.
		for p, out := range outs {
			if !strings.Contains(out, `&lt;script&gt;`) {
				t.Fatalf("%s missing escaped script: %q", p, out)
			}
			if !strings.Contains(out, `&#34;`) && !strings.Contains(out, `&quot;`) {
				t.Fatalf("%s missing escaped quote: %q", p, out)
			}
			if !strings.Contains(out, `&#39;`) && !strings.Contains(out, `&#x27;`) {
				t.Fatalf("%s missing escaped apostrophe: %q", p, out)
			}
			if !strings.Contains(out, `&amp;`) {
				t.Fatalf("%s missing escaped amp: %q", p, out)
			}
			if strings.Contains(out, `<script>`) {
				t.Fatalf("%s raw script: %q", p, out)
			}
		}
		// Core escaped fragment must match across paths (ignore layout chrome).
		core := func(s string) string {
			s = strings.TrimPrefix(s, "<html>")
			s = strings.TrimSuffix(s, "</html>")

			return strings.TrimSpace(s)
		}
		a, h, l := core(outs[pathAOT]), core(outs[pathHTML]), core(outs[pathLayout])
		if a != h || a != l {
			t.Fatalf("escape mismatch aot=%q html=%q layout=%q", a, h, l)
		}
	})

	t.Run("trusted_html", func(t *testing.T) {
		dir := t.TempDir()
		body := `{{ $x }}`
		for _, p := range paths {
			name := writePathTemplate(t, dir, p, body)
			eng := canvas.New(dir)
			if p == pathHTML {
				eng.PreferHTMLPath(true)
			}
			for _, val := range []any{rt.SafeHTML(`<b>ok</b>`), template.HTML(`<i>ok</i>`)} {
				out, err := eng.Render(name, map[string]any{"x": val})
				if err != nil {
					t.Fatalf("%s: %v", p, err)
				}
				if _, ok := val.(rt.SafeHTML); ok && !strings.Contains(out, `<b>ok</b>`) {
					t.Fatalf("%s SafeHTML not raw: %q", p, out)
				}
				if _, ok := val.(template.HTML); ok && !strings.Contains(out, `<i>ok</i>`) {
					t.Fatalf("%s template.HTML not raw: %q", p, out)
				}
			}
		}
	})

	t.Run("go_string_slot", func(t *testing.T) {
		dir := t.TempDir()
		body := `{{ $slot }}`
		for _, p := range paths {
			name := writePathTemplate(t, dir, p, body)
			eng := canvas.New(dir)
			if p == pathHTML {
				eng.PreferHTMLPath(true)
			}
			out, err := eng.Render(name, map[string]any{"slot": `<script>x</script>`})
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if strings.Contains(out, `<script>x</script>`) {
				t.Fatalf("%s Go string slot not escaped: %q", p, out)
			}
			out2, err := eng.Render(name, map[string]any{"slot": rt.SafeHTML(`<em>y</em>`)})
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if !strings.Contains(out2, `<em>y</em>`) {
				t.Fatalf("%s SafeHTML slot not raw: %q", p, out2)
			}
		}
	})

	t.Run("json_script_safe", func(t *testing.T) {
		dir := t.TempDir()
		body := `<script>var a=@json($x);</script>`
		outs := map[renderPath]string{}
		for _, p := range paths {
			name := writePathTemplate(t, dir, p, body)
			eng := canvas.New(dir)
			if p == pathHTML {
				eng.PreferHTMLPath(true)
			}
			out, err := eng.Render(name, map[string]any{"x": "</script>\u2028"})
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if strings.Contains(out, `"</script>`) {
				t.Fatalf("%s raw script break in json: %q", p, out)
			}
			if strings.Contains(out, "\u2028") {
				t.Fatalf("%s raw U+2028: %q", p, out)
			}
			if !strings.Contains(out, `\u003c`) {
				t.Fatalf("%s missing \\u003c: %q", p, out)
			}
			if !strings.Contains(out, `\u2028`) {
				t.Fatalf("%s missing \\u2028: %q", p, out)
			}
			outs[p] = out
		}
		// JSON payload fragment should agree (strip layout tags).
		extract := func(s string) string {
			i := strings.Index(s, "var a=")
			j := strings.LastIndex(s, ";</script>")
			if i < 0 || j < 0 {
				return s
			}
			return s[i:j]
		}
		a, h, l := extract(outs[pathAOT]), extract(outs[pathHTML]), extract(outs[pathLayout])
		if a != h || a != l {
			t.Fatalf("json mismatch aot=%q html=%q layout=%q", a, h, l)
		}
	})
}
