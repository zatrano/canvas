package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/lex"
	"github.com/zatrano/canvas/rt"
)

// Escape-surface red→green cases for inventory in docs/escape-surface.md.

func TestEscapeSurface_SectionShortVarInTitle(t *testing.T) {
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p), func(t *testing.T) {
			dir := t.TempDir()
			_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
			_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`<title>@yield('t')</title>@yield('content')`), 0o644)
			body := "@extends('layouts.app')\n@section('t', $x)\n@section('content')\nok\n@endsection\n"
			var name string
			switch p {
			case escAOT:
				_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(body), 0o644)
				name = "p"
			case escHTML:
				_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(body), 0o644)
				name = "p"
			case escLayout:
				_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(body), 0o644)
				name = "p"
			}
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, map[string]any{"x": `</title><script>alert(1)</script>`})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `</title><script>`) {
				t.Fatalf("title breakout: %q", out)
			}
			if !strings.Contains(out, `&lt;/title&gt;`) {
				t.Fatalf("want escaped title content, got %q", out)
			}
		})
	}
}

func TestEscapeSurface_YieldDefaultLiteral(t *testing.T) {
	// @yield('x', 'default') uses a static string default.
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`<p>@yield('missing', 'safe-default')</p>`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte("@extends('layouts.app')\n"), 0o644)
	eng := canvas.New(dir)
	eng.SetEscapeMode(rt.EscapeStrict)
	out, err := eng.Render("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "safe-default") {
		t.Fatalf("got %q", out)
	}
}

func TestEscapeSurface_YieldDefaultVar(t *testing.T) {
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p), func(t *testing.T) {
			dir := t.TempDir()
			_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
			_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`<title>@yield('t', $x)</title>`), 0o644)
			body := "@extends('layouts.app')\n"
			var name string
			switch p {
			case escHTML:
				_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(body), 0o644)
				name = "p"
			default:
				_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(body), 0o644)
				name = "p"
			}
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, map[string]any{"x": `</title><script>x</script>`})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `</title><script>`) {
				t.Fatalf("yield default var breakout: %q", out)
			}
			if !strings.Contains(out, `&lt;`) {
				t.Fatalf("want escaped default, got %q", out)
			}
		})
	}
}

func TestEscapeSurface_LangCatalogTrusted(t *testing.T) {
	// Default: catalog HTML trusted in text; attr always escaped.
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p)+"/text_raw", func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<p>@lang('k')</p>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, map[string]any{
				"__trans": map[string]string{"k": `<b>ok</b>`},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, `<b>ok</b>`) {
				t.Fatalf("want raw catalog HTML in text, got %q", out)
			}
		})
		t.Run(string(p)+"/attr_escaped", func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<div title="@lang('k')"></div>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, map[string]any{
				"__trans": map[string]string{"k": `<b>ok</b>`},
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `title="<b>ok</b>"`) {
				t.Fatalf("catalog must be attr-escaped, got %q", out)
			}
			if !strings.Contains(out, `&lt;b&gt;`) {
				t.Fatalf("want attr-escaped catalog, got %q", out)
			}
		})
		t.Run(string(p)+"/script_forbid", func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<script>@lang('k')</script>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			_, err := eng.Render(name, map[string]any{
				"__trans": map[string]string{"k": `x`},
			})
			if err == nil {
				t.Fatal("want Strict compile error for @lang in script")
			}
		})
	}
}

func TestEscapeSurface_LangEscapeCatalogOptIn(t *testing.T) {
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p), func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<p>@lang('k')</p>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			eng.SetLangEscapeCatalog(true)
			out, err := eng.Render(name, map[string]any{
				"__trans": map[string]string{"k": `<b>ok</b>`},
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `<b>ok</b>`) {
				t.Fatalf("opt-in catalog escape: raw HTML leaked: %q", out)
			}
			if !strings.Contains(out, `&lt;b&gt;`) {
				t.Fatalf("want escaped catalog, got %q", out)
			}
		})
	}
}

