package canvas_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/ast"
)

func TestRichIfExprASTLower(t *testing.T) {
	cases := []struct {
		name string
		src  string
		data map[string]any
		want string
		miss string
	}{
		{
			name: "count",
			src:  `@if(count($items) > 0)yes@endif`,
			data: map[string]any{"items": []any{1, 2}},
			want: "yes",
		},
		{
			name: "count_empty",
			src:  `@if(count($items) > 0)yes@else no@endif`,
			data: map[string]any{"items": []any{}},
			want: "no",
		},
		{
			name: "ternary_echo",
			src:  `{{ $on ? 'Y' : 'N' }}`,
			data: map[string]any{"on": true},
			want: "Y",
		},
		{
			name: "in_array",
			src:  `@if(in_array('admin', $roles))ok@endif`,
			data: map[string]any{"roles": []any{"user", "admin"}},
			want: "ok",
		},
		{
			name: "index",
			src:  `@if($matrix[$role][$perm])hit@endif`,
			data: map[string]any{
				"role": "a",
				"perm": "b",
				"matrix": map[string]any{
					"a": map[string]any{"b": true},
				},
			},
			want: "hit",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ast.ParseSource(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if !doc.CanASTLower() {
				t.Fatal("want CanASTLower via CondExprLower bridge")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "t.html")
			if err := os.WriteFile(path, []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			eng := canvas.New(dir)
			out, err := eng.Render("t", tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("got %q want substring %q", out, tc.want)
			}
		})
	}
}
