package oracle

import (
	"fmt"
	"strings"
)

// Template fragments for generative fuzzing.
var (
	tagNames  = []string{"div", "a", "p", "span", "img", "form", "button", "textarea", "title"}
	attrNames = []string{"title", "class", "id", "href", "src", "action", "data-x", "onclick", "style", "srcdoc"}
	quotes    = []string{`"`, `'`, ``}
)

// GenTemplate builds a template from a seed byte slice.
func GenTemplate(data []byte) string {
	if len(data) == 0 {
		data = []byte{0}
	}
	pick := func(i int, n int) int {
		if n <= 0 {
			return 0
		}
		return int(data[i%len(data)]) % n
	}
	kind := pick(0, 10)
	tag := tagNames[pick(1, len(tagNames))]
	attr := attrNames[pick(2, len(attrNames))]
	q := quotes[pick(3, len(quotes))]

	switch kind {
	case 0:
		return fmt.Sprintf(`<%s>%s</%s>`, tag, "{{ $x }}", tag)
	case 1:
		if q == "" {
			return fmt.Sprintf(`<%s %s={{ $x }}>`, tag, attr)
		}
		return fmt.Sprintf(`<%s %s=%s{{ $x }}%s>`, tag, attr, q, q)
	case 2:
		return fmt.Sprintf(`<a href=%s{{ $x }}%s>`, qOr(q, `"`), qOr(q, `"`))
	case 3:
		return `<script>{{ $x }}</script>`
	case 4:
		return `<style>{{ $x }}</style>`
	case 5:
		return fmt.Sprintf(`<%s onclick=%s{{ $x }}%s>`, tag, qOr(q, `"`), qOr(q, `"`))
	case 6:
		return `<div>@json($x)</div>`
	case 7:
		return `<p>@js($x)</p>`
	case 8:
		return `@verbatim <script> @endverbatim {{ $x }}`
	case 9:
		return `<div class="x" @attrs($m)>`
	default:
		return `<p>{{ $x }}</p>`
	}
}

func qOr(q, def string) string {
	if q == "" {
		return def
	}
	return q
}

// GenTemplateExt adds include-point and multi-interp variants.
func GenTemplateExt(data []byte) string {
	if len(data) < 2 {
		return GenTemplate(data)
	}
	switch data[0] % 5 {
	case 0:
		return GenTemplate(data[1:])
	case 1:
		return `<a href="{{ $a }}{{ $b }}">`
	case 2:
		return `<!-- {{ $x }} -->`
	case 3:
		return `<iframe srcdoc="{{ $x }}">`
	case 4:
		return `<` + `{{ $t }}` + `>`
	default:
		return GenTemplate(data)
	}
}

// CorpusSeeds are interesting starting inputs for the fuzzer.
func CorpusSeeds() [][]byte {
	var out [][]byte
	for i := 0; i < 8; i++ {
		out = append(out, []byte{byte(i), 1, 2, 3})
	}
	interesting := []string{
		`<p>{{ $x }}</p>`,
		`<a href="{{ $x }}">`,
		`<a title="{{ $x }}">`,
		`<div class={{ $x }}>`,
		`<script>{{ $x }}</script>`,
		`<a onclick="{{ $x }}">`,
		`<iframe srcdoc="{{ $x }}">`,
		`<a href="{{ $a }}{{ $b }}">`,
		`<div style="width: {{ $x }}px">`,
		`<textarea>{{ $x }}</textarea>`,
		`<div class="x" @attrs($m)>`,
		`<script>@yield('js')</script>`,
		`<a href="@yield('u')">`,
		`<div title="{{ $slot }}">`,
		`<div class=@yield('c')>`,
	}
	for _, s := range interesting {
		out = append(out, append([]byte{0xff}, []byte(s)...))
	}
	return out
}

// TemplateFromSeed decodes corpus / fuzz bytes into a template string.
func TemplateFromSeed(data []byte) string {
	if len(data) > 1 && data[0] == 0xff {
		return string(data[1:])
	}
	return GenTemplateExt(data)
}

// --- Combinatorial catalog ---

