//go:build mutation

package oracle_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMutationGenerateResults mutates sources in a temp tree and records which
// regression subtest fails. Regenerate:
//
//	go test -C oracle -tags=mutation -run TestMutationGenerateResults -count=1 -timeout 40m
func TestMutationGenerateResults(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	type mut struct {
		rule     string
		file     string
		old, new string
		oldRe    string
		runs     []string
		pkgRoot  bool
		note     string
	}
	muts := []mut{
		{rule: "url-scheme", old: `return "#unsafe"`, new: `return htmlEscapeString(trimmed)`,
			runs: []string{"TestMutationCoverage_Rules/url-scheme"}},
		{rule: "unquoted-attr-spaces",
			oldRe: `(?s)func EscapeUnquotedAttr\(s string\) string \{.*?^\}`,
			new:   "func EscapeUnquotedAttr(s string) string { return s }",
			runs:  []string{"TestMutationCoverage_Rules/unquoted-space"}},
		{rule: "on-forbid", oldRe: `strings\.HasPrefix\(attr, "on"\) \|\| forbidAttrs\[attr\]`, new: `false /*mut-on*/`,
			runs: []string{"TestMutationCoverage_Rules/on-forbid"}},
		{rule: "srcdoc-forbid", old: `"srcdoc": true`, new: `"srcdoc": false`,
			runs: []string{"TestMutationCoverage_Rules/srcdoc-forbid"}},
		{rule: "multi-interp-value-start",
			old:  "\tif safeURLStaticLock(valuePrefix) {\n\t\treturn EscHTML\n\t}\n\treturn EscURLBlock",
			new:  "\tif safeURLStaticLock(valuePrefix) {\n\t\treturn EscHTML\n\t}\n\treturn EscHTML /*mut-urlblock*/",
			runs: []string{"TestMutationCoverage_Rules/multi-interp-url"}},
		{rule: "css-value-filter",
			oldRe: `(?s)func EscapeCSSValue\(s string\) string \{.*?^\}`,
			new:   "func EscapeCSSValue(s string) string { return s }",
			runs:  []string{"TestMutationCoverage_Rules/css-value-filter"}},
		{rule: "script-echo-forbid", old: "\t\treturn EscForbid\n\tcase ctxStyle:", new: "\t\treturn EscHTML /*mut-script*/\n\tcase ctxStyle:",
			runs: []string{"TestStrict_ExpectedCompileErrors/aot/script_echo_bare"}, pkgRoot: true},
		{rule: "style-element-forbid", old: "\t\t// Style element: no {{ }}, @json, or @js (CSS-in-JSON is not a safe subset).\n\t\treturn EscForbid",
			new:  "\t\treturn EscHTML /*mut-style-el*/",
			runs: []string{"TestStrict_ExpectedCompileErrors/aot/style_el_echo"}, pkgRoot: true},
		{rule: "style-whole-value-forbid",
			oldRe: `(?s)func styleAttrKind\(atValueStart bool, valuePrefix string\) EscapeKind \{.*?^\}`,
			new:   "func styleAttrKind(atValueStart bool, valuePrefix string) EscapeKind { return EscHTML }",
			runs:  []string{"TestStrict_ExpectedCompileErrors/aot/style_attr_whole"}, pkgRoot: true},
		{rule: "tag-name-forbid", old: "case ctxTagOpen, ctxAttrName:\n\t\treturn EscForbid",
			new:  "case ctxTagOpen, ctxAttrName:\n\t\treturn EscHTML /*mut-tag*/",
			runs: []string{"TestMutationCoverage_Rules/tag-name-forbid"}},
		{rule: "unquoted-json-forbid",
			old:  "if strings.HasPrefix(attr, \"on\") || forbidAttrs[attr] || attr == \"style\" || jsonDirective {",
			new:  "if strings.HasPrefix(attr, \"on\") || forbidAttrs[attr] || attr == \"style\" || false /*mut-uq-json*/ {",
			runs: []string{"TestMutationCoverage_Rules/unquoted-json-forbid"}},
		{rule: "json-script-escape", old: "\t\tif jsonDirective {\n\t\t\treturn EscJSON\n\t\t}\n\t\treturn EscForbid\n\tcase ctxStyle:",
			new:  "\t\tif jsonDirective {\n\t\t\treturn EscHTML /*mut-json*/\n\t\t}\n\t\treturn EscForbid\n\tcase ctxStyle:",
			runs: []string{"TestMutationCoverage_Rules/json-script-escape"}},

		// @attrs — one mutation → one unique subtest
		{rule: "attrs-on-reject", file: "rt/attrs.go",
			old:  "\tif strings.HasPrefix(lower, \"on\") {\n\t\treturn false\n\t}",
			new:  "\tif false /*mut-attrs-on*/ && strings.HasPrefix(lower, \"on\") {\n\t\treturn false\n\t}",
			runs: []string{"TestAttrs_Strict/aot/on_reject"}, pkgRoot: true},
		{rule: "attrs-position", file: "rt/escape_ctx.go",
			old:  "func AttrsPositionOK(staticBefore string) (ok bool, context string) {\n\tst, attr, _, _ := scanHTML(staticBefore)",
			new:  "func AttrsPositionOK(staticBefore string) (ok bool, context string) {\n\treturn true, \"attribute\" /*mut-attrs-pos*/\n\tst, attr, _, _ := scanHTML(staticBefore)",
			runs: []string{"TestAttrs_Strict/aot/position"}, pkgRoot: true},
		{rule: "attrs-url-scheme", file: "rt/attrs.go",
			old:  "\t\tif attrsURLNames[lower] {\n\t\t\tval = EscapeURLAttr(val, true)\n\t\t} else {\n\t\t\tval = html.EscapeString(val)\n\t\t}",
			new:  "\t\tif attrsURLNames[lower] {\n\t\t\tval = val /*mut-attrs-url*/\n\t\t} else {\n\t\t\tval = html.EscapeString(val)\n\t\t}",
			runs: []string{"TestAttrs_Strict/aot/url_scheme"}, pkgRoot: true},
		{rule: "attrs-name-regex", file: "rt/attrs.go",
			old:  "var attrNameOK = regexp.MustCompile(`^[A-Za-z_:][-A-Za-z0-9_:.]*$`)",
			new:  "var attrNameOK = regexp.MustCompile(`(?s)^.*$`) /*mut-attrs-name*/",
			runs: []string{"TestAttrs_Strict/aot/name_regex"}, pkgRoot: true},
		{rule: "attrs-srcdoc-style-reject", file: "rt/attrs.go",
			old:  "\tcase \"srcdoc\", \"style\", \"srcset\":\n\t\treturn false",
			new:  "\tcase \"srcdoc\", \"style\", \"srcset\":\n\t\treturn true /*mut-attrs-srcdoc*/",
			runs: []string{"TestAttrs_Strict/aot/srcdoc_style_reject"}, pkgRoot: true},
		{rule: "attrs-value-escape", file: "rt/attrs.go",
			old:  "\t\t\tval = html.EscapeString(val)",
			new:  "\t\t\tval = val; _ = html.EscapeString /*mut-attrs-esc*/",
			runs: []string{"TestAttrs_Strict/aot/value_escape"}, pkgRoot: true},
		{rule: "attrs-safehtml-not-raw", file: "rt/attrs.go",
			old:  "\t\tval := stringifyAttrValue(p.Value)\n\t\tif attrsURLNames[lower] {\n\t\t\tval = EscapeURLAttr(val, true)\n\t\t} else {\n\t\t\tval = html.EscapeString(val)\n\t\t}",
			new:  "\t\tval := stringifyAttrValue(p.Value)\n\t\tif _, ok := p.Value.(SafeHTML); ok {\n\t\t\t/*mut-attrs-safehtml: raw*/\n\t\t} else if attrsURLNames[lower] {\n\t\t\tval = EscapeURLAttr(val, true)\n\t\t} else {\n\t\t\tval = html.EscapeString(val)\n\t\t}",
			runs: []string{"TestAttrs_Strict/aot/safehtml_not_raw"}, pkgRoot: true},

		// Layout placement — directive × context (+ root reject).
		// Each mutation inserts a unique early-return so only that cell's subtest breaks.
		{rule: "layout-yield-script", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && ctx == \"script\" {\n\t\t\treturn nil /*mut-yield-script*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_in_script"}, pkgRoot: true},
		{rule: "layout-yield-style", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && ctx == \"style\" {\n\t\t\treturn nil /*mut-yield-style*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_in_style"}, pkgRoot: true},
		{rule: "layout-yield-tag", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && (ctx == \"tag\" || ctx == \"tag-name\") {\n\t\t\treturn nil /*mut-yield-tag*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_in_tag"}, pkgRoot: true},
		{rule: "layout-yield-attr-dq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && strings.HasPrefix(ctx, \"attribute:href\") {\n\t\t\treturn nil /*mut-yield-attr-dq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_in_href"}, pkgRoot: true},
		{rule: "layout-yield-attr-sq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"href='\") {\n\t\t\treturn nil /*mut-yield-attr-sq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_in_href_sq"}, pkgRoot: true},
		{rule: "layout-yield-attr-uq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"class=\") && !strings.Contains(content[:loc[0]], \"class=\\\"\") {\n\t\t\treturn nil /*mut-yield-attr-uq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_unquoted_attr"}, pkgRoot: true},
		{rule: "layout-yield-between-attrs", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && (ctx == \"attribute\" || ctx == \"attribute-name\") {\n\t\t\treturn nil /*mut-yield-between*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_between_attrs"}, pkgRoot: true},
		{rule: "layout-yield-comment", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"yield\") && ctx == \"comment\" {\n\t\t\treturn nil /*mut-yield-comment*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/yield_in_comment"}, pkgRoot: true},

		{rule: "layout-stack-script", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && ctx == \"script\" {\n\t\t\treturn nil /*mut-stack-script*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_in_script_forbid"}, pkgRoot: true},
		{rule: "layout-stack-style", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && ctx == \"style\" {\n\t\t\treturn nil /*mut-stack-style*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_in_style_forbid"}, pkgRoot: true},
		{rule: "layout-stack-tag", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && (ctx == \"tag\" || ctx == \"tag-name\") {\n\t\t\treturn nil /*mut-stack-tag*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_in_tag"}, pkgRoot: true},
		{rule: "layout-stack-attr-dq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && strings.HasPrefix(ctx, \"attribute:href\") && strings.Contains(content[:loc[0]], \"href=\\\"\") {\n\t\t\treturn nil /*mut-stack-attr-dq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_in_href_forbid"}, pkgRoot: true},
		{rule: "layout-stack-attr-sq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"href='\") {\n\t\t\treturn nil /*mut-stack-attr-sq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_in_href_sq"}, pkgRoot: true},
		{rule: "layout-stack-attr-uq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"class=\") {\n\t\t\treturn nil /*mut-stack-attr-uq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_unquoted_attr"}, pkgRoot: true},
		{rule: "layout-stack-between-attrs", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && (ctx == \"attribute\" || ctx == \"attribute-name\") {\n\t\t\treturn nil /*mut-stack-between*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_between_attrs"}, pkgRoot: true},
		{rule: "layout-stack-comment", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"stack\") && ctx == \"comment\" {\n\t\t\treturn nil /*mut-stack-comment*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/stack_in_comment"}, pkgRoot: true},

		{rule: "layout-slot-script", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && ctx == \"script\" {\n\t\t\treturn nil /*mut-slot-script*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_in_script"}, pkgRoot: true},
		{rule: "layout-slot-style", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && ctx == \"style\" {\n\t\t\treturn nil /*mut-slot-style*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_in_style"}, pkgRoot: true},
		{rule: "layout-slot-tag", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && (ctx == \"tag\" || ctx == \"tag-name\") {\n\t\t\treturn nil /*mut-slot-tag*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_in_tag"}, pkgRoot: true},
		{rule: "layout-slot-attr-dq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && strings.HasPrefix(ctx, \"attribute:title\") {\n\t\t\treturn nil /*mut-slot-attr-dq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_in_title_attr"}, pkgRoot: true},
		{rule: "layout-slot-attr-sq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"href='\") {\n\t\t\treturn nil /*mut-slot-attr-sq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_in_href_sq"}, pkgRoot: true},
		{rule: "layout-slot-attr-uq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"class=\") {\n\t\t\treturn nil /*mut-slot-attr-uq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_unquoted_attr"}, pkgRoot: true},
		{rule: "layout-slot-between-attrs", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && (ctx == \"attribute\" || ctx == \"attribute-name\" || ctx == \"attribute-value\") {\n\t\t\treturn nil /*mut-slot-between*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_between_attrs"}, pkgRoot: true},
		{rule: "layout-slot-comment", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(content[loc[0]:], \"$slot\") && ctx == \"comment\" {\n\t\t\treturn nil /*mut-slot-comment*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/slot_in_comment"}, pkgRoot: true},

		{rule: "layout-include-script", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && ctx == \"script\" {\n\t\t\treturn nil /*mut-include-script*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_in_script_forbid"}, pkgRoot: true},
		{rule: "layout-include-style", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && ctx == \"style\" {\n\t\t\treturn nil /*mut-include-style*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_in_style"}, pkgRoot: true},
		{rule: "layout-include-tag", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && (ctx == \"tag\" || ctx == \"tag-name\") {\n\t\t\treturn nil /*mut-include-tag*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_in_tag"}, pkgRoot: true},
		{rule: "layout-include-attr-dq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && strings.HasPrefix(ctx, \"attribute:href\") && strings.Contains(content[:loc[0]], \"href=\\\"\") {\n\t\t\treturn nil /*mut-include-attr-dq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_in_href"}, pkgRoot: true},
		{rule: "layout-include-attr-sq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"href='\") {\n\t\t\treturn nil /*mut-include-attr-sq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_in_href_sq"}, pkgRoot: true},
		{rule: "layout-include-attr-uq", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && strings.Contains(ctx, \"attribute\") && strings.Contains(content[:loc[0]], \"class=\") {\n\t\t\treturn nil /*mut-include-attr-uq*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_unquoted_attr"}, pkgRoot: true},
		{rule: "layout-include-between-attrs", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && (ctx == \"attribute\" || ctx == \"attribute-name\") {\n\t\t\treturn nil /*mut-include-between*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_between_attrs"}, pkgRoot: true},
		{rule: "layout-include-comment", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"include\") && ctx == \"comment\" && !strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"includeif\") && !strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"includewhen\") {\n\t\t\treturn nil /*mut-include-comment*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/include_in_comment"}, pkgRoot: true},
		{rule: "layout-includeWhen-script", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"includewhen\") && ctx == \"script\" {\n\t\t\treturn nil /*mut-includewhen-script*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/includeWhen_in_script"}, pkgRoot: true},
		{rule: "layout-includeIf-comment", file: "layouts.go",
			old:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}",
			new:  "\t\tif !placementContextForbidden(kind, ctx) {\n\t\t\treturn nil\n\t\t}\n\t\tif strings.Contains(strings.ToLower(content[loc[0]:loc[1]]), \"includeif\") && ctx == \"comment\" {\n\t\t\treturn nil /*mut-includeif-comment*/\n\t\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/includeIf_in_comment"}, pkgRoot: true},
		{rule: "layout-root-reject", file: "parent.go",
			old:  "\tif e.escapeMode == rt.EscapeStrict {\n\t\tif err := rejectLayoutDirectivesInForbiddenContexts(name, content); err != nil {\n\t\t\treturn \"\", err\n\t\t}\n\t}",
			new:  "\tif false && e.escapeMode == rt.EscapeStrict {\n\t\tif err := rejectLayoutDirectivesInForbiddenContexts(name, content); err != nil {\n\t\t\treturn \"\", err\n\t\t}\n\t}",
			runs: []string{"TestLayoutPlacement_StrictForbiddenCallSites/aot/root_layout_href_reject"}, pkgRoot: true},
		{rule: "lang-param-escape", file: "rt/lang.go",
			old:  "\t\tesc := html.EscapeString(fmt.Sprint(repl[k]))\n\t\tmsg = strings.ReplaceAll(msg, \":\"+k, esc)\n\t\tmsg = strings.ReplaceAll(msg, \"{\"+k+\"}\", esc)",
			new:  "\t\t_ = html.EscapeString\n\t\traw := fmt.Sprint(repl[k]) /*mut-lang-param-raw*/\n\t\tmsg = strings.ReplaceAll(msg, \":\"+k, raw)\n\t\tmsg = strings.ReplaceAll(msg, \"{\"+k+\"}\", raw)",
			runs: []string{"TestEscapeSurface_LangParamEscape/html/strict/text"}, pkgRoot: true},
		{rule: "lang-catalog-trusted", file: "rt/lang.go",
			old:  "func LangTrustedText(ctxName string, escapeCatalog bool) bool {\n\treturn !escapeCatalog && ctxName == \"text\"\n}",
			new:  "func LangTrustedText(ctxName string, escapeCatalog bool) bool {\n\treturn false /*mut-lang-catalog-never-trusted*/\n}",
			runs: []string{"TestEscapeSurface_LangCatalogTrusted/aot/text_raw"}, pkgRoot: true},
		{rule: "lang-catalog-escape-opt", file: "rt/lang.go",
			old:  "func LangTrustedText(ctxName string, escapeCatalog bool) bool {\n\treturn !escapeCatalog && ctxName == \"text\"\n}",
			new:  "func LangTrustedText(ctxName string, escapeCatalog bool) bool {\n\t_ = escapeCatalog\n\treturn ctxName == \"text\" /*mut-lang-escape-opt-ignored*/\n}",
			runs: []string{"TestEscapeSurface_LangEscapeCatalogOptIn/aot"}, pkgRoot: true},
		{rule: "lang-attr-context", file: "rt/lang.go",
			old:  "func WriteLang(w *Writer, msg string, kind EscapeKind, ctxName string, escapeCatalog bool) {\n\tif LangTrustedText(ctxName, escapeCatalog) && kind == EscHTML {\n\t\tw.WriteString(msg)\n\t\treturn\n\t}\n\twriteByKind(w, msg, kind)\n}",
			new:  "func WriteLang(w *Writer, msg string, kind EscapeKind, ctxName string, escapeCatalog bool) {\n\t_ = kind\n\t_ = escapeCatalog\n\tw.WriteString(msg) /*mut-lang-attr-raw*/\n}",
			runs: []string{"TestEscapeSurface_LangCatalogTrusted/aot/attr_escaped"}, pkgRoot: true},
		{rule: "yield-default-var-escape", file: "layouts.go",
			old:  "\t\t// Keep {{ $var }} so compile applies contextual escape.\n\t\treturn \"{{ $\" + match[2] + \" }}\"",
			new:  "\t\t// mut: raw echo so title breakout is possible\n\t\treturn \"{!! $\" + match[2] + \" !!}\"",
			runs: []string{"TestEscapeSurface_YieldDefaultVar/aot"}, pkgRoot: true},
		{rule: "unknown-end-directive-error", file: "unknown_end.go",
			old:  "\tif mode != rt.EscapeStrict {\n\t\treturn nil\n\t}",
			new:  "\tif true /*mut-unknown-end*/ || mode != rt.EscapeStrict {\n\t\treturn nil\n\t}",
			runs: []string{"TestUnknownClosingDirective_Strict/aot/endfoo"}, pkgRoot: true},
	}

	type row struct {
		Rule         string `json:"rule"`
		BreakingTest string `json:"breaking_test"`
		Note         string `json:"note,omitempty"`
	}
	var rows []row

	for _, m := range muts {
		m := m
		t.Run(m.rule, func(t *testing.T) {
			wt := t.TempDir()
			if err := syncTree(root, wt); err != nil {
				t.Fatal(err)
			}
			target := m.file
			if target == "" {
				target = "rt/escape_ctx.go"
			}
			esc := filepath.Join(wt, filepath.FromSlash(target))
			raw, err := os.ReadFile(esc)
			if err != nil {
				t.Fatal(err)
			}
			s := string(raw)
			var out string
			if m.oldRe != "" {
				re := regexp.MustCompile(`(?m)` + m.oldRe)
				out = re.ReplaceAllString(s, m.new)
			} else {
				if !strings.Contains(s, m.old) {
					t.Fatalf("needle not found for %s in %s", m.rule, target)
				}
				out = strings.Replace(s, m.old, m.new, 1)
			}
			if out == s {
				t.Fatalf("mutation %s made no change", m.rule)
			}
			if err := os.WriteFile(esc, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}

			broke := ""
			for _, run := range m.runs {
				dir := filepath.Join(wt, "oracle")
				if m.pkgRoot || strings.HasPrefix(run, "TestStrict") || strings.HasPrefix(run, "TestEscape") ||
					strings.HasPrefix(run, "TestSafe") || strings.HasPrefix(run, "TestSlot") ||
					strings.HasPrefix(run, "TestAttrs") || strings.HasPrefix(run, "TestLayout") {
					dir = wt
				}
				cmd := exec.Command("go", "test", "-C", dir, "-count=1", "-timeout", "90s", "-run", "^"+regexp.QuoteMeta(run)+"$")
				b, err := cmd.CombinedOutput()
				txt := string(b)
				buildFail := strings.Contains(txt, "build failed") || strings.Contains(txt, "undefined:")
				testFail := err != nil && (strings.Contains(txt, "--- FAIL:") || strings.Contains(txt, "FAIL\t"))
				t.Logf("run=%s buildFail=%v testFail=%v err=%v", run, buildFail, testFail, err)
				if buildFail {
					t.Fatalf("mutated tree does not build:\n%s", truncate(txt, 500))
				}
				if testFail {
					broke = run
					fails := regexp.MustCompile(`--- FAIL: ([^\s]+)`).FindAllStringSubmatch(txt, -1)
					for i := len(fails) - 1; i >= 0; i-- {
						name := fails[i][1]
						if name == run || strings.HasPrefix(name, run+"/") || strings.HasPrefix(run, name+"/") {
							broke = name
							break
						}
					}
					break
				}
			}
			if broke == "" {
				t.Fatalf("mutation %s broke no tests", m.rule)
			}
			rows = append(rows, row{Rule: m.rule, BreakingTest: broke, Note: m.note})
		})
	}

	extras := []row{
		{Rule: "attr-name-forbid", BreakingTest: "TestStrict_ExpectedCompileErrors/aot/attr_name"},
		{Rule: "between-attrs-forbid", BreakingTest: "TestStrict_ExpectedCompileErrors/aot/between_attrs"},
		{Rule: "include-in-script", BreakingTest: "TestStrict_ExpectedCompileErrors/aot/include_in_script"},
		{Rule: "json-attr-escape", BreakingTest: "TestMutationCoverage_Rules/json-attr-escape"},
		{Rule: "js-in-script", BreakingTest: "TestStrict_ExpectedCompileErrors/aot/script_js_allowed"},
		{Rule: "SafeHTML-only-raw", BreakingTest: "TestSafeHTML_AndTemplateHTMLTrusted"},
		{Rule: "Go-string-slot-escape", BreakingTest: "TestSlot_TemplateBodyRaw_GoStringEscaped"},
		{Rule: "unquoted-per-char", BreakingTest: "TestEscapeUnquotedAttr_PerChar"},
	}
	rows = append(rows, extras...)

	path := filepath.Join(root, "oracle", "mutation_results.json")
	b, _ := json.MarshalIndent(rows, "", "  ")
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d rules)", path, len(rows))
}

func truncate(s string, n int) string {
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func syncTree(src, dst string) error {
	skip := map[string]bool{".git": true}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		for s := range skip {
			if rel == s || strings.HasPrefix(rel, s+"/") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if strings.Contains(rel, "testdata/fuzz") || strings.HasPrefix(rel, "audit_") ||
			strings.HasSuffix(rel, "_out.txt") || rel == "AUDIT_REPORT.md" ||
			strings.HasPrefix(rel, "tmpcount") || strings.HasPrefix(rel, "bench/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
