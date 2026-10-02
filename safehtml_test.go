package canvas_test

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

func TestSafeHTML_AndTemplateHTMLTrusted(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`{{ $x }}`), 0o644)
	eng := canvas.New(dir)

	out, err := eng.Render("page", map[string]any{"x": `<b>no</b>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `<b>no</b>`) {
		t.Fatalf("plain string must be escaped, got %q", out)
	}

	out, err = eng.Render("page", map[string]any{"x": rt.SafeHTML(`<b>ok</b>`)})
	if err != nil {
		t.Fatal(err)
	}
	if out != `<b>ok</b>` {
		t.Fatalf("SafeHTML must be raw, got %q", out)
	}

	out, err = eng.Render("page", map[string]any{"x": template.HTML(`<i>ok</i>`)})
	if err != nil {
		t.Fatal(err)
	}
	if out != `<i>ok</i>` {
		t.Fatalf("template.HTML must be raw, got %q", out)
	}
}

func TestSlot_TemplateBodyRaw_GoStringEscaped(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "components", "box.html"), []byte(`<div>{{ $slot }}</div>`), 0o644)

	_ = os.WriteFile(filepath.Join(dir, "body.html"), []byte(
		`@component('box')<script>alert(1)</script>@endcomponent`,
	), 0o644)
	eng := canvas.New(dir)
	out, err := eng.Render("body", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<script>alert(1)</script>`) {
		t.Fatalf("template-body slot must stay raw, got %q", out)
	}

	_ = os.WriteFile(filepath.Join(dir, "gostr.html"), []byte(`<div>{{ $slot }}</div>`), 0o644)
	out, err = eng.Render("gostr", map[string]any{"slot": `<script>alert(1)</script>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `<script>alert(1)</script>`) {
		t.Fatalf("Go string slot must be escaped, got %q", out)
	}
	if !strings.Contains(out, `&lt;script&gt;`) {
		t.Fatalf("expected escaped script, got %q", out)
	}

	out, err = eng.Render("gostr", map[string]any{"slot": rt.SafeHTML(`<em>x</em>`)})
	if err != nil {
		t.Fatal(err)
	}
	if out != `<div><em>x</em></div>` {
		t.Fatalf("SafeHTML Go slot must be raw, got %q", out)
	}
}
