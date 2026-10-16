package canvas_test

import (
	"os"
	"path/filepath"
	"testing"

	template "github.com/zatrano/canvas"
)

// An @if inside an inner @foreach must not disturb the outer loop: every
// outer iteration keeps its own variable and inner collection.
func TestNestedForeachWithIfKeepsOuterIterations(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"if-else": {
			src:  "@foreach($nav as $s)[{{ $s.title }}:@foreach($s.pages as $p)@if($p.active)<b>{{ $p.t }}</b>@else{{ $p.t }}@endif;@endforeach]@endforeach",
			want: "[A:a1;<b>a2</b>;][B:b1;]",
		},
		"if-only": {
			src:  "@foreach($nav as $s)[{{ $s.title }}:@foreach($s.pages as $p)@if($p.active)*@endif{{ $p.t }};@endforeach]@endforeach",
			want: "[A:a1;*a2;][B:b1;]",
		},
		"no-if": {
			src:  "@foreach($nav as $s)[{{ $s.title }}:@foreach($s.pages as $p){{ $p.t }};@endforeach]@endforeach",
			want: "[A:a1;a2;][B:b1;]",
		},
		"outer-if": {
			src:  "@foreach($nav as $s)@if($s.title)[{{ $s.title }}]@endif@endforeach",
			want: "[A][B]",
		},
	}
	data := map[string]any{"nav": []map[string]any{
		{"title": "A", "pages": []map[string]any{{"t": "a1", "active": false}, {"t": "a2", "active": true}}},
		{"title": "B", "pages": []map[string]any{{"t": "b1", "active": false}}},
	}}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if out := renderSrc(t, tc.src, data); out != tc.want {
				t.Fatalf("got  %q\nwant %q", out, tc.want)
			}
		})
	}
}

type navItem struct {
	Title  string
	Active bool
}

// A top-level @foreach over a collection the zero-alloc frame does not
// handle (map, struct slice) must fall back to the alias path, with an @if
// in the body so the interpreter runs.
func TestForeachFallbackCollectionsWithIf(t *testing.T) {
	cases := map[string]struct {
		src  string
		data map[string]any
		want string
	}{
		"struct-slice": {
			src:  "@foreach($items as $i)@if($i.Active)*@endif{{ $i.Title }};@endforeach",
			data: map[string]any{"items": []navItem{{"a", false}, {"b", true}}},
			want: "a;*b;",
		},
		"map": {
			src:  "@foreach($items as $k => $v)@if($v){{ $k }}@endif@endforeach",
			data: map[string]any{"items": map[string]any{"only": true}},
			want: "only",
		},
		"nested-struct-in-map-slice": {
			src: "@foreach($nav as $s)[{{ $s.title }}:@foreach($s.items as $i)@if($i.Active)*@endif{{ $i.Title }};@endforeach]@endforeach",
			data: map[string]any{"nav": []map[string]any{
				{"title": "A", "items": []navItem{{"a1", true}}},
				{"title": "B", "items": []navItem{{"b1", false}}},
			}},
			want: "[A:*a1;][B:b1;]",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if out := renderSrc(t, tc.src, tc.data); out != tc.want {
				t.Fatalf("got  %q\nwant %q", out, tc.want)
			}
		})
	}
}

func renderSrc(t *testing.T, src string, data map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.html"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := template.New(dir).Render("t", data)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
