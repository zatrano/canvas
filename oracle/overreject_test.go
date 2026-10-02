package oracle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/oracle"
	"github.com/zatrano/canvas/rt"
)

// TestOracle_OverRejectionReport groups combinatorial compile errors by
// (context × position × expr) across 3 render paths × AttackPayloads,
// matching TestOracle_CombinatorialStrict's compile_errors total (~13050).
//
// Compile failure is payload-independent, so each (tmpl, path) is rendered once
// and counted × len(AttackPayloads).
func TestOracle_OverRejectionReport(t *testing.T) {
	if testing.Short() {
		t.Skip("over-rejection report follows full combo catalog")
	}
	catalog := oracle.BuildComboCatalog()
	paths := []renderPath{pathAOT, pathHTML, pathLayout}
	nPay := len(oracle.AttackPayloads)

	type key struct{ ctx, pos, expr string }
	errN := map[key]int{}
	passN := map[key]int{}
	var unexpected []string

	expectedForbid := func(c oracle.ComboCase) bool {
		if strings.HasPrefix(c.Name, "yield-") || strings.HasPrefix(c.Name, "slot-") ||
			strings.HasPrefix(c.Name, "stack-in-script") {
			return true
		}
		if strings.HasPrefix(c.Name, "stack-in-text") || strings.HasPrefix(c.Name, "lang-") ||
			strings.HasPrefix(c.Name, "csrf-") || strings.HasPrefix(c.Name, "yield-default") {
			return false
		}
		switch c.Ctx {
		case oracle.CtxTagName, oracle.CtxAttrName, oracle.CtxBetweenAttrs:
			return c.Expr == oracle.ExprEcho
		case oracle.CtxVerbatimOpenScript, oracle.CtxVerbatimOpenStyle:
			return true
		case oracle.CtxTemplateCommentScript, oracle.CtxAttrsBag:
			return false
		case oracle.CtxUnquoted:
			if c.Expr == oracle.ExprJSON || c.Expr == oracle.ExprJS {
				return true
			}
			if c.Attr == "onclick" || c.Attr == "srcdoc" {
				return true
			}
			if c.Attr == "style" {
				if c.Pos == oracle.PosAfterStaticPrefix || c.Pos == oracle.PosMidValue {
					return false
				}
				return true
			}
		case oracle.CtxDoubleQuoted, oracle.CtxSingleQuoted:
			if c.Attr == "onclick" || c.Attr == "srcdoc" {
				return true
			}
			if c.Attr == "style" {
				if c.Expr == oracle.ExprJSON || c.Expr == oracle.ExprJS {
					return true
				}
				if c.Pos == oracle.PosValueStart || c.Pos == oracle.PosMultiInterp {
					return true
				}
				return false
			}
		}
		return false
	}

	seen := map[string]struct{}{}
	tmplFail, tmplOK := 0, 0
	for _, c := range catalog {
		if _, ok := seen[c.Tmpl]; ok {
			continue
		}
		seen[c.Tmpl] = struct{}{}
		k := key{c.Ctx.String(), c.Pos.String(), c.Expr.String()}
		wantForbid := expectedForbid(c)
		anyErr, anyOK := false, false
		for _, p := range paths {
			dir := t.TempDir()
			name := writePath(t, dir, p, c.Tmpl)
			eng := canvas.New(dir)
			if p == pathHTML {
				eng.PreferHTMLPath(true)
			}
			eng.SetEscapeMode(rt.EscapeStrict)
			_, err := eng.Render(name, map[string]any{
				"x": "1", "a": "1", "b": "1", "t": "div",
			})
			if err != nil {
				errN[k] += nPay
				anyErr = true
			} else {
				passN[k] += nPay
				anyOK = true
			}
		}
		if anyErr && !anyOK {
			tmplFail++
			if !wantForbid {
				unexpected = append(unexpected, fmt.Sprintf("ALWAYS-ERR? %s tmpl=%q", c.Name, c.Tmpl))
			}
		} else if anyOK && !anyErr {
			tmplOK++
			if wantForbid {
				unexpected = append(unexpected, fmt.Sprintf("ALWAYS-OK but expected forbid %s tmpl=%q", c.Name, c.Tmpl))
			}
		} else if anyErr && anyOK {
			unexpected = append(unexpected, fmt.Sprintf("MIXED %s tmpl=%q", c.Name, c.Tmpl))
		}
	}

	var keys []key
	seenK := map[key]struct{}{}
	for k := range errN {
		keys = append(keys, k)
		seenK[k] = struct{}{}
	}
	for k := range passN {
		if _, ok := seenK[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.ctx != b.ctx {
			return a.ctx < b.ctx
		}
		if a.pos != b.pos {
			return a.pos < b.pos
		}
		return a.expr < b.expr
	})

	totalErr, totalOK := 0, 0
	var b strings.Builder
	b.WriteString("# Combinatorial compile-error groups\n\n")
	b.WriteString("Counts are over unique templates × 3 render paths × `AttackPayloads` ")
	b.WriteString("(same cells as `TestOracle_CombinatorialStrict`). ")
	b.WriteString("Compile failure is payload-independent; each (tmpl, path) is counted × payload count.\n\n")
	b.WriteString("| context | position | expr | compile_errors | compiles_ok |\n")
	b.WriteString("|---------|----------|------|----------------:|------------:|\n")
	for _, k := range keys {
		totalErr += errN[k]
		totalOK += passN[k]
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %d |\n", k.ctx, k.pos, k.expr, errN[k], passN[k]))
	}
	b.WriteString(fmt.Sprintf("\nTotal compile_errors: %d\n", totalErr))
	b.WriteString(fmt.Sprintf("Total compiles_ok: %d\n", totalOK))
	b.WriteString(fmt.Sprintf("Unique templates always-forbid: %d, always-ok: %d\n", tmplFail, tmplOK))

	b.WriteString("\n## Expected forbid rules (must match groups above)\n\n")
	b.WriteString("- tag-name / attr-name / between-attrs: `{{ }}` only → forbid\n")
	b.WriteString("- unquoted `@json` / `@js`: always forbid\n")
	b.WriteString("- on* / srcdoc (any quote style, any expr): forbid\n")
	b.WriteString("- style attr whole-value / multi-interp: forbid; after `prop:` → CSS filter (compiles)\n")
	b.WriteString("- style attr `@json`/`@js`: forbid\n")
	b.WriteString("- text / comment / title / textarea: compile OK\n")

	b.WriteString("\n## Migration: common patterns that fail closed\n\n")
	b.WriteString("| Pattern | Why | Supported safe alternative | Fix |\n|---------|-----|----------------------------|-----|\n")
	b.WriteString("| `<div {{ $attrs }}>` (attribute bag) | attribute-name interpolation forbidden | **VAR** (`@attrs($attrs)`) | Use `@attrs($map)` (name regex, reject on*/srcdoc/style/srcset, URL scheme filter). |\n")
	b.WriteString("| `<option {{ $sel }}>` | same | **VAR** (partial: `@selected`) | Prefer `@selected($cond)`. |\n")
	b.WriteString("| `<input {{ $attrs }}>` | same | **VAR** (`@attrs($attrs)`) | `@attrs($map)` and/or boolean directives. |\n")
	b.WriteString("| `<a href=\"{{ $u }}\">` untrusted scheme | URL filter → `#unsafe` | **VAR** | Validate scheme or static `https://` prefix. |\n")
	b.WriteString("| `<div style=\"{{ $x }}\">` | whole style value forbidden | **VAR** | `style=\"color: {{ $c }}\"` or classes. |\n")
	b.WriteString("| Unquoted `style=width:{{ $x }}` / `style=color:{{ $x }};` | spaces split unquoted attrs; now forbid | **VAR** | Quote: `style=\"width: {{ $x }}\"` (EscCSS after `:`) or use classes. |\n")
	b.WriteString("| `<a onclick=\"…\">` / `@js` / `@json` | on* forbidden | **VAR** | `data-*` + JS, or `@js` in `<script>`. |\n")
	b.WriteString("| Unquoted `@json`/`@js` | spaces break attrs | **VAR** | Quote: `data-x=\"@json($x)\"`. |\n")
	b.WriteString("| `<iframe srcdoc=\"…\">` | srcdoc forbidden | **YOK** | Outside templates or intentional SafeHTML. |\n")
	b.WriteString("| `<script>@yield('js')</script>` / `<a href=\"@yield('u')\">` | yield call site in script/attr | **YOK** (fail-closed) | Put `@yield` in text/`<title>` only; pass data via `@js` / attrs. |\n")
	b.WriteString("| `<div title=\"{{ $slot }}\">` | `$slot` in attribute | **YOK** | Keep `{{ $slot }}` in element body text. |\n")
	b.WriteString("| `@lang('k')` with HTML in catalog | trusted in text by default | **VAR** |  Tenant-editable: `SetLangEscapeCatalog(true)`. Attr always escaped; params always escaped. |\n")
	b.WriteString("\n### Still without a full bag helper\n\n")
	b.WriteString("- `<option {{ $sel }}>` beyond `@selected`\n")
	b.WriteString("- `srcdoc` interpolation (intentional fail-closed)\n")
	b.WriteString("\n## Over-rejection delta (unquoted-style forbid: 18180 → 18360 compile errors)\n\n")
	b.WriteString("+180 cells (= 2 templates × 3 paths × 30 payloads). Both are **new unquoted-style forbids**:\n\n")
	b.WriteString("| template | context | position | expr |\n|----------|---------|----------|------|\n")
	b.WriteString("| `<div style=width:{{ $x }}>` | unquoted | after-static | `{{}}` |\n")
	b.WriteString("| `<div style=color:{{ $x }};>` | unquoted | mid-value | `{{}}` |\n\n")
	b.WriteString("Previously these used `EscCSS` and compiled; spaces in CSS tokens could split attributes. **Not** a safe-and-common pattern — fix by quoting the attribute (table above).\n")

	outPath := filepath.Join("..", "docs", "strict-overrejection.md")
	if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
		_ = os.WriteFile(filepath.Join("testdata", "strict-overrejection.md"), []byte(b.String()), 0o644)
		t.Logf("wrote testdata/strict-overrejection.md")
	} else {
		t.Logf("wrote %s", outPath)
	}
	t.Logf("compile_errors=%d compiles_ok=%d groups=%d unexpected=%d", totalErr, totalOK, len(keys), len(unexpected))
	if totalErr != 18360 {
		t.Logf("NOTE: compile_errors=%d (expected 18360 after unquoted-style forbid; check drift)", totalErr)
	}
	if len(unexpected) > 40 {
		unexpected = unexpected[:40]
	}
	for _, u := range unexpected {
		t.Log(u)
	}
}
