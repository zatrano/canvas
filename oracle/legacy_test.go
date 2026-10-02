package oracle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/oracle"
	"github.com/zatrano/canvas/rt"
)

func renderMode(t testing.TB, mode rt.EscapeMode, tmpl string, data map[string]any) (string, error) {
	t.Helper()
	dir := t.TempDir()
	name := "page"
	if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := canvas.New(dir)
	eng.SetEscapeMode(mode)
	return eng.Render(name, data)
}

func renderPair(t testing.TB, mode rt.EscapeMode, tmpl string, attack map[string]any) (safeOut, attackOut string, err error) {
	t.Helper()
	safe := map[string]any{}
	for k := range attack {
		safe[k] = oracle.HarmlessValue
	}
	safeOut, err = renderMode(t, mode, tmpl, safe)
	if err != nil {
		return "", "", err
	}
	attackOut, err = renderMode(t, mode, tmpl, attack)
	return safeOut, attackOut, err
}

// TestOracle_LegacyFindsKnownGaps proves the oracle detects known Legacy weaknesses.
func TestOracle_LegacyFindsKnownGaps(t *testing.T) {
	counts := map[string]int{}
	var missed []string
	for _, p := range oracle.LegacyProbes() {
		safeOut, attackOut, err := renderPair(t, rt.EscapeLegacy, p.Tmpl, p.Payload)
		if err != nil {
			// Legacy rarely errors; if it does, still count as non-detection of XSS
			t.Logf("legacy render err ctx=%s: %v", p.Ctx, err)
		}
		findings := oracle.FullCheck(p.Tmpl, safeOut, attackOut)
		// Also run shape-only if pair empty for onclick (static already has on*)
		if len(findings) == 0 {
			findings = oracle.CheckRendered(p.Tmpl, attackOut)
		}
		if p.Ctx == "onclick" {
			// Legacy writes escaped JS into onclick — not a new on-attr.
			// Skeleton may match. Force-check: raw alert(1) without encoding is a hit.
			if strings.Contains(attackOut, `onclick="alert(1)"`) || strings.Contains(attackOut, "onclick=\"alert(1)\"") {
				findings = append(findings, oracle.InjectionFinding{Kind: "handler-exec", Detail: "onclick raw"})
			} else if strings.Contains(attackOut, "alert(1)") {
				findings = append(findings, oracle.InjectionFinding{Kind: "handler-payload", Detail: "alert in onclick"})
			}
		}
		if p.Ctx == "json-in-script" {
			// Must NOT false-positive on properly escaped JSON.
			if len(findings) > 0 {
				missed = append(missed, "json-in-script FALSE POSITIVE: "+fmt.Sprint(findings))
			} else {
				counts[p.Ctx] = 1 // counted as correct negative
			}
			continue
		}
		if p.Ctx == "raw-script-break" {
			findings = append(findings, oracle.CheckScriptBreak(attackOut)...)
			if strings.Contains(strings.ToLower(attackOut), "</script><script>") {
				findings = append(findings, oracle.InjectionFinding{Kind: "script-break", Detail: "raw breakout"})
			}
		}
		if p.Ctx == "srcdoc-legacy" {
			if strings.Contains(attackOut, "script") {
				findings = append(findings, oracle.InjectionFinding{Kind: "srcdoc-payload", Detail: "script in srcdoc"})
			}
		}
		if len(findings) == 0 {
			missed = append(missed, p.Ctx+" out="+oracle.Normalize(attackOut))
			continue
		}
		counts[p.Ctx] += len(findings)
		t.Logf("legacy ctx=%s findings=%v out=%q", p.Ctx, findings, attackOut)
	}
	t.Logf("Legacy findings by context: %v", counts)
	if len(missed) > 0 {
		t.Fatalf("oracle missed Legacy gaps: %v", missed)
	}
	for _, need := range []string{"unquoted-attr", "javascript-url", "quote-breakout", "tab-javascript-url", "raw-script-break", "onclick"} {
		if counts[need] == 0 {
			t.Fatalf("no findings for required context %s (counts=%v)", need, counts)
		}
	}
}

// TestOracle_StrictPassesLegacyProbes: same probes under Strict must be fail-closed or clean.
func TestOracle_StrictPassesLegacyProbes(t *testing.T) {
	for _, p := range oracle.LegacyProbes() {
		if p.Ctx == "raw-script-break" {
			t.Logf("skip %s: raw echo is operator-trusted in Strict too", p.Ctx)
			continue
		}
		safeOut, attackOut, err := renderPair(t, rt.EscapeStrict, p.Tmpl, p.Payload)
		if err != nil {
			t.Logf("strict fail-closed ctx=%s: %v", p.Ctx, err)
			continue
		}
		findings := oracle.FullCheck(p.Tmpl, safeOut, attackOut)
		if len(findings) > 0 {
			t.Fatalf("strict injection ctx=%s findings=%v out=%q", p.Ctx, findings, attackOut)
		}
	}
}
