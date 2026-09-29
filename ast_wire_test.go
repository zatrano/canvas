package canvas_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	template "github.com/zatrano/canvas"
)

func TestCompileRejectsUnclosedEcho(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.html")
	if err := os.WriteFile(path, []byte(`Hello {{ $name`), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := template.New(dir)
	_, err := eng.Render("bad", nil)
	if err == nil || !strings.Contains(err.Error(), "unclosed") {
		t.Fatalf("expected unclosed lex error, got %v", err)
	}
}

func TestCompileCSRFOnlyViaAST(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "form.html"), []byte(`<form>@csrf</form>`), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := template.New(dir)
	out, err := eng.Render("form", map[string]any{"_token": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `name="_token"`) || !strings.Contains(out, "abc") {
		t.Fatalf("out=%q", out)
	}
}
