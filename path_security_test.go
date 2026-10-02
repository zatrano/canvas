package canvas_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
)

// TestPath_RejectTraversalOS is OS-aware: slash/backslash semantics differ.
func TestPath_RejectTraversalOS(t *testing.T) {
	root := t.TempDir()
	tpl := filepath.Join(root, "templates")
	outside := filepath.Join(root, "outside")
	_ = os.MkdirAll(tpl, 0o755)
	_ = os.MkdirAll(outside, 0o755)
	_ = os.WriteFile(filepath.Join(outside, "secret.html"), []byte(`SECRET_OUTSIDE`), 0o644)
	_ = os.WriteFile(filepath.Join(tpl, "ok.html"), []byte(`OK`), 0o644)

	// Absolute secret with no '.' in the path (old ReplaceAll would still Join-away).
	absDir := filepath.Join(root, "absout")
	_ = os.MkdirAll(absDir, 0o755)
	absBase := filepath.Join(absDir, "secret") // no extension in name
	_ = os.WriteFile(absBase+".html", []byte(`SECRET_ABS`), 0o644)

	eng := canvas.New(tpl)
	out, err := eng.Render("ok", nil)
	if err != nil || out != "OK" {
		t.Fatalf("legitimate template: err=%v out=%q", err, out)
	}

	mustErr := func(t *testing.T, view string) {
		t.Helper()
		out, err := eng.Render(view, nil)
		if strings.Contains(out, "SECRET_") {
			t.Fatalf("Render(%q) leaked outside content: %q", view, out)
		}
		if err == nil {
			t.Fatalf("Render(%q) must error, got out=%q", view, out)
		}
	}

	t.Run("dotdot_slash", func(t *testing.T) { mustErr(t, "../../secret") })
	t.Run("a_dotdot_b", func(t *testing.T) { mustErr(t, "a/../../b") })
	t.Run("nul", func(t *testing.T) { mustErr(t, "foo\x00bar") })
	t.Run("empty", func(t *testing.T) { mustErr(t, "") })
	t.Run("long", func(t *testing.T) { mustErr(t, strings.Repeat("a/", 300)+"x") })
	t.Run("relative_outside", func(t *testing.T) { mustErr(t, "../outside/secret") })
	t.Run("absolute_escape", func(t *testing.T) { mustErr(t, absBase) })

	if runtime.GOOS == "windows" {
		t.Run("win_abs", func(t *testing.T) { mustErr(t, `C:\Windows\win.ini`) })
		t.Run("dotdot_backslash", func(t *testing.T) { mustErr(t, `..\..\x`) })
		t.Run("backslash_outside", func(t *testing.T) { mustErr(t, `..\outside\secret`) })
	} else {
		t.Run("etc_passwd", func(t *testing.T) { mustErr(t, "/etc/passwd") })
		t.Run("unix_backslash_filename_ok", func(t *testing.T) {
			name := `weird\name`
			if err := os.WriteFile(filepath.Join(tpl, name+".html"), []byte(`BACKSLASH_OK`), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := eng.Render(name, nil)
			if err != nil {
				t.Fatalf("unix backslash filename must load: %v", err)
			}
			if out != "BACKSLASH_OK" {
				t.Fatalf("got %q", out)
			}
		})
	}

	_ = os.WriteFile(filepath.Join(tpl, "inc_slash.html"), []byte(`@include('../outside/secret')`), 0o644)
	_ = os.WriteFile(filepath.Join(tpl, "inc_abs.html"), []byte(`@include('`+filepath.ToSlash(absBase)+`')`), 0o644)
	for _, page := range []string{"inc_slash", "inc_abs"} {
		t.Run("include_"+page, func(t *testing.T) {
			out, err := eng.Render(page, nil)
			if strings.Contains(out, "SECRET_") {
				t.Fatalf("%s leaked: %q", page, out)
			}
			if err == nil {
				t.Fatalf("%s: expected path error, got out=%q", page, out)
			}
		})
	}

	t.Run("symlink_outside", func(t *testing.T) {
		link := filepath.Join(tpl, "link_out.html")
		_ = os.Remove(link)
		if err := os.Symlink(filepath.Join(outside, "secret.html"), link); err != nil {
			t.Skipf("symlink not permitted: %v", err)
		}
		out, err := eng.Render("link_out", nil)
		if err == nil && strings.Contains(out, "SECRET_OUTSIDE") {
			t.Log("NOTE: DirFS followed outside symlink (Go<1.24); see SECURITY.md")
		}
	})
}
