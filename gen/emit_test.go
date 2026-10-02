package gen_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/gen"
)

func TestEmitListPage(t *testing.T) {
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := gen.EmitListPage("pages", "StreamUsers", doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package pages",
		"func StreamUsers(",
		"rt.WriteEscaped",
		"DO NOT EDIT",
		"Title string",
		"items []StreamUsersItem",
		"it.Name",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestEmitAndOrTemplateStillCompiles(t *testing.T) {
	src := `@if($a && $b)<p>{{ $a }}</p>@endif`
	doc, err := ast.ParseSource(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := gen.Emit("pages", "StreamAnd", doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "func StreamAnd(") {
		t.Fatalf("%s", out)
	}
}
