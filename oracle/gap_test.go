package oracle_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/oracle"
)

// TestOracle_DocScriptHTMLEscapeNeutralization documents (not a product bug) that
// if script-body EscForbid were weakened only to EscHTML, `</script>` breakout is
// neutralized by HTML escaping (&lt;/script&gt;). Executable injection then requires
// a further mutation (raw write). Fail-closed compile errors for {{ }} in <script>
// are asserted in canvas_test.TestStrict_ExpectedCompileErrors — that table is what
// prevents relaxing the rule silently.
func TestOracle_DocScriptHTMLEscapeNeutralization(t *testing.T) {
	tmpl := `<script>{{ $x }}</script>`
	rendered := `<script>&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;</script>`
	f := oracle.FullCheck(tmpl, `<script>safeok</script>`, rendered)
	f = append(f, oracle.CheckScriptBreak(rendered)...)
	if len(f) > 0 {
		t.Fatalf("doc case: HTML-escaped script body should not look like injection, got %v", f)
	}
	if !strings.Contains(rendered, "&lt;/script") {
		t.Fatal("fixture must stay HTML-escaped")
	}
}

func TestOracle_CSSUnsafeDetection(t *testing.T) {
	tmpl := `<div style="color: {{ $c }}">`
	attack := `<div style="color: red;background:url(javascript:x)">`
	f := oracle.CheckRendered(tmpl, attack)
	if len(f) == 0 {
		t.Fatal("expected unsafe-css finding")
	}
	t.Log(f)
}
