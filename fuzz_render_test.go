package canvas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

// FuzzRender compiles a fixed template set once, then maps fuzz input to
// (template index, data bytes). Target: >100k exec / 60s in CI nightly.
func FuzzRender(f *testing.F) {
	dir := f.TempDir()
	tmpls := []struct {
		name string
		src  string
	}{
		{"t0", `<p>{{ $x }}</p>`},
		{"t1", `<a href="{{ $u }}">{{ $x }}</a>`},
		{"t2", `<div title={{ $x }}>`},
		{"t3", `@foreach($items as $item)<li>{{ $item }}</li>@endforeach`},
		{"t4", `@if($on)<b>{{ $x }}</b>@endif`},
		{"t5", `<div data-x="@json($x)">`},
		{"t6", `<script>var a=@js($x);</script>`},
		{"t7", `@@if(true){{ $x }}`},
		{"t8", `@verbatim<script>@endverbatim{{ $x }}`},
		{"t9", `{{-- <script> --}}{{ $x }}`},
	}
	for _, tm := range tmpls {
		if err := os.WriteFile(filepath.Join(dir, tm.name+".html"), []byte(tm.src), 0o644); err != nil {
			f.Fatal(err)
		}
	}
	eng := canvas.New(dir)
	eng.SetEscapeMode(rt.EscapeStrict)
	eng.EnableCache(true)
	// Warm compile once.
	for _, tm := range tmpls {
		_, _ = eng.Render(tm.name, map[string]any{
			"x": "ok", "u": "/p", "on": true, "items": []any{"a"},
		})
	}

	f.Add(uint8(0), []byte("hello"))
	f.Add(uint8(1), []byte("javascript:alert(1)"))
	f.Add(uint8(3), []byte("x"))
	f.Add(uint8(7), []byte("<b>"))
	f.Fuzz(func(t *testing.T, idx uint8, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v idx=%d data=%q", r, idx, data)
			}
		}()
		name := tmpls[int(idx)%len(tmpls)].name
		s := string(data)
		if len(s) > 64 {
			s = s[:64]
		}
		_, _ = eng.Render(name, map[string]any{
			"x": s, "u": s, "on": len(data)%2 == 0,
			"items": []any{s, "b"},
		})
	})
}