func TestEscapeSurface_LangParamEscape(t *testing.T) {
	// Params always HTML-escaped before insertion (all modes), even when catalog is trusted.
	paths := []escPath{escAOT, escHTML, escLayout}
	modes := []struct {
		name string
		mode rt.EscapeMode
	}{
		{"strict", rt.EscapeStrict},
		{"legacy", rt.EscapeLegacy},
	}
	for _, p := range paths {
		for _, m := range modes {
			t.Run(string(p)+"/"+m.name+"/text", func(t *testing.T) {
				dir := t.TempDir()
				name := escWrite(t, dir, p, `<p>@lang('k', ['name' => $x])</p>`)
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(m.mode)
				out, err := eng.Render(name, map[string]any{
					"x":       `<script>alert(1)</script>`,
					"__trans": map[string]string{"k": `Hi <b>:name</b>`},
				})
				if err != nil {
					if p == escAOT {
						t.Logf("aot lang-with-repl may fall back: %v", err)
					} else {
						t.Fatal(err)
					}
				}
				if err != nil {
					return
				}
				if !strings.Contains(out, `<b>`) {
					t.Fatalf("catalog HTML should stay raw in text, got %q", out)
				}
				if strings.Contains(out, `<script>`) {
					t.Fatalf("param must be escaped, got %q", out)
				}
				if !strings.Contains(out, `&lt;script&gt;`) {
					t.Fatalf("want escaped param entity, got %q", out)
				}
			})
			t.Run(string(p)+"/"+m.name+"/attr", func(t *testing.T) {
				dir := t.TempDir()
				name := escWrite(t, dir, p, `<div title="@lang('k', ['name' => $x])"></div>`)
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(m.mode)
				out, err := eng.Render(name, map[string]any{
					"x":       `"><img onerror=x>`,
					"__trans": map[string]string{"k": `Hi :name`},
				})
				if err != nil {
					if p == escAOT {
						t.Logf("aot: %v", err)
						return
					}
					t.Fatal(err)
				}
				if strings.Contains(out, `<img`) {
					t.Fatalf("param/attr breakout: %q", out)
				}
			})
		}
	}
}

func TestEscapeSurface_LangEscapes(t *testing.T) {
	// Backward-compatible name: opt-in catalog escape still locked by mutation.
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p), func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<p>@lang('k')</p>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			eng.SetLangEscapeCatalog(true)
			out, err := eng.Render(name, map[string]any{
				"__trans": map[string]string{"k": `<b>x</b>`},
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `<b>x</b>`) {
				t.Fatalf("lang raw HTML: %q", out)
			}
			if !strings.Contains(out, `&lt;b&gt;`) {
				t.Fatalf("want escaped lang, got %q", out)
			}
		})
		t.Run(string(p)+"/script_payload", func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<p>@lang('k')</p>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			eng.SetLangEscapeCatalog(true)
			out, err := eng.Render(name, map[string]any{
				"__trans": map[string]string{"k": `"><script>alert(1)</script>`},
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `<script>`) {
				t.Fatalf("lang script breakout: %q", out)
			}
		})
	}
}

