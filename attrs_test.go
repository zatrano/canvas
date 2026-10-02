package canvas_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

func TestAttrs_Strict(t *testing.T) {
	type tc struct {
		name      string
		src       string
		data      map[string]any
		wantErr   bool
		errNeedle string
		wantSub   []string
		forbid    []string
	}
	cases := []tc{
		{
			name:    "valid_bag",
			src:     `<div class="x" @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"id": "a", "data-x": "1"}},
			wantSub: []string{`class="x"`, ` data-x="1"`, ` id="a"`}, // sorted: data-x before id
		},
		{
			name:    "empty_nil",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": nil},
			wantSub: []string{`<div`},
			forbid:  []string{`id=`, `onclick`},
		},
		{
			name:    "boolean_true_false",
			src:     `<input @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]any{"disabled": true, "checked": false, "name": "n"}},
			wantSub: []string{` disabled`, ` name="n"`},
			forbid:  []string{`checked`},
		},
		{
			name:    "on_reject",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"onclick": "alert(1)", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`onclick`, `alert`},
		},
		{
			name:    "name_regex",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"x onmouseover": "alert(1)", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`onmouseover`},
		},
		{
			name:    "reject_quote_name",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{`a"b`: "x", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`a"b`},
		},
		{
			name:    "reject_ON_prefix",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"ONclick": "x", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`ONclick`, `onclick`},
		},
		{
			name:    "srcdoc_style_reject",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"srcdoc": "<b>", "style": "x", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`srcdoc`, `style=`},
		},
		{
			name:    "value_escape",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"title": `"><script>alert(1)</script>`}},
			wantSub: []string{`&lt;`, `&#34;`, `&gt;`},
			forbid:  []string{`<script>`},
		},
		{
			name:    "url_scheme",
			src:     `<a @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"href": "javascript:alert(1)"}},
			wantSub: []string{`href="#unsafe"`},
			forbid:  []string{`javascript:alert`},
		},
		{
			name:    "href_tab_scheme",
			src:     `<a @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"href": "java\tscript:alert(1)"}},
			wantSub: []string{`#unsafe`},
			forbid:  []string{`javascript:`},
		},
		{
			name:    "safehtml_not_raw",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]any{"title": rt.SafeHTML(`<b>x</b>`)}},
			wantSub: []string{`&lt;b&gt;`},
			forbid:  []string{`<b>x</b>`},
		},
		{
			name:    "ordered_attrs_type",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": rt.Attrs{{Name: "z", Value: "1"}, {Name: "a", Value: "2"}}},
			wantSub: []string{` z="1" a="2"`}, // preserve Attrs order
		},
		{
			name:      "position",
			src:       `<p>@attrs($attrs)</p>`,
			data:      map[string]any{"attrs": map[string]string{"id": "x"}},
			wantErr:   true,
			errNeedle: "text",
		},
		{
			name:      "wrong_pos_value",
			src:       `<div title="@attrs($attrs)">`,
			data:      map[string]any{"attrs": map[string]string{"id": "x"}},
			wantErr:   true,
			errNeedle: "attribute",
		},
		{
			name:      "wrong_pos_script",
			src:       `<script>@attrs($attrs)</script>`,
			data:      map[string]any{"attrs": map[string]string{"id": "x"}},
			wantErr:   true,
			errNeedle: "script",
		},
		{
			name:      "wrong_pos_style",
			src:       `<style>@attrs($attrs)</style>`,
			data:      map[string]any{"attrs": map[string]string{"id": "x"}},
			wantErr:   true,
			errNeedle: "style",
		},
		{
			name:    "boolean_nil_skip",
			src:     `<input @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]any{"disabled": nil, "name": "n"}},
			wantSub: []string{` name="n"`},
			forbid:  []string{`disabled`},
		},
		{
			name:    "reject_ON_space",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"ON click": "x", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`ON click`, `click=`},
		},
		{
			name:    "reject_srcset_xmlns",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"srcset": "x", "xmlns:x": "y", "id": "ok"}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`srcset`, `xmlns`},
		},
		{
			name: "reject_null_long_unicode_names",
			src:  `<div @attrs($attrs)>`,
			data: map[string]any{"attrs": map[string]string{
				"id":                     "ok",
				"a\x00b":                 "x",
				strings.Repeat("a", 300): "x",
				"名前":                     "x",
			}},
			wantSub: []string{` id="ok"`},
			forbid:  []string{`名前`, strings.Repeat("a", 20)},
		},
		{
			name:    "value_onfocus_breakout",
			src:     `<div @attrs($attrs)>`,
			data:    map[string]any{"attrs": map[string]string{"title": `' onfocus='x`}},
			wantSub: []string{`&#39;`, `onfocus`},
			forbid:  []string{`title="' onfocus`},
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
				if tc.wantErr {
					if err == nil {
						t.Fatalf("expected err, out=%q", out)
					}
					if tc.errNeedle != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errNeedle)) {
						t.Fatalf("want %q in %v", tc.errNeedle, err)
					}
					if !strings.Contains(err.Error(), "@attrs") {
						t.Fatalf("want @attrs hint in %v", err)
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

func TestAttrs_DeterministicMapOrder(t *testing.T) {
	dir := t.TempDir()
	name := escWrite(t, dir, escAOT, `<div @attrs($attrs)>`)
	eng := canvas.New(dir)
	data := map[string]any{"attrs": map[string]string{"c": "1", "a": "2", "b": "3"}}
	var outs []string
	for i := 0; i < 20; i++ {
		out, err := eng.Render(name, data)
		if err != nil {
			t.Fatal(err)
		}
		outs = append(outs, out)
	}
	for i := 1; i < len(outs); i++ {
		if outs[i] != outs[0] {
			t.Fatalf("non-deterministic: %q vs %q", outs[0], outs[i])
		}
	}
	if !strings.Contains(outs[0], ` a="2" b="3" c="1"`) {
		t.Fatalf("want sorted keys, got %q", outs[0])
	}
}

func TestAttrs_ForbidHintMentionsAttrs(t *testing.T) {
	dir := t.TempDir()
	name := escWrite(t, dir, escAOT, `<div {{ $attrs }}>`)
	eng := canvas.New(dir)
	_, err := eng.Render(name, map[string]any{"attrs": "x"})
	if err == nil {
		t.Fatal("expected forbid")
	}
	if !strings.Contains(err.Error(), "@attrs") {
		t.Fatalf("want @attrs in %v", err)
	}
}
