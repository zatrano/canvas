package canvas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas/rt"
)

// TestLayoutPlacement_StrictForbiddenCallSites — red→green: before yield/stack/$slot
// placement checks, href/unquoted @yield and title="{{ $slot }}" compiled successfully
// (literal javascript: / onmouseover breakout risk). Now call sites in script/style/
// tag/attribute/comment fail closed with the context name in the error.
func TestLayoutPlacement_StrictForbiddenCallSites(t *testing.T) {
	type tc struct {
		name      string
		layout    string // layouts/app.html; empty → single-file body
		body      string // page.html (extends when layout set)
		partial   string // optional partials/x.html
		component string // optional components/box.html
		data      map[string]any
		wantErr   bool
		errNeedle string
		wantSub   []string
		forbid    []string
	}
	cases := []tc{
		{
			name:    "yield_in_script",
			layout:  `<html><script>@yield('js')</script>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('js')\nvar a = 1;\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "script",
		},
		{
			name:    "yield_in_style",
			layout:  `<html><style>@yield('css')</style>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('css')\ncolor: red;\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "style",
		},
		{
			name:    "yield_in_href",
			layout:  `<html><a href="@yield('u')">x</a>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('u')\njavascript:alert(1)\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "yield_in_href_sq",
			layout:  `<html><a href='@yield("u")'>x</a>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('u')\nx\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "yield_in_tag",
			layout:  `<html><@yield('t')>x</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('t')\ndiv\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "tag",
		},
		{
			name:    "yield_between_attrs",
			layout:  `<html><div @yield('attrs')>x</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('attrs')\nid=\"ok\"\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "yield_unquoted_attr",
			layout:  `<html><div class=@yield('c')>x</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('c')\nx onmouseover=alert(1)\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "yield_in_comment",
			layout:  `<html><!-- @yield('c') -->@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('c')\nx\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "comment",
		},
		{
			name:    "stack_in_style_forbid",
			layout:  `<html><style>@stack('s')</style>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('s')\ncolor:red;\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "style",
		},
		{
			name:    "stack_in_href_forbid",
			layout:  `<html><a href="@stack('s')">x</a>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('s')\n#\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:      "slot_in_title_attr",
			component: `<div title="{{ $slot }}">`,
			body:      "@component('components.box')\n\" onmouseover=alert(1)\n@endcomponent\n",
			wantErr:   true, errNeedle: "attribute",
		},
		{
			name:      "slot_in_script",
			component: `<script>{{ $slot }}</script>`,
			body:      "@component('components.box')\nvar a=1;\n@endcomponent\n",
			// Placement reject names the component; compile-time echo forbid does not.
			wantErr: true, errNeedle: "components.box",
		},
		{
			name:    "include_in_comment",
			body:    `<!-- @include('partials.x') -->`,
			partial: `x`,
			wantErr: true, errNeedle: "comment",
		},
		{
			name:    "root_layout_href_reject",
			layout:  `<html><a href="@yield('u')">x</a>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('u')\nok\n@endsection\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "stack_in_text_ok_but_script_echo_err",
			layout:  `<html><div>@stack('s')</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('s')\n<script>var a={{ $x }};</script>\n@endpush\n@section('content')\nok\n@endsection\n",
			data:    map[string]any{"x": "1"},
			wantErr: true, errNeedle: "script",
		},
		{
			name:    "stack_in_script_forbid",
			layout:  `<html><script>@stack('s')</script>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('s')\nvar a=1;\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "script",
		},
		{
			name:    "yield_in_title_ok",
			layout:  `<html><title>@yield('title')</title>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@section('title')\nHello\n@endsection\n@section('content')\nok\n@endsection\n",
			wantSub: []string{`<title>Hello</title>`, `ok`},
		},
		{
			name:    "include_in_text_ok",
			body:    `<p>@include('partials.x')</p>`,
			partial: `ok`,
			wantSub: []string{`<p>ok</p>`},
		},
		{
			name:    "include_in_script_forbid",
			body:    `<script>@include('partials.x')</script>`,
			partial: `1`,
			wantErr: true, errNeedle: "script",
		},
		// Full yield/stack/slot/include × context matrix (mutation granularity).
		{
			name:    "stack_in_tag",
			layout:  `<html><@stack('t')>x</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('t')\ndiv\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "tag",
		},
		{
			name:    "stack_between_attrs",
			layout:  `<html><div @stack('a')>x</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('a')\nid=\"ok\"\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "stack_unquoted_attr",
			layout:  `<html><div class=@stack('c')>x</div>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('c')\nx\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "stack_in_href_sq",
			layout:  `<html><a href='@stack("s")'>x</a>@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('s')\n#\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "stack_in_comment",
			layout:  `<html><!-- @stack('s') -->@yield('content')</html>`,
			body:    "@extends('layouts.app')\n@push('s')\nx\n@endpush\n@section('content')\nok\n@endsection\n",
			wantErr: true, errNeedle: "comment",
		},
		{
			name:      "slot_in_style",
			component: `<style>{{ $slot }}</style>`,
			body:      "@component('components.box')\ncolor:red;\n@endcomponent\n",
			wantErr:   true, errNeedle: "components.box",
		},
		{
			name:      "slot_in_tag",
			component: `<{{ $slot }}>x</div>`,
			body:      "@component('components.box')\ndiv\n@endcomponent\n",
			wantErr:   true, errNeedle: "components.box",
		},
		{
			name:      "slot_between_attrs",
			component: `<div {{ $slot }}>x</div>`,
			body:      "@component('components.box')\nid=\"ok\"\n@endcomponent\n",
			wantErr:   true, errNeedle: "components.box",
		},
		{
			name:      "slot_unquoted_attr",
			component: `<div class={{ $slot }}>x</div>`,
			body:      "@component('components.box')\nok\n@endcomponent\n",
			wantErr:   true, errNeedle: "attribute",
		},
		{
			name:      "slot_in_href_sq",
			component: `<a href='{{ $slot }}'>x</a>`,
			body:      "@component('components.box')\n#\n@endcomponent\n",
			wantErr:   true, errNeedle: "attribute",
		},
		{
			name:      "slot_in_comment",
			component: `<!-- {{ $slot }} -->`,
			body:      "@component('components.box')\nx\n@endcomponent\n",
			wantErr:   true, errNeedle: "comment",
		},
		{
			name:    "include_in_style",
			body:    `<style>@include('partials.x')</style>`,
			partial: `1`,
			wantErr: true, errNeedle: "style",
		},
		{
			name:    "include_in_tag",
			body:    `<@include('partials.x')>x</div>`,
			partial: `div`,
			wantErr: true, errNeedle: "tag",
		},
		{
			name:    "include_in_href",
			body:    `<a href="@include('partials.x')">x</a>`,
			partial: `#`,
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "include_in_href_sq",
			body:    `<a href='@include("partials.x")'>x</a>`,
			partial: `#`,
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "include_unquoted_attr",
			body:    `<div class=@include('partials.x')>x</div>`,
			partial: `ok`,
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "include_between_attrs",
			body:    `<div @include('partials.x')>x</div>`,
			partial: `id="ok"`,
			wantErr: true, errNeedle: "attribute",
		},
		{
			name:    "includeWhen_in_script",
			body:    `<script>@includeWhen($ok, 'partials.x')</script>`,
			partial: `1`,
			data:    map[string]any{"ok": true},
			wantErr: true, errNeedle: "script",
		},
		{
			name:    "includeIf_in_comment",
			body:    `<!-- @includeIf('partials.x') -->`,
			partial: `x`,
			wantErr: true, errNeedle: "comment",
		},
	}

	paths := []escPath{escAOT, escHTML, escLayout}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", p, tc.name), func(t *testing.T) {
				dir := t.TempDir()
				var name string
				if tc.layout != "" {
					_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
					_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(tc.layout), 0o644)
				}
				if tc.partial != "" {
					_ = os.MkdirAll(filepath.Join(dir, "partials"), 0o755)
					_ = os.WriteFile(filepath.Join(dir, "partials", "x.html"), []byte(tc.partial), 0o644)
				}
				if tc.component != "" {
					_ = os.MkdirAll(filepath.Join(dir, "components"), 0o755)
					_ = os.WriteFile(filepath.Join(dir, "components", "box.html"), []byte(tc.component), 0o644)
				}
				if tc.layout != "" || strings.Contains(tc.body, "@extends") || strings.Contains(tc.body, "@component") {
					// layout path wraps with @extends — but body already has extends; for escLayout
					// escWrite would double-wrap. Write files manually for placement cases.
					switch p {
					case escAOT:
						_ = os.WriteFile(filepath.Join(dir, "p_aot.html"), []byte(tc.body), 0o644)
						name = "p_aot"
					case escHTML:
						_ = os.WriteFile(filepath.Join(dir, "p_html.html"), []byte(tc.body), 0o644)
						name = "p_html"
					case escLayout:
						if tc.layout != "" {
							// already extends layouts.app
							_ = os.WriteFile(filepath.Join(dir, "p_layout.html"), []byte(tc.body), 0o644)
						} else {
							_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
							_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`@yield('content')`), 0o644)
							src := "@extends('layouts.app')\n@section('content')\n" + tc.body + "\n@endsection\n"
							_ = os.WriteFile(filepath.Join(dir, "p_layout.html"), []byte(src), 0o644)
						}
						name = "p_layout"
					}
				} else {
					name = escWrite(t, dir, p, tc.body)
				}
				eng := newEscEngine(dir, p)
				eng.SetEscapeMode(rt.EscapeStrict)
				out, err := eng.Render(name, tc.data)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected err, out=%q", out)
					}
					if tc.errNeedle != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errNeedle)) {
						t.Fatalf("want %q in %v", tc.errNeedle, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("err=%v", err)
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
