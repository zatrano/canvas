package oracle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

func TestMutationCoverage_Rules(t *testing.T) {
	cases := []struct {
		rule string
		tmpl string
		data map[string]any
		err  bool
		sub  string
	}{
		{"url-scheme", `<a href="{{ $x }}">`, map[string]any{"x": "javascript:alert(1)"}, false, "#unsafe"},
		{"unquoted-space", `<div title={{ $x }}>`, map[string]any{"x": "a b"}, false, "&#32;"},
		{"unquoted-tab", `<div title={{ $x }}>`, map[string]any{"x": "a\tb"}, false, "&#9;"},
		{"unquoted-eq", `<div title={{ $x }}>`, map[string]any{"x": "a=b"}, false, "&#61;"},
		{"unquoted-lt", `<div title={{ $x }}>`, map[string]any{"x": "a<b"}, false, "&lt;"},
		{"unquoted-amp", `<div title={{ $x }}>`, map[string]any{"x": "a&b"}, false, "&amp;"},
		{"unquoted-quot", `<div title={{ $x }}>`, map[string]any{"x": `a"b`}, false, "&#34;"},
		{"on-forbid", `<a onclick="{{ $x }}">`, map[string]any{"x": "1"}, true, ""},
		{"script-forbid-echo", `<script>{{ $x }}</script>`, map[string]any{"x": "1"}, true, ""},
		{"style-el-forbid", `<style>{{ $x }}</style>`, map[string]any{"x": "1"}, true, ""},
		{"srcdoc-forbid", `<iframe srcdoc="{{ $x }}">`, map[string]any{"x": "1"}, true, ""},
		{"style-whole-forbid", `<div style="{{ $x }}">`, map[string]any{"x": "1"}, true, ""},
		{"tag-name-forbid", `<{{ $t }}>`, map[string]any{"t": "x"}, true, ""},
		{"attr-name-forbid", `<div {{ $a }}>`, map[string]any{"a": "x"}, true, ""},
		{"json-script-escape", `<script>var a=@json($x);</script>`, map[string]any{"x": "</script>"}, false, `\u003c`},
		{"json-attr-escape", `<div data-x="@json($x)">`, map[string]any{"x": `a"b`}, false, "&#34;"},
		{"js-in-script", `<script>@js($x)</script>`, map[string]any{"x": "ok"}, false, ""},
		{"multi-interp-url", `<a href="x{{ $b }}">`, map[string]any{"b": "javascript:alert(1)"}, false, "#unsafe"},
		{"css-value-filter", `<div style="color: {{ $c }}">`, map[string]any{"c": "red;x"}, false, "unsafe"},
		{"unquoted-json-forbid", `<div id=@json($x)>`, map[string]any{"x": "1"}, true, ""},
		{"onclick-json-forbid", `<a onclick="@json($x)">`, map[string]any{"x": "1"}, true, ""},
		{"srcdoc-json-forbid", `<iframe srcdoc="@json($x)">`, map[string]any{"x": "1"}, true, ""},
		{"include-in-script", `<script>@include('partials.x')</script>`, map[string]any{}, true, ""},
		{"unknown-end-directive-error", `@endfoo`, map[string]any{}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			d := t.TempDir()
			if strings.Contains(tc.tmpl, "partials") {
				_ = os.MkdirAll(filepath.Join(d, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(d, "partials", "x.html"), []byte(`1`), 0o644)
			}
			_ = os.WriteFile(filepath.Join(d, "p.html"), []byte(tc.tmpl), 0o644)
			eng := canvas.New(d)
			eng.SetEscapeMode(rt.EscapeStrict)
			out, err := eng.Render("p", tc.data)
			if tc.err {
				if err == nil {
					t.Fatalf("rule %s: expected err, out=%q", tc.rule, out)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.sub != "" && !strings.Contains(out, tc.sub) {
				t.Fatalf("rule %s: want %q in %q", tc.rule, tc.sub, out)
			}
		})
	}
}
