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

type renderPath string

const (
	pathAOT    renderPath = "aot"
	pathHTML   renderPath = "html"
	pathLayout renderPath = "layout"
)

func writePath(t testing.TB, dir string, p renderPath, body string) string {
	t.Helper()
	switch p {
	case pathAOT:
		_ = os.WriteFile(filepath.Join(dir, "p_aot.html"), []byte(body), 0o644)
		return "p_aot"
	case pathHTML:
		_ = os.WriteFile(filepath.Join(dir, "p_html.html"), []byte(body), 0o644)
		return "p_html"
	case pathLayout:
		_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`@yield('content')`), 0o644)
		src := "@extends('layouts.app')\n@section('content')\n" + body + "\n@endsection\n"
		_ = os.WriteFile(filepath.Join(dir, "p_layout.html"), []byte(src), 0o644)
		return "p_layout"
	}
	t.Fatalf("bad path")
	return ""
}

func TestOracle_CombinatorialStrict(t *testing.T) {
	if testing.Short() {
		t.Skip("full combinatorial (use -short=false on main/nightly)")
	}
	catalog := oracle.BuildComboCatalog()
	paths := []renderPath{pathAOT, pathHTML, pathLayout}
	payloads := oracle.AttackPayloads

	var (
		tmplN, compileErrN, passN, violateN int
		violations                          []string
	)

	seenTmpl := map[string]struct{}{}
	for _, c := range catalog {
		if _, ok := seenTmpl[c.Tmpl]; ok {
			continue
		}
		seenTmpl[c.Tmpl] = struct{}{}
		tmplN++
		for _, p := range paths {
			for _, payload := range payloads {
				bagAttack := map[string]string{
					"id": "ok", "title": payload, "onclick": payload,
					"href": payload, "srcdoc": payload, "style": payload,
				}
				bagSafe := map[string]string{
					"id": "ok", "title": oracle.HarmlessValue, "onclick": oracle.HarmlessValue,
					"href": oracle.HarmlessValue, "srcdoc": oracle.HarmlessValue, "style": oracle.HarmlessValue,
				}
				data := map[string]any{
					"x": payload, "a": payload, "b": payload, "t": "div", "m": bagAttack,
				}
				safeData := map[string]any{
					"x": oracle.HarmlessValue, "a": oracle.HarmlessValue,
					"b": oracle.HarmlessValue, "t": "div", "m": bagSafe,
				}
				dir := t.TempDir()
				name := writePath(t, dir, p, c.Tmpl)
				eng := canvas.New(dir)
				if p == pathHTML {
					eng.PreferHTMLPath(true)
				}
				eng.SetEscapeMode(rt.EscapeStrict)

				_, errSafe := eng.Render(name, safeData)
				attackOut, errAttack := eng.Render(name, data)

				if errAttack != nil {
					compileErrN++
					continue
				}
				if errSafe != nil {
					// safe failed but attack ok — unusual; treat as violate
					violateN++
					violations = append(violations, fmt.Sprintf("%s/%s safe-err=%v", c.Name, p, errSafe))
					continue
				}
				safeOut, _ := eng.Render(name, safeData)
				findings := oracle.FullCheck(c.Tmpl, safeOut, attackOut)
				if len(findings) > 0 {
					violateN++
					violations = append(violations, fmt.Sprintf("%s/%s/%s findings=%v out=%q",
						c.Name, p, oracle.Normalize(payload), findings, attackOut))
					if len(violations) > 20 {
						t.Fatalf("too many violations (showing 20):\n%s\n… total=%d",
							strings.Join(violations, "\n"), violateN)
					}
					continue
				}
				passN++
			}
		}
	}

	t.Logf("combinatorial: templates=%d compile_errors=%d pass=%d violations=%d",
		tmplN, compileErrN, passN, violateN)
	if violateN != 0 {
		t.Fatalf("violations=%d:\n%s", violateN, strings.Join(violations, "\n"))
	}
}
