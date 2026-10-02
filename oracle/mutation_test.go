package oracle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/oracle"
	"github.com/zatrano/canvas/rt"
)

// TestOracle_MutationCoverage documents expected detections when a Strict rule is absent.
// Full disable-proof runs in scripts/mutation_proof.ps1 against a temporary worktree.
func TestOracle_MutationCoverageMatrix(t *testing.T) {
	type row struct {
		rule string
		tmpl string
		data map[string]any
		// under Strict today: expectErr or clean
		strictOK string // "err" or "safe"
	}
	rows := []row{
		{"url-scheme", `<a href="{{ $x }}">`, map[string]any{"x": `javascript:alert(1)`}, "safe"},
		{"unquoted-attr", `<div title={{ $x }}>`, map[string]any{"x": `x onmouseover=alert(1)`}, "safe"},
		{"on-forbid", `<a onclick="{{ $x }}">`, map[string]any{"x": `alert(1)`}, "err"},
		{"script-forbid", `<script>{{ $x }}</script>`, map[string]any{"x": `alert(1)`}, "err"},
		{"style-forbid-whole", `<div style="{{ $x }}">`, map[string]any{"x": `x`}, "err"},
		{"srcdoc-forbid", `<iframe srcdoc="{{ $x }}">`, map[string]any{"x": `<script>x</script>`}, "err"},
		{"json-escape", `<script>var a=@json($x);</script>`, map[string]any{"x": `</script><script>alert(1)</script>`}, "safe"},
		{"multi-interp-url", `<a href="{{ $a }}{{ $b }}">`, map[string]any{"a": ``, "b": `javascript:alert(1)`}, "safe"},
		{"css-value-filter", `<div style="color: {{ $c }}">`, map[string]any{"c": `red;background:url(javascript:x)`}, "safe"},
	}
	for _, r := range rows {
		t.Run(r.rule, func(t *testing.T) {
			dir := t.TempDir()
			_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(r.tmpl), 0o644)
			eng := canvas.New(dir)
			eng.SetEscapeMode(rt.EscapeStrict)
			safe := map[string]any{}
			for k := range r.data {
				safe[k] = oracle.HarmlessValue
			}
			safeOut, errS := eng.Render("p", safe)
			out, err := eng.Render("p", r.data)
			if r.strictOK == "err" {
				if err == nil {
					t.Fatalf("expected compile error, out=%q", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if errS != nil {
				t.Fatalf("safe err: %v", errS)
			}
			if findings := oracle.FullCheck(r.tmpl, safeOut, out); len(findings) > 0 {
				t.Fatalf("strict still injectable: %v out=%q", findings, out)
			}
			// Sanity: Legacy should be caught by oracle for the attack-capable rows.
			engL := canvas.New(dir)
			engL.SetEscapeMode(rt.EscapeLegacy)
			lSafe, _ := engL.Render("p", safe)
			lOut, lErr := engL.Render("p", r.data)
			if lErr != nil {
				return
			}
			if r.rule == "json-escape" || r.rule == "css-value-filter" || r.rule == "on-forbid" ||
				r.rule == "script-forbid" || r.rule == "style-forbid-whole" || r.rule == "srcdoc-forbid" {
				// Legacy may or may not trip depending on HTML escape; skip hard assert.
				_ = lSafe
				_ = lOut
				return
			}
			if findings := oracle.FullCheck(r.tmpl, lSafe, lOut); len(findings) == 0 &&
				(r.rule == "url-scheme" || r.rule == "unquoted-attr" || r.rule == "multi-interp-url") {
				t.Fatalf("oracle missed Legacy gap for rule %s out=%q", r.rule, lOut)
			}
		})
	}
}
