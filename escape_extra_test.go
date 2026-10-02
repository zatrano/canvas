package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
)

func TestStrictExtra_Matrix(t *testing.T) {
	type tc struct {
		name    string
		src     string
		data    map[string]any
		wantErr bool
		wantSub []string
		forbid  []string
		setup   func(dir string) // optional extra files
	}
	cases := []tc{
		{
			name:    "srcdoc_forbid",
			src:     `<iframe srcdoc="{{ $x }}">`,
			data:    map[string]any{"x": `<script>alert(1)</script>`},
			wantErr: true,
		},
		{
			name:    "style_attr_forbid",
			src:     `<div style="{{ $x }}">`,
			data:    map[string]any{"x": `red;background:url(javascript:x)`},
			wantErr: true, // whole-value style interp still compile error
		},
		{
			name:    "style_css_value_ok",
			src:     `<div style="width: {{ $w }}px">`,
			data:    map[string]any{"w": `10`},
			wantSub: []string{`width: 10px`},
		},
		{
			name:    "style_css_value_url",
			src:     `<div style="color: {{ $c }}">`,
			data:    map[string]any{"c": `red;background:url(javascript:x)`},
			wantSub: []string{`unsafe`},
			forbid:  []string{`url(javascript`},
		},
		{
			name:    "style_css_expression",
			src:     `<div style="color: {{ $c }}">`,
			data:    map[string]any{"c": `expression(alert(1))`},
			wantSub: []string{`unsafe`},
		},
		{
			name:    "style_css_breakout",
			src:     `<div style="color: {{ $c }}">`,
			data:    map[string]any{"c": `}</style><script>`},
			wantSub: []string{`unsafe`},
			forbid:  []string{`</style><script>`},
		},
		{
			name:    "style_css_hexescape",
			src:     `<div style="color: {{ $c }}">`,
			data:    map[string]any{"c": `\3c script`},
			wantSub: []string{`unsafe`},
		},
		{
			name:    "tag_name_interp",
			src:     `<{{ $t }}>`,
			data:    map[string]any{"t": `script`},
			wantErr: true,
		},
		{
			name:    "attr_name_interp",
			src:     `<div {{ $attrs }}>`,
			data:    map[string]any{"attrs": `onclick=alert(1)`},
			wantErr: true,
		},
		{
			name:    "between_attrs_interp",
			src:     `<a href="x" {{ $y }}>`,
			data:    map[string]any{"y": `onclick=alert(1)`},
			wantErr: true,
		},
		{
			name:    "multi_interp_url_empty_then_js",
			src:     `<a href="{{ $a }}{{ $b }}">`,
			data:    map[string]any{"a": ``, "b": `javascript:alert(1)`},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`javascript:alert`},
		},
		{
			name:    "multi_interp_url_colon",
			src:     `<a href="{{ $a }}:{{ $b }}">`,
			data:    map[string]any{"a": `javascript`, "b": `alert(1)`},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`javascript:alert`},
		},
		{
			name:    "HREF_case",
			src:     `<a HREF="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:alert(1)`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "formaction",
			src:     `<button formaction="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "poster",
			src:     `<video poster="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "ping",
			src:     `<a ping="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "background",
			src:     `<body background="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "action",
			src:     `<form action="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "xlink_href",
			src:     `<a xlink:href="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "srcset",
			src:     `<img srcset="{{ $x }}">`,
			data:    map[string]any{"x": `javascript:x`},
			wantSub: []string{`#unsafe`},
		},
		{
			name:    "script_close_spaced",
			src:     "<script>var a=1;</script >{{ $x }}",
			data:    map[string]any{"x": `<script>alert(1)</script>`},
			wantSub: []string{`&lt;script&gt;`},
			forbid:  []string{`<script>alert`},
		},
		{
			name:    "ONCLICK_case",
			src:     `<a ONCLICK="{{ $x }}">`,
			data:    map[string]any{"x": `alert(1)`},
			wantErr: true,
		},
		{
			name:    "onClick_mixed",
			src:     `<a onClick="{{ $x }}">`,
			data:    map[string]any{"x": `alert(1)`},
			wantErr: true,
		},
		{
			name:    "SCRIPT_upper",
			src:     `<SCRIPT >var a="{{ $x }}";</SCRIPT>`,
			data:    map[string]any{"x": `x`},
			wantErr: true,
		},
		{
			name:    "script_newline",
			src:     "<script\n>var a=\"{{ $x }}\";</script>",
			data:    map[string]any{"x": `x`},
			wantErr: true,
		},
		{
			name:    "svg_script",
			src:     `<svg><script>{{ $x }}</script></svg>`,
			data:    map[string]any{"x": `alert(1)`},
			wantErr: true,
		},
		{
			name:    "svg_style",
			src:     `<svg><style>{{ $x }}</style></svg>`,
			data:    map[string]any{"x": `x`},
			wantErr: true,
		},
		{
			name: "include_in_script_forbid",
			src:  `<script>@include('partials.x')</script>`,
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`var a=1`), 0o644)
			},
			wantErr: true,
		},
		{
			name: "include_in_text_ok",
			src:  `<p>@include('partials.x')</p>`,
			setup: func(dir string) {
				_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(`ok`), 0o644)
			},
			wantSub: []string{`<p>ok</p>`},
		},
		{
			name:    "textarea_rcdata",
			src:     `<textarea>{{ $x }}</textarea>`,
			data:    map[string]any{"x": `</textarea><script>alert(1)</script>`},
			wantSub: []string{`&lt;/textarea&gt;`},
			forbid:  []string{`</textarea><script>`},
		},
		{
			name:    "title_rcdata",
			src:     `<title>{{ $x }}</title>`,
			data:    map[string]any{"x": `</title><script>alert(1)</script>`},
			wantSub: []string{`&lt;/title&gt;`},
			forbid:  []string{`</title><script>`},
		},
		{
			name:    "html_comment",
			src:     `<!-- {{ $x }} -->`,
			data:    map[string]any{"x": `--> <script>alert(1)</script>`},
			wantSub: []string{`--&gt;`},
			forbid:  []string{`--> <script>`},
		},
	}

	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				if tc.setup != nil {
					tc.setup(dir)
				}
				name := escWrite(t, dir, p, tc.src)
				eng := newEscEngine(dir, p)
				out, err := eng.Render(name, tc.data)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected error, got out=%q", out)
					}
					return
				}
				if err != nil {
					t.Fatalf("err=%v out=%q", err, out)
				}
				for _, w := range tc.wantSub {
					if !strings.Contains(out, w) {
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

func TestStrictExtra_DirectiveAttrPayload(t *testing.T) {
	// Inventory: @class/@style/@checked/@csrf/@method must not break out of attributes.
	payload := `" onmouseover="alert(1)`
	cases := []struct {
		name string
		src  string
		data map[string]any
	}{
		{"class", `@class($m)`, map[string]any{"m": map[string]any{payload: true}}},
		{"style", `@style($m)`, map[string]any{"m": map[string]any{payload: true}}},
		{"checked", `<input type="checkbox" @checked($on)>`, map[string]any{"on": true}},
		{"selected", `<option @selected($on)>`, map[string]any{"on": true}},
		{"disabled", `<input @disabled($on)>`, map[string]any{"on": true}},
		{"csrf", `@csrf`, map[string]any{"_token": payload}},
		{"method", `@method('PUT')`, nil},
	}
	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				name := escWrite(t, dir, p, tc.src)
				eng := newEscEngine(dir, p)
				out, err := eng.Render(name, tc.data)
				if err != nil {
					// some directives may not AOT-lower; still check html/layout
					if p == escAOT {
						t.Skip(err)
					}
					t.Fatal(err)
				}
				if strings.Contains(out, ` onmouseover="alert`) || strings.Contains(out, `onmouseover=alert`) {
					t.Fatalf("attribute breakout via %s: %q", tc.name, out)
				}
				if strings.Contains(out, payload) && tc.name != "method" {
					// raw payload in output is a breakout risk unless entity-escaped
					if !strings.Contains(out, `&#34;`) && !strings.Contains(out, `&quot;`) {
						t.Fatalf("%s leaked raw quote payload: %q", tc.name, out)
					}
				}
			})
		}
	}
}

func TestStrictExtra_ErrorMessageQuality(t *testing.T) {
	dir := t.TempDir()
	src := "<a onclick=\"{{ $x }}\">\n"
	_ = os.WriteFile(filepath.Join(dir, "bad.html"), []byte(src), 0o644)
	eng := canvas.New(dir)
	_, err := eng.Render("bad", map[string]any{"x": `alert(1)`})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad") {
		t.Fatalf("want template name in %v", err)
	}
	if !strings.Contains(msg, ":") {
		t.Fatalf("want line:col in %v", err)
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "on") && !strings.Contains(lower, "attribute") {
		t.Fatalf("want detected context in %v", err)
	}
	if !strings.Contains(msg, "@js") && !strings.Contains(msg, "data-*") && !strings.Contains(msg, "SafeHTML") && !strings.Contains(msg, "@attrs") {
		t.Fatalf("want fix hint (@js / @attrs / data-* / SafeHTML) in %v", err)
	}
}
