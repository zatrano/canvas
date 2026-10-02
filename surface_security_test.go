package canvas_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
)

// Expression surface, AddFunc, custom directives, path traversal.
// Pre-stated expectations:
//   - No Method() call from templates (FieldByName only)
//   - No code-execution directive in the engine
//   - Custom Directive returns raw HTML (caller responsibility)
//   - pathFor may allow .. traversal — VERIFY

type surfaceVictim struct {
	Public  string
	private string
}

func (v surfaceVictim) Secret() string { return "METHOD_OK" }

func TestSurface_NoMethodCall(t *testing.T) {
	dir := t.TempDir()
	// Force html/template path (AOT wLookup is map-only) so dataGet struct rules apply.
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`{{ $v.Secret }}|{{ $v.Public }}|{{ $v.private }}`), 0o644)
	eng := canvas.New(dir)
	eng.PreferHTMLPath(true)
	out, err := eng.Render("page", map[string]any{
		"v": surfaceVictim{Public: "PUB", private: "PRIV"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("OUT=%q", out)
	if strings.Contains(out, "METHOD_OK") {
		t.Fatalf("method call must not be reachable from templates, got %q", out)
	}
	if strings.Contains(out, "PRIV") {
		t.Fatalf("unexported field must not leak, got %q", out)
	}
	if !strings.Contains(out, "PUB") {
		t.Fatalf("exported field Public missing: %q", out)
	}
}

func TestSurface_UnknownCodeBlockLeftInSource(t *testing.T) {
	// Unrecognized @widget…@endwidget: Strict rejects the unknown closer (no execution).
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte("@widget('secret')\n@endwidget\n"), 0o644)
	eng := canvas.New(dir)
	out, err := eng.Render("page", nil)
	if err == nil {
		t.Fatalf("expected Strict error for unknown @endwidget, out=%q", out)
	}
	if !strings.Contains(err.Error(), "@endwidget") {
		t.Fatalf("error should name @endwidget: %v", err)
	}
}

// TestCustomDirectiveRawOutput documents that Engine.Directive replacers emit raw HTML.
func TestCustomDirectiveRawOutput(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`@evil('x')`), 0o644)
	eng := canvas.New(dir)
	eng.Directive("evil", func(args string) string {
		return `<script>alert(1)</script>`
	})
	out, err := eng.Render("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<script>alert(1)</script>`) {
		t.Fatalf("current: Directive output is raw (not escaped), got %q", out)
	}
}

// TestAddFuncReachableFromTemplate documents that AddFunc registrations are callable in templates.
func TestAddFuncReachableFromTemplate(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`{{ envleak }}`), 0o644)
	eng := canvas.New(dir)
	const marker = "ENVLEAK_SENTINEL_CANVAS"
	eng.AddFunc("envleak", func() string {
		return marker + os.Getenv("PATH")
	})
	out, err := eng.Render("page", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, marker) {
		t.Fatalf("current: registered AddFunc is callable from templates, got %q", out)
	}
	eng2 := canvas.New(dir)
	_, err2 := eng2.Render("page", nil)
	if err2 == nil {
		t.Fatal("without AddFunc, envleak must fail")
	}
}

// TestIncludeRejectsPathTraversal documents that traversal-like @include names
// must error (no read outside the template root).
func TestIncludeRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	tpl := filepath.Join(root, "templates")
	_ = os.MkdirAll(outside, 0o755)
	_ = os.MkdirAll(tpl, 0o755)
	secret := filepath.Join(outside, "secret.html")
	_ = os.WriteFile(secret, []byte(`SECRET_OUTSIDE`), 0o644)
	_ = os.WriteFile(filepath.Join(tpl, "page.html"), []byte(`@include('..outside.secret')`), 0o644)
	_ = os.WriteFile(filepath.Join(tpl, "page2.html"), []byte(`@include('../outside/secret')`), 0o644)

	eng := canvas.New(tpl)
	out, err := eng.Render("page", nil)
	if strings.Contains(out, "SECRET_OUTSIDE") {
		t.Fatalf("include must not leak SECRET_OUTSIDE: err=%v out=%q", err, out)
	}
	out2, err2 := eng.Render("page2", nil)
	if err2 == nil && strings.Contains(out2, "SECRET_OUTSIDE") {
		t.Fatalf("slash include leaked SECRET")
	}
	if strings.Contains(out2, "SECRET_OUTSIDE") {
		t.Fatalf("slash include leaked SECRET: %q", out2)
	}
	// Path-like ../ must fail closed with an error from resolve/read.
	if err2 == nil {
		t.Fatalf("expected error for @include('../outside/secret'), got out=%q", out2)
	}
}

func TestSurface_NestedFieldAccess(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`{{ $user.profile.name }}`), 0o644)
	eng := canvas.New(dir)
	out, err := eng.Render("page", map[string]any{
		"user": map[string]any{"profile": map[string]any{"name": "Ada"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Ada") {
		t.Fatalf("nested map field: got %q", out)
	}
}
