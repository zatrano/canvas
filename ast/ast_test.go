package ast_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/ast"
)

func TestParseAndLowerCSRF(t *testing.T) {
	doc, err := ast.ParseSource(`Hello @csrf {{ $name }}`)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("expected CanASTLower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, `_token`) || !strings.Contains(out, "dataGet . `name`") {
		t.Fatalf("out=%q", out)
	}
}

func TestCanASTLowerFalseForAttributes(t *testing.T) {
	doc, err := ast.ParseSource(`{{ $attributes }}`)
	if err != nil {
		t.Fatal(err)
	}
	if doc.CanASTLower() {
		t.Fatal("attributes echo must use regex pipeline")
	}
}

func TestCSRFOnlyFalseForEcho(t *testing.T) {
	doc, err := ast.ParseSource(`{{ $name }} @csrf`)
	if err != nil {
		t.Fatal(err)
	}
	if doc.CSRFOnly() {
		t.Fatal("echo must not be csrf-only")
	}
}

func TestLowerLeafDirectives(t *testing.T) {
	src := `@lang('shop.add') @json($payload) @class($cls) @checked($on) @old('email', '') @auth hi @endauth`
	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("expected CanASTLower for leaf set")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := []string{
		"canvasTrans $ `shop.add`",
		"json (dataGet . `payload`)",
		"classAttr (dataGet . `cls`)",
		"attrBool (dataGet . `on`) `checked`",
		"old . `email` ``",
		"if dataGet . `auth`",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in %q", w, out)
		}
	}
}

func TestLangWithReplacementsFallsBack(t *testing.T) {
	doc, err := ast.ParseSource(`@lang('shop.hello', ['name' => 'Ada'])`)
	if err != nil {
		t.Fatal(err)
	}
	if doc.CanASTLower() {
		t.Fatal("lang replacements must use regex pipeline")
	}
}

func TestLowerIfUnless(t *testing.T) {
	src := `@if($active)yes@elseif($draft)maybe@else no@endif @unless($hidden)show@endunless`
	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("expected CanASTLower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	for _, w := range []string{
		"{{ if dataGet . `active` }}",
		"{{ else if dataGet . `draft` }}",
		"{{ else }}",
		"{{ if not (dataGet . `hidden`) }}",
	} {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in %q", w, out)
		}
	}
}

func TestIfComparisonFallsBack(t *testing.T) {
	doc, err := ast.ParseSource(`@if($n > 1)x@endif`)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("simple comparisons must AST-lower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, "cmpGt") {
		t.Fatalf("want cmpGt in %q", out)
	}
}

func TestForelseKeyAliasLowers(t *testing.T) {
	doc, err := ast.ParseSource(`@forelse($items as $k => $v){{ $k }}:{{ $v }}@empty none@endforelse`)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("forelse key=>alias must AST-lower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, "range $k, $v") {
		t.Fatalf("want keyed range in %q", out)
	}
}

func TestIfAndOrLowers(t *testing.T) {
	doc, err := ast.ParseSource(`@if($a && $b || $c)yes@endif`)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("&& / || must AST-lower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, "(or") || !strings.Contains(out, "(and") {
		t.Fatalf("want and/or in %q", out)
	}
}

func TestIfParenNotEmptyLowers(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{`@if(($a || $b) && $c)yes@endif`, []string{"(and", "(or"}},
		{`@if(!$a)x@endif`, []string{"(not"}},
		{`@if(!empty($x))y@endif`, []string{"(not", "(empty"}},
		{`@if(empty($x))z@endif`, []string{"(empty"}},
		{`@if(isset($user.name))ok@endif`, []string{"issetPath"}},
	}
	for _, tc := range cases {
		doc, err := ast.ParseSource(tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if !doc.CanASTLower() {
			t.Fatalf("%s: want CanASTLower", tc.src)
		}
		out, ok, err := ast.Lower(doc)
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", tc.src, ok, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Fatalf("%s: want %q in %q", tc.src, w, out)
			}
		}
	}
	// Richer calls remain regex.
	rich, err := ast.ParseSource(`@if(count($items) > 0)x@endif`)
	if err != nil {
		t.Fatal(err)
	}
	if rich.CanASTLower() {
		t.Fatal("count() compare must not CanASTLower yet")
	}
}

func TestSectionNest(t *testing.T) {
	doc, err := ast.ParseSource(`@section('content')hello@endsection`)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 1 {
		t.Fatalf("nodes=%d", len(doc.Nodes))
	}
	blk, ok := doc.Nodes[0].(ast.Block)
	if !ok || blk.Name != "section" {
		t.Fatalf("want section block, got %#v", doc.Nodes[0])
	}
	if doc.CanASTLower() {
		// @section … @endsection is definition-only (empty lower); @show emits body.
		out, ok, err := ast.Lower(doc)
		if err != nil || !ok {
			t.Fatalf("section endsection must AST-lower: ok=%v err=%v", ok, err)
		}
		if strings.TrimSpace(out) != "" {
			t.Fatalf("endsection must emit empty, got %q", out)
		}
	} else {
		t.Fatal("section must CanASTLower")
	}

	show, err := ast.ParseSource(`@section('title')App@show`)
	if err != nil {
		t.Fatal(err)
	}
	sb, ok := show.Nodes[0].(ast.Block)
	if !ok || sb.Name != "section" {
		t.Fatalf("show nest: %#v", show.Nodes[0])
	}
	if sb.End != "show" {
		t.Fatalf("End want show got %q", sb.End)
	}
	if !show.CanASTLower() {
		t.Fatal("section@show must CanASTLower")
	}
	sout, sok, serr := ast.Lower(show)
	if serr != nil || !sok {
		t.Fatalf("show lower: ok=%v err=%v", sok, serr)
	}
	if !strings.Contains(sout, "App") {
		t.Fatalf("show must emit body, got %q", sout)
	}

	short, err := ast.ParseSource(`@section('title', 'Hi')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := short.Nodes[0].(ast.Directive); !ok {
		t.Fatalf("short section must stay Directive, got %T", short.Nodes[0])
	}
	if !short.CanASTLower() {
		t.Fatal("short section must CanASTLower")
	}
}

func TestLowerForeach(t *testing.T) {
	src := `@foreach($items as $item)<li>{{ $item.name }} {{ $title }}</li>@endforeach`
	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("expected CanASTLower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	for _, w := range []string{
		`{{ range $__zfi, $item := dataGet $ ` + "`items`" + ` }}`,
		"dataGet $item `name`",
		"dataGet $ `title`",
		"{{ end }}",
	} {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in %q", w, out)
		}
	}
}

func TestForelseFallsBack(t *testing.T) {
	doc, err := ast.ParseSource(`@forelse($items as $i){{ $i }}@empty empty@endforelse`)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("expected CanASTLower for simple forelse")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, "if not (empty") || !strings.Contains(out, "{{ else }}") {
		t.Fatalf("out=%q", out)
	}
}

func TestLowerErrorCan(t *testing.T) {
	src := `@error('email')bad@enderror @can('edit')ok@endcan`
	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.CanASTLower() {
		t.Fatal("expected CanASTLower")
	}
	out, ok, err := ast.Lower(doc)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, "hasError . `email`") || !strings.Contains(out, "can . `edit`") {
		t.Fatalf("out=%q", out)
	}
}

func TestMissingEndforeachErrors(t *testing.T) {
	_, err := ast.ParseSource(`@foreach($items as $i){{ $i }}`)
	if err == nil {
		t.Fatal("expected missing endforeach error")
	}
}
