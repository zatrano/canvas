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