func TestEscapeSurface_LangReplaceParam(t *testing.T) {
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p), func(t *testing.T) {
			dir := t.TempDir()
			name := escWrite(t, dir, p, `<p>@lang('hi', ['name' => $x])</p>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, map[string]any{
				"x":       `<b>Ada</b>`,
				"__trans": map[string]string{"hi": "Hello :name"},
			})
			if err != nil {
				if p == escAOT {
					t.Logf("aot custom trans: %v", err)
					return
				}
				t.Fatal(err)
			}
			if strings.Contains(out, `<b>Ada</b>`) {
				t.Fatalf("replacement not escaped: %q", out)
			}
			if !strings.Contains(out, `&lt;b&gt;Ada&lt;/b&gt;`) {
				t.Fatalf("want escaped replacement, got %q", out)
			}
		})
	}
}

func TestEscapeSurface_EachAndIncludeWhen(t *testing.T) {
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		t.Run(string(p)+"/each_text_ok", func(t *testing.T) {
			dir := t.TempDir()
			_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
			_ = os.WriteFile(filepath.Join(dir, "partials", "row.html"), []byte(`<li>{{ $item }}</li>`), 0o644)
			name := escWrite(t, dir, p, `@each('partials.row', $items, 'item')`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, map[string]any{"items": []any{`<script>x</script>`}})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, `<script>x</script>`) {
				t.Fatalf("each body not escaped: %q", out)
			}
		})
		t.Run(string(p)+"/includeWhen_script_forbid", func(t *testing.T) {
			dir := t.TempDir()
			_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
			_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`1`), 0o644)
			name := escWrite(t, dir, p, `<script>@includeWhen($ok, 'partials.x')</script>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			_, err := eng.Render(name, map[string]any{"ok": true})
			if err == nil {
				t.Fatal("expected forbid includeWhen in script")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "script") {
				t.Fatalf("want script in %v", err)
			}
		})
		t.Run(string(p)+"/includeFirst_text_ok", func(t *testing.T) {
			dir := t.TempDir()
			_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
			_ = os.WriteFile(filepath.Join(dir, "partials", "a.html"), []byte(`ok`), 0o644)
			name := escWrite(t, dir, p, `<p>@includeFirst(['partials.missing', 'partials.a'])</p>`)
			eng := newEscEngine(dir, p)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "ok") {
				t.Fatalf("got %q", out)
			}
		})
	}
}

func TestEscapeSurface_DumpDdNotImplemented(t *testing.T) { // @dump / @dd are not in KnownDirectives / engine — must remain inert or fail closed.
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(`<p>@dump($x)@dd($x)</p>`), 0o644)
	eng := canvas.New(dir)
	eng.SetEscapeMode(rt.EscapeStrict)
	out, err := eng.Render("p", map[string]any{"x": `<script>`})
	if err != nil {
		t.Logf("compile err (acceptable): %v", err)
		return
	}
	// If treated as literal text, fine; must not eval as raw dump of $x.
	if strings.Contains(out, `<script>`) && !strings.Contains(out, `@dump`) {
		t.Fatalf("unexpected raw script from dump/dd: %q", out)
	}
}

func TestEscapeSurface_URLHelpersNotImplemented(t *testing.T) {
	// @asset/@vite/@route/@url are not Canvas builtins — document absence.
	for _, d := range []string{"asset", "vite", "route", "url", "dump", "dd"} {
		for _, k := range lex.KnownDirectives {
			if strings.EqualFold(k, d) {
				t.Fatalf("%s unexpectedly in KnownDirectives", d)
			}
		}
	}
}

func TestEscapeSurface_CSRFMethodClassStyle(t *testing.T) {
	payload := `" onfocus="x`
	cases := []struct {
		name string
		src  string
		data map[string]any
	}{
		{"csrf", `@csrf`, map[string]any{"_token": payload}},
		{"method", `@method('DELETE')`, nil},
		{"class", `<div @class($m)>`, map[string]any{"m": map[string]any{"ok": true, payload: true}}},
		{"checked", `<input @checked($on)>`, map[string]any{"on": true}},
	}
	for _, p := range []escPath{escAOT, escHTML, escLayout} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				name := escWrite(t, dir, p, tc.src)
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(rt.EscapeStrict)
				out, err := eng.Render(name, tc.data)
				if err != nil {
					if p == escAOT {
						t.Skip(err)
					}
					t.Fatal(err)
				}
				if strings.Contains(out, ` onfocus="x`) {
					t.Fatalf("breakout via %s: %q", tc.name, out)
				}
			})
		}
	}
}