// ContextKind for exhaustive scan.
type ContextKind int

const (
	CtxText ContextKind = iota
	CtxComment
	CtxTextarea
	CtxTitle
	CtxTagName
	CtxAttrName
	CtxBetweenAttrs
	CtxUnquoted
	CtxDoubleQuoted
	CtxSingleQuoted
	CtxVerbatimOpenScript // @verbatim <script> @endverbatim then echo
	CtxVerbatimOpenStyle
	CtxTemplateCommentScript // {{-- <script> --}} then echo
	CtxAttrsBag          // <div @attrs($m)>
)

func (c ContextKind) String() string {
	switch c {
	case CtxText:
		return "text"
	case CtxComment:
		return "comment"
	case CtxTextarea:
		return "textarea"
	case CtxTitle:
		return "title"
	case CtxTagName:
		return "tag-name"
	case CtxAttrName:
		return "attr-name"
	case CtxBetweenAttrs:
		return "between-attrs"
	case CtxUnquoted:
		return "unquoted"
	case CtxDoubleQuoted:
		return "double-quoted"
	case CtxSingleQuoted:
		return "single-quoted"
	case CtxVerbatimOpenScript:
		return "verbatim-open-script"
	case CtxVerbatimOpenStyle:
		return "verbatim-open-style"
	case CtxTemplateCommentScript:
		return "template-comment-script"
	case CtxAttrsBag:
		return "attrs-bag"
	default:
		return "?"
	}
}

// PositionKind for interpolation placement.
type PositionKind int

const (
	PosValueStart PositionKind = iota
	PosAfterStaticPrefix
	PosMultiInterp
	PosMidValue
)

func (p PositionKind) String() string {
	switch p {
	case PosValueStart:
		return "value-start"
	case PosAfterStaticPrefix:
		return "after-static"
	case PosMultiInterp:
		return "multi-interp"
	case PosMidValue:
		return "mid-value"
	default:
		return "?"
	}
}

// ExprKind for expression form.
type ExprKind int

const (
	ExprEcho ExprKind = iota
	ExprJSON
	ExprJS
	ExprAttrs
)

func (e ExprKind) String() string {
	switch e {
	case ExprEcho:
		return "{{}}"
	case ExprJSON:
		return "@json"
	case ExprJS:
		return "@js"
	case ExprAttrs:
		return "@attrs"
	default:
		return "?"
	}
}

// ComboCase is one deterministic template cell.
type ComboCase struct {
	Name     string
	Tmpl     string
	Ctx      ContextKind
	Attr     string
	Pos      PositionKind
	Expr     ExprKind
	SkipAttr bool // context ignores attr
}

var comboAttrs = []string{
	"href", "src", "action", "formaction", "poster", "srcset",
	"srcdoc", "style", "onclick", "title", "data-x", "class", "value", "id",
}

