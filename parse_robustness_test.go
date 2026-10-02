package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/lex"
	"github.com/zatrano/canvas/rt"
)

// Directive parse robustness. Expect: no panic; CSS @media stays text or harmless.

func TestParseRobust_CSSAtRulesNotDirectives(t *testing.T) {
	inputs := []string{
		`<style>@media (min-width:1px){body{color:red}}</style>`,
		`<style>@font-face{font-family:X;src:url(x)}</style>`,
		`<style>@keyframes spin{from{transform:rotate(0)}}</style>`,
		`Contact user@example.com please`,
		`<a href="mailto:user@example.com">mail</a>`,
		`@@if(true) should be escaped literal`,
		`<script>var s = "@foreach";</script>`,
		`<!-- @foreach($x as $y) -->`,
	}
	dir := t.TempDir()
	for i, src := range inputs {
		name := fmt.Sprintf("case%d", i)
		fname := filepath.Join(dir, name+".html")
		if err := os.WriteFile(fname, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		eng := canvas.New(dir)
		out, err := eng.Render(name, map[string]any{})
		t.Logf("INPUT=%q ERR=%v OUT=%q", src, err, out)
	}
}

func TestParseRobust_UnbalancedDirectives(t *testing.T) {
	cases := []string{
		`@if($x)<p>hi`,
		`@foreach($items as $item)<li>`,
		`@endif`,
		`@if(`,
	}
	dir := t.TempDir()
	for i, src := range cases {
		name := fmt.Sprintf("u%d", i)
		fname := filepath.Join(dir, name+".html")
		_ = os.WriteFile(fname, []byte(src), 0o644)
		eng := canvas.New(dir)
		out, err := eng.Render(name, map[string]any{
			"x": true, "items": []map[string]any{{"a": 1}},
		})
		t.Logf("SRC=%q ERR=%v OUT_LEN=%d", src, err, len(out))
	}
}

func TestParseRobust_LargeNestedNoHang(t *testing.T) {
	var b strings.Builder
	const depth = 50 // 500 may OOM/hang — start 50; separate attempt for deeper
	for i := 0; i < depth; i++ {
		b.WriteString("@if($x)")
	}
	b.WriteString("ok")
	for i := 0; i < depth; i++ {
		b.WriteString("@endif")
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "deep.html"), []byte(b.String()), 0o644)
	eng := canvas.New(dir)
	out, err := eng.Render("deep", map[string]any{"x": true})
	t.Logf("DEEP50 ERR=%v OUT=%q", err, out)
}

func TestParseRobust_NULAndInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	src := "hello\x00world{{ $x }}\xff\xfe"
	_ = os.WriteFile(filepath.Join(dir, "nul.html"), []byte(src), 0o644)
	eng := canvas.New(dir)
	out, err := eng.Render("nul", map[string]any{"x": "a"})
	t.Logf("NUL ERR=%v OUT=%q", err, out)
}

func FuzzParse_Lex(f *testing.F) {
	f.Add([]byte(`<h1>{{ $title }}</h1>`))
	f.Add([]byte(`@foreach($a as $b)x@endforeach`))
	f.Add([]byte(`@media (min-width:1px){}`))
	f.Add([]byte("\x00\xff{{"))
	f.Fuzz(func(t *testing.T, src []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("lex panic: %v on %q", r, src)
			}
		}()
		_, _ = lex.Lex(string(src))
	})
}

func FuzzParse_AST(f *testing.F) {
	f.Add([]byte(`{{ $x }}`))
	f.Add([]byte(`@if($x)a@else b@endif`))
	f.Add([]byte(`@foreach($a as $b){{ $b }}@endforeach`))
	f.Fuzz(func(t *testing.T, src []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ast panic: %v on %q", r, src)
			}
		}()
		_, _ = ast.ParseSource(string(src))
	})
}

func FuzzParse_Render(f *testing.F) {
	f.Add([]byte(`{{ $x }}`))
	f.Add([]byte(`@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`))
	f.Add([]byte(`{!! $x !!}`))
	f.Fuzz(func(t *testing.T, src []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("render panic: %v on %q", r, src)
			}
		}()
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "page.html"), src, 0o644)
		eng := canvas.New(dir)
		eng.EnableCache(false)
		_, _ = eng.Render("page", map[string]any{
			"x":     "v",
			"items": []map[string]any{{"name": "n"}},
		})
		doc, err := ast.ParseSource(string(src))
		if err != nil || doc == nil {
			return
		}
		if c, ok, _ := rt.CompileFunc(doc, "local"); ok && c != nil {
			w := rt.AcquireWriter()
			_ = c.Execute(w, map[string]any{"x": "v", "items": []map[string]any{{"name": "n"}}})
			rt.ReleaseWriter(w)
		}
	})
}
