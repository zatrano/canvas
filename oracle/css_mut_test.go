package oracle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/oracle"
	"github.com/zatrano/canvas/rt"
)

func TestMut_CSSFilterIntact(t *testing.T) {
	dir := t.TempDir()
	tmpl := `<div style="color: {{ $c }}">`
	_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(tmpl), 0o644)
	eng := canvas.New(dir)
	eng.SetEscapeMode(rt.EscapeStrict)
	o, err := eng.Render("p", map[string]any{"c": "red;background:url(javascript:x)"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(o, "unsafe") {
		t.Fatalf("want unsafe filter, got %q", o)
	}
	for _, x := range oracle.CheckRendered(tmpl, o) {
		if x.Kind == "unsafe-css" {
			t.Fatalf("false positive on filtered output: %v out=%q", x, o)
		}
	}
}
