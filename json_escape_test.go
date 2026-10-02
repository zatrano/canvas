package canvas_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
)

func TestJSON_HTMLScriptSafe(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`<script>var a = @json($x);</script>`), 0o644)
	eng := canvas.New(dir)

	cases := []struct {
		name   string
		in     string
		bad    []string
		needle string
	}{
		{"script_break", `</script><script>alert(1)</script>`, []string{`"</script>`, `"<script>`}, `\u003c`},
		{"comment", `<!--`, []string{`<!--`}, `\u003c`},
		{"amp", `a&b`, []string{`"a&b"`}, `\u0026`},
		{"ls", "line\u2028break", []string{"\u2028"}, `\u2028`},
		{"ps", "line\u2029break", []string{"\u2029"}, `\u2029`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := eng.Render("page", map[string]any{"x": tc.in})
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range tc.bad {
				if strings.Contains(out, b) {
					t.Fatalf("@json must HTML/script-escape %q; got %q", b, out)
				}
			}
			if !strings.Contains(out, tc.needle) {
				t.Fatalf("expected %q in output, got %q", tc.needle, out)
			}
		})
	}
}
