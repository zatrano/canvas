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

// TestContextTracker_VerbatimOutputContext: escape scanner must treat verbatim
// bodies as raw HTML in the OUTPUT stream (not the lexer placeholder).
func TestContextTracker_VerbatimOutputContext(t *testing.T) {
	type tc struct {
		name       string
		src        string
		data       map[string]any
		wantErr    bool
		errNeedle  string
		wantSub    []string
		forbid     []string
		outContext string // documented HTML context after expand
	}
	cases := []tc{
		{
			name:       "1_script_wraps_verbatim_then_echo",
			src:        `<script>@verbatim <b>@endverbatim{{ $x }}</script>`,
			data:       map[string]any{"x": `alert(1)`},
			wantErr:    true,
			errNeedle:  "script",
			outContext: "script (verbatim body is <b> inside <script>)",
		},
		{
			name:       "2_verbatim_opens_script_then_echo",
			src:        `@verbatim <script> @endverbatim {{ $x }}`,
			data:       map[string]any{"x": `alert(1)`},
			wantErr:    true,
			errNeedle:  "script",
			outContext: "script (verbatim emitted open <script> before echo)",
		},
		{
			name:       "3_verbatim_unquoted_title",
			src:        `@verbatim <div title= @endverbatim{{ $x }}>`,
			data:       map[string]any{"x": `a onmouseover=alert(1)`},
			wantSub:    []string{`&#32;`, `&#61;`},
			forbid:     []string{`title=a onmouseover`, ` onmouseover=`},
			outContext: "unquoted attr value",
		},
		{
			name:       "4_verbatim_href_open_quote",
			src:        `@verbatim <a href=" @endverbatim{{ $x }}">`,
			data:       map[string]any{"x": `javascript:alert(1)`},
			wantSub:    []string{`#unsafe`},
			forbid:     []string{`javascript:alert`},
			outContext: "URL attr (quoted)",
		},
		{
			name:       "5_verbatim_opens_style",
			src:        `@verbatim <style> @endverbatim {{ $x }}`,
			data:       map[string]any{"x": `x`},
			wantErr:    true,
			errNeedle:  "style",
			outContext: "style element",
		},
		{
			name:       "6_verbatim_literal_mustache",
			src:        `@verbatim {{ $x }} @endverbatim`,
			data:       map[string]any{"x": `ignored`},
			wantSub:    []string{`{{ $x }}`},
			outContext: "text (echo inside verbatim is literal)",
		},
		{
			name:       "7_template_comment_script",
			src:        `{{-- <script> --}} {{ $x }}`,
			data:       map[string]any{"x": `<b>ok</b>`},
			wantSub:    []string{`&lt;b&gt;`},
			forbid:     []string{`<script>`},
			outContext: "text (comment stripped before scan)",
		},
		{
			name:       "8a_template_comment_close_seq",
			src:        `{{-- --> --}} {{ $x }}`,
			data:       map[string]any{"x": `hi`},
			wantSub:    []string{`hi`},
			outContext: "text",
		},
		{
			name:       "8b_atat_if",
			src:        `@@if(true) {{ $x }}`,
			data:       map[string]any{"x": `y`},
			wantSub:    []string{`@if(true)`, `y`},
			outContext: "text",
		},
		{
			name:       "8c_atat_in_attr_with_echo",
			src:        `<a title="@@foo {{ $x }}">`,
			data:       map[string]any{"x": `"bar`},
			wantSub:    []string{`title="@foo `, `&#34;`},
			outContext: "quoted attr",
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
				t.Logf("ctx=%s err=%v out=%q", tc.outContext, err, out)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected compile error (%s), out=%q", tc.outContext, out)
					}
					if tc.errNeedle != "" && !strings.Contains(strings.ToLower(err.Error()), tc.errNeedle) {
						t.Fatalf("error missing %q: %v", tc.errNeedle, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("err=%v", err)
				}
				for _, w := range tc.wantSub {
					if !strings.Contains(out, w) {
						// html/template may double-escape entities on html path
						if w == `&#32;` && strings.Contains(out, `&amp;#32;`) {
							continue
						}
						if w == `&#61;` && strings.Contains(out, `&amp;#61;`) {
							continue
						}
						if w == `&#34;` && strings.Contains(out, `&amp;#34;`) {
							continue
						}
						t.Fatalf("want %q in %q", w, out)
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

func TestMaxNestingDepth(t *testing.T) {
	build := func(depth int) string {
		var b strings.Builder
		for i := 0; i < depth; i++ {
			b.WriteString("@if($x)")
		}
		b.WriteString("ok")
		for i := 0; i < depth; i++ {
			b.WriteString("@endif")
		}
		return b.String()
	}

	t.Run("depth_500_no_panic", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "d.html"), []byte(build(500)), 0o644)
		eng := canvas.New(dir)
		eng.MaxNestingDepth = 600
		out, err := eng.Render("d", map[string]any{"x": true})
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if !strings.Contains(out, "ok") {
			t.Fatalf("out=%q", out)
		}
	})

	t.Run("default_200_rejects_201", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "d.html"), []byte(build(201)), 0o644)
		eng := canvas.New(dir)
		_, err := eng.Render("d", map[string]any{"x": true})
		if err == nil {
			t.Fatal("expected MaxNestingDepth error")
		}
		if !strings.Contains(err.Error(), "MaxNestingDepth") {
			t.Fatalf("want MaxNestingDepth in %v", err)
		}
	})

	t.Run("foreach_depth_limit", func(t *testing.T) {
		var b strings.Builder
		const d = 201
		for i := 0; i < d; i++ {
			b.WriteString(fmt.Sprintf("@foreach($items as $i%d)", i))
		}
		b.WriteString("x")
		for i := 0; i < d; i++ {
			b.WriteString("@endforeach")
		}
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "d.html"), []byte(b.String()), 0o644)
		eng := canvas.New(dir)
		_, err := eng.Render("d", map[string]any{"items": []any{1}})
		if err == nil {
			t.Fatal("expected nesting error")
		}
		if !strings.Contains(err.Error(), "MaxNestingDepth") {
			t.Fatalf("want MaxNestingDepth in %v", err)
		}
	})
}