// BuildComboCatalog builds the full deterministic template set.
func BuildComboCatalog() []ComboCase {
	var out []ComboCase
	contexts := []ContextKind{
		CtxText, CtxComment, CtxTextarea, CtxTitle,
		CtxTagName, CtxAttrName, CtxBetweenAttrs,
		CtxUnquoted, CtxDoubleQuoted, CtxSingleQuoted,
	}
	positions := []PositionKind{PosValueStart, PosAfterStaticPrefix, PosMultiInterp, PosMidValue}
	exprs := []ExprKind{ExprEcho, ExprJSON, ExprJS}

	echo := func(e ExprKind, v string) string {
		switch e {
		case ExprJSON:
			return "@json($" + v + ")"
		case ExprJS:
			return "@js($" + v + ")"
		case ExprAttrs:
			return "@attrs($" + v + ")"
		default:
			return "{{ $" + v + " }}"
		}
	}

	for _, ctx := range contexts {
		attrs := comboAttrs
		if ctx == CtxText || ctx == CtxComment || ctx == CtxTextarea || ctx == CtxTitle ||
			ctx == CtxTagName || ctx == CtxAttrName || ctx == CtxBetweenAttrs ||
			ctx == CtxVerbatimOpenScript || ctx == CtxVerbatimOpenStyle || ctx == CtxTemplateCommentScript ||
			ctx == CtxAttrsBag {
			attrs = []string{""}
		}
		for _, attr := range attrs {
			for _, pos := range positions {
				for _, expr := range exprs {
					if ctx == CtxAttrsBag && expr != ExprAttrs {
						continue
					}
					if ctx != CtxAttrsBag && expr == ExprAttrs {
						continue
					}
					tmpl, ok := buildComboTmpl(ctx, attr, pos, expr, echo)
					if !ok {
						continue
					}
					name := fmt.Sprintf("%s/%s/%s/%s", ctx, attrOrNone(attr), pos, expr)
					out = append(out, ComboCase{
						Name: name, Tmpl: tmpl, Ctx: ctx, Attr: attr, Pos: pos, Expr: expr,
						SkipAttr: attr == "",
					})
				}
			}
		}
	}
	// Extra verbatim / comment / @attrs / layout-placement tokens.
	extras := []ComboCase{
		{Name: "verbatim-open-script/extra", Tmpl: `@verbatim <script> @endverbatim {{ $x }}`, Ctx: CtxVerbatimOpenScript, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "verbatim-open-style/extra", Tmpl: `@verbatim <style> @endverbatim {{ $x }}`, Ctx: CtxVerbatimOpenStyle, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "template-comment-script/extra", Tmpl: `{{-- <script> --}}{{ $x }}`, Ctx: CtxTemplateCommentScript, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "attrs-bag/extra", Tmpl: `<div class="x" @attrs($m)>`, Ctx: CtxAttrsBag, Pos: PosValueStart, Expr: ExprAttrs, SkipAttr: true},
		{Name: "yield-in-script/extra", Tmpl: `<script>@yield('js')</script>`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "yield-in-style/extra", Tmpl: `<style>@yield('css')</style>`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "yield-in-href/extra", Tmpl: `<a href="@yield('u')">`, Ctx: CtxDoubleQuoted, Attr: "href", Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "yield-between-attrs/extra", Tmpl: `<div @yield('attrs')>`, Ctx: CtxBetweenAttrs, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "yield-unquoted/extra", Tmpl: `<div class=@yield('c')>`, Ctx: CtxUnquoted, Attr: "class", Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "slot-in-title-attr/extra", Tmpl: `<div title="{{ $slot }}">`, Ctx: CtxDoubleQuoted, Attr: "title", Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "stack-in-script/extra", Tmpl: `<script>@stack('s')</script>`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "stack-in-text/extra", Tmpl: `<div>@stack('s')</div>`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "lang-text/extra", Tmpl: `<p>@lang('k')</p>`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "csrf-text/extra", Tmpl: `@csrf`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "yield-default-var/extra", Tmpl: `<title>@yield('t', $x)</title>`, Ctx: CtxTitle, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "unknown-end-foo/extra", Tmpl: `@endfoo`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "unknown-end-widget/extra", Tmpl: `@endwidget`, Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
		{Name: "unknown-end-pair/extra", Tmpl: "@foo(x)\nbody\n@endfoo", Ctx: CtxText, Pos: PosValueStart, Expr: ExprEcho, SkipAttr: true},
	}
	out = append(out, extras...)
	return out
}

func attrOrNone(a string) string {
	if a == "" {
		return "-"
	}
	return a
}

func buildComboTmpl(ctx ContextKind, attr string, pos PositionKind, expr ExprKind, echo func(ExprKind, string) string) (string, bool) {
	x := echo(expr, "x")
	a := echo(expr, "a")
	b := echo(expr, "b")

	switch ctx {
	case CtxText:
		if pos != PosValueStart || expr != ExprEcho && expr != ExprJSON && expr != ExprJS {
			if pos != PosValueStart {
				return "", false
			}
		}
		return `<p>` + x + `</p>`, true
	case CtxComment:
		if pos != PosValueStart {
			return "", false
		}
		return `<!-- ` + x + ` -->`, true
	case CtxTextarea:
		if pos != PosValueStart {
			return "", false
		}
		return `<textarea>` + x + `</textarea>`, true
	case CtxTitle:
		if pos != PosValueStart {
			return "", false
		}
		return `<title>` + x + `</title>`, true
	case CtxTagName:
		if pos != PosValueStart || expr != ExprEcho {
			return "", false
		}
		return `<` + x + `>`, true
	case CtxAttrName:
		if pos != PosValueStart || expr != ExprEcho {
			return "", false
		}
		return `<div ` + x + `>`, true
	case CtxBetweenAttrs:
		if pos != PosValueStart || expr != ExprEcho {
			return "", false
		}
		return `<a href="x" ` + x + `>`, true
	case CtxUnquoted, CtxDoubleQuoted, CtxSingleQuoted:
		if attr == "" {
			return "", false
		}
		qL, qR := ``, ``
		switch ctx {
		case CtxDoubleQuoted:
			qL, qR = `"`, `"`
		case CtxSingleQuoted:
			qL, qR = `'`, `'`
		}
		switch pos {
		case PosValueStart:
			return `<div ` + attr + `=` + qL + x + qR + `>`, true
		case PosAfterStaticPrefix:
			prefix := "/ok/"
			if attr == "style" {
				prefix = "width:"
			} else if strings.HasPrefix(attr, "on") || attr == "srcdoc" {
				prefix = "x"
			}
			return `<div ` + attr + `=` + qL + prefix + x + qR + `>`, true
		case PosMultiInterp:
			return `<div ` + attr + `=` + qL + a + b + qR + `>`, true
		case PosMidValue:
			mid := ":"
			if attr == "style" {
				return `<div ` + attr + `=` + qL + `color:` + x + `;` + qR + `>`, true
			}
			return `<div ` + attr + `=` + qL + `pre` + mid + x + qR + `>`, true
		}
	}
	return "", false
}

// LegacyProbeCases are known-vulnerable patterns under EscapeLegacy.
type LegacyProbe struct {
	Ctx     string
	Tmpl    string
	Payload map[string]any
	WantKind string // expected finding kind substring
}

// LegacyProbes returns cases Legacy must fail (oracle findings > 0).
func LegacyProbes() []LegacyProbe {
	return []LegacyProbe{
		{
			Ctx: "unquoted-attr",
			Tmpl: `<div title={{ $x }}>`,
			Payload: map[string]any{"x": `x onmouseover=alert(1)`},
			WantKind: "on-attr",
		},
		{
			Ctx: "javascript-url",
			Tmpl: `<a href="{{ $x }}">`,
			Payload: map[string]any{"x": `javascript:alert(1)`},
			WantKind: "unsafe-url",
		},
		{
			Ctx:      "json-in-script",
			Tmpl:     `<script>var a=@json($x);</script>`,
			Payload:  map[string]any{"x": `</script><script>alert(1)</script>`},
			WantKind: "", // encoding/json escapes — expect no injection (oracle must not false-positive)
		},
		{
			Ctx:      "raw-script-break",
			Tmpl:     `<script>var a="{!! $x !!}";</script>`,
			Payload:  map[string]any{"x": `</script><script>alert(1)</script>`},
			WantKind: "script",
		},
		{
			Ctx: "onclick",
			Tmpl: `<a onclick="{{ $x }}">`,
			Payload: map[string]any{"x": `alert(1)`},
			WantKind: "", // Legacy may HTML-escape but still set onclick — skeleton/on may vary; shape has onclick
		},
		{
			Ctx:      "quote-breakout",
			Tmpl:     `<div id={{ $x }}>`,
			Payload:  map[string]any{"x": `x onclick=alert(1)`},
			WantKind: "on-attr",
		},
		{
			Ctx:      "srcdoc-legacy",
			Tmpl:     `<iframe srcdoc="{{ $x }}">`,
			Payload:  map[string]any{"x": `<script>alert(1)</script>`},
			WantKind: "script",
		},
		{
			Ctx: "tab-javascript-url",
			Tmpl: `<a href="{{ $x }}">`,
			Payload: map[string]any{"x": "java\tscript:alert(1)"},
			WantKind: "unsafe-url",
		},
	}
}
