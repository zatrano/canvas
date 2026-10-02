// Package oracle fuzzes Canvas Strict escape against an HTML injection oracle.
// It is a separate module so golang.org/x/net stays out of the root go.mod.
package oracle

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// AttackPayloads used as interpolation data (~30 vectors).
var AttackPayloads = []string{
	`"><script>alert(1)</script>`,
	`' onmouseover='x`,
	`" onfocus="alert(1)`,
	`javascript:alert(1)`,
	"java\tscript:alert(1)",
	"java\nscript:alert(1)",
	"java\rscript:alert(1)",
	`JaVaScRiPt:alert(1)`,
	`data:text/html,<script>alert(1)</script>`,
	`vbscript:msgbox(1)`,
	`</script>`,
	`</script><script>alert(1)</script>`,
	"\u2028",
	"\u2029",
	`</textarea>`,
	`</textarea><script>alert(1)</script>`,
	`</title><script>alert(1)</script>`,
	`-->`,
	`--><script>alert(1)</script>`,
	`<img src=x onerror=alert(1)>`,
	`x onmouseover=alert(1)`,
	`#\"><script>alert(1)</script>`,
	`red;background:url(javascript:x)`,
	`expression(alert(1))`,
	`}</style><script>alert(1)</script>`,
	`\3c script`,
	`url(javascript:alert(1))`,
	`/* */url(javascript:x)`,
	`&apos; onmouseover=&apos;x`,
	`&#34; onmouseover=&#34;x`,
}

// StaticShape is the set of elements/attrs present in the template with
// interpolations replaced by placeholders (baseline).
type StaticShape struct {
	Tags  map[string]struct{}
	Attrs map[string]struct{} // "tag|attr" or "*|attr"
}

func shapeOf(htmlSrc string) StaticShape {
	s := StaticShape{
		Tags:  map[string]struct{}{},
		Attrs: map[string]struct{}{},
	}
	reTag := regexp.MustCompile(`(?i)<\s*/?\s*([a-zA-Z][\w:]*)`)
	for _, m := range reTag.FindAllStringSubmatch(htmlSrc, -1) {
		s.Tags[strings.ToLower(m[1])] = struct{}{}
	}
	reAttr := regexp.MustCompile(`(?i)([a-zA-Z_:][\w:.-]*)\s*=`)
	for _, m := range reAttr.FindAllStringSubmatch(htmlSrc, -1) {
		s.Attrs["*|"+strings.ToLower(m[1])] = struct{}{}
	}
	z := html.NewTokenizer(strings.NewReader(htmlSrc))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return s
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			cur := strings.ToLower(t.Data)
			s.Tags[cur] = struct{}{}
			for _, a := range t.Attr {
				s.Attrs[cur+"|"+strings.ToLower(a.Key)] = struct{}{}
				s.Attrs["*|"+strings.ToLower(a.Key)] = struct{}{}
			}
		}
	}
}

// InjectionFinding describes an oracle violation.
type InjectionFinding struct {
	Kind   string
	Detail string
	Ctx    string // optional context bucket for Legacy reports
}

// Skeleton is an ordered DOM structural fingerprint.
type Skeleton struct {
	// ElemAttrs is ordered "tag" then "tag@attr" entries for each start tag.
	Seq            []string
	ElemCount      int
	ScriptLenClass int // 0=empty, 1=short(<32), 2=med(<256), 3=long
	StyleLenClass  int
}

func lenClass(n int) int {
	switch {
	case n == 0:
		return 0
	case n < 32:
		return 1
	case n < 256:
		return 2
	default:
		return 3
	}
}

// SkeletonOf parses HTML and builds an ordered element/attribute-name skeleton.
func SkeletonOf(htmlSrc string) (Skeleton, error) {
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return Skeleton{}, err
	}
	var sk Skeleton
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			sk.Seq = append(sk.Seq, tag)
			sk.ElemCount++
			for _, a := range n.Attr {
				sk.Seq = append(sk.Seq, tag+"@"+strings.ToLower(a.Key))
			}
			if tag == "script" {
				sk.ScriptLenClass = lenClass(textLen(n))
			}
			if tag == "style" {
				sk.StyleLenClass = lenClass(textLen(n))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return sk, nil
}

func textLen(n *html.Node) int {
	var nlen int
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			nlen += len(c.Data)
		}
	}
	return nlen
}

// CheckSkeletonDiff compares safe vs attack render skeletons.
func CheckSkeletonDiff(safeSk, attackSk Skeleton) []InjectionFinding {
	var findings []InjectionFinding
	if safeSk.ElemCount != attackSk.ElemCount {
		findings = append(findings, InjectionFinding{
			Kind: "elem-count", Detail: fmt.Sprintf("%d→%d", safeSk.ElemCount, attackSk.ElemCount),
		})
	}
	if safeSk.ScriptLenClass == 0 && attackSk.ScriptLenClass > 0 {
		findings = append(findings, InjectionFinding{
			Kind: "script-len-class", Detail: fmt.Sprintf("%d→%d", safeSk.ScriptLenClass, attackSk.ScriptLenClass),
		})
	}
	if safeSk.StyleLenClass == 0 && attackSk.StyleLenClass > 0 {
		findings = append(findings, InjectionFinding{
			Kind: "style-len-class", Detail: fmt.Sprintf("%d→%d", safeSk.StyleLenClass, attackSk.StyleLenClass),
		})
	}
	min := len(safeSk.Seq)
	if len(attackSk.Seq) < min {
		min = len(attackSk.Seq)
	}
	for i := 0; i < min; i++ {
		if safeSk.Seq[i] != attackSk.Seq[i] {
			findings = append(findings, InjectionFinding{
				Kind: "seq-mismatch", Detail: fmt.Sprintf("@%d %s→%s", i, safeSk.Seq[i], attackSk.Seq[i]),
			})
			break
		}
	}
	if len(safeSk.Seq) != len(attackSk.Seq) {
		findings = append(findings, InjectionFinding{
			Kind: "seq-len", Detail: fmt.Sprintf("%d→%d", len(safeSk.Seq), len(attackSk.Seq)),
		})
	}
	return findings
}

// CheckRendered compares rendered HTML against the static template shape
// and absolute URL / script-break rules.
func CheckRendered(staticTmpl, rendered string) []InjectionFinding {
	base := stripDynamic(staticTmpl)
	shape := shapeOf(base)
	baseLower := strings.ToLower(base)

	if strings.Contains(strings.ToLower(rendered), "zgotmplz") {
		return nil
	}

	var findings []InjectionFinding
	findings = append(findings, checkAbsoluteRules(rendered)...)

	z := html.NewTokenizer(strings.NewReader(rendered))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return findings
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			tag := strings.ToLower(t.Data)
			if dangerousTag(tag) {
				if _, ok := shape.Tags[tag]; !ok && !strings.Contains(baseLower, "<"+tag) {
					findings = append(findings, InjectionFinding{Kind: "new-element", Detail: tag})
				}
			}
			for _, a := range t.Attr {
				key := strings.ToLower(a.Key)
				full := tag + "|" + key
				if strings.HasPrefix(key, "on") {
					if _, ok := shape.Attrs[full]; !ok {
						if _, ok2 := shape.Attrs["*|"+key]; !ok2 {
							findings = append(findings, InjectionFinding{Kind: "on-attr", Detail: full + "=" + a.Val})
						}
					}
				}
				if isURLAttr(key) && unsafeScheme(a.Val) {
					findings = append(findings, InjectionFinding{Kind: "unsafe-url", Detail: full + "=" + a.Val})
				}
				if key == "style" && unsafeCSSValue(a.Val) {
					findings = append(findings, InjectionFinding{Kind: "unsafe-css", Detail: full + "=" + a.Val})
				}
			}
		}
	}
}

// CheckPair runs skeleton + absolute checks for safe vs attack renders of the same template.
func CheckPair(staticTmpl, safeOut, attackOut string) []InjectionFinding {
	if strings.Contains(strings.ToLower(attackOut), "zgotmplz") {
		return nil
	}
	var findings []InjectionFinding
	findings = append(findings, CheckRendered(staticTmpl, attackOut)...)
	safeSk, err1 := SkeletonOf(safeOut)
	attackSk, err2 := SkeletonOf(attackOut)
	if err1 == nil && err2 == nil {
		findings = append(findings, CheckSkeletonDiff(safeSk, attackSk)...)
	}
	return dedupeFindings(findings)
}

func dedupeFindings(in []InjectionFinding) []InjectionFinding {
	seen := map[string]struct{}{}
	var out []InjectionFinding
	for _, f := range in {
		k := f.Kind + "|" + f.Detail
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, f)
	}
	return out
}

func checkAbsoluteRules(rendered string) []InjectionFinding {
	var findings []InjectionFinding
	z := html.NewTokenizer(strings.NewReader(rendered))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return findings
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			for _, a := range t.Attr {
				key := strings.ToLower(a.Key)
				if isURLAttr(key) && unsafeScheme(a.Val) {
					findings = append(findings, InjectionFinding{
						Kind: "unsafe-url", Detail: strings.ToLower(t.Data) + "|" + key + "=" + a.Val,
					})
				}
			}
		case html.TextToken:
			// handled via script content scan below using Parse
		}
	}
}

// CheckScriptBreak reports if any <script> text contains the literal "</script".
func CheckScriptBreak(rendered string) []InjectionFinding {
	doc, err := html.Parse(strings.NewReader(rendered))
	if err != nil {
		return nil
	}
	var findings []InjectionFinding
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "script") {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode && strings.Contains(strings.ToLower(c.Data), "</script") {
					findings = append(findings, InjectionFinding{Kind: "script-break", Detail: "</script in script body"})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return findings
}

// FullCheck is the strongest oracle: shape + skeleton pair + script-break.
func FullCheck(staticTmpl, safeOut, attackOut string) []InjectionFinding {
	f := CheckPair(staticTmpl, safeOut, attackOut)
	f = append(f, CheckScriptBreak(attackOut)...)
	return dedupeFindings(f)
}

func dangerousTag(name string) bool {
	switch name {
	case "script", "iframe", "object", "embed", "svg", "math",
		"link", "meta", "base", "form", "applet", "frame", "frameset":
		return true
	default:
		return false
	}
}

func isURLAttr(name string) bool {
	switch name {
	case "href", "src", "action", "formaction", "poster", "ping", "background",
		"srcset", "xlink:href", "cite", "data", "manifest":
		return true
	default:
		return false
	}
}

func unsafeScheme(v string) bool {
	v = strings.TrimSpace(v)
	var b strings.Builder
	for _, r := range v {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	s := b.String()
	return strings.HasPrefix(s, "javascript:") ||
		strings.HasPrefix(s, "data:") ||
		strings.HasPrefix(s, "vbscript:")
}

func unsafeCSSValue(v string) bool {
	lower := strings.ToLower(v)
	return strings.Contains(lower, "url(javascript") ||
		strings.Contains(lower, "expression(") ||
		strings.Contains(lower, "url(data:") ||
		strings.Contains(v, "</style")
}

func stripDynamic(tmpl string) string {
	var b bytes.Buffer
	i := 0
	for i < len(tmpl) {
		if strings.HasPrefix(tmpl[i:], "{{") {
			j := strings.Index(tmpl[i:], "}}")
			if j < 0 {
				break
			}
			b.WriteByte('x')
			i += j + 2
			continue
		}
		if tmpl[i] == '@' {
			j := i + 1
			start := j
			for j < len(tmpl) && (unicode.IsLetter(rune(tmpl[j])) || unicode.IsDigit(rune(tmpl[j]))) {
				j++
			}
			if j == start {
				b.WriteByte(tmpl[i])
				i++
				continue
			}
			if j < len(tmpl) && tmpl[j] == '(' {
				depth := 1
				j++
				for j < len(tmpl) && depth > 0 {
					if tmpl[j] == '(' {
						depth++
					} else if tmpl[j] == ')' {
						depth--
					}
					j++
				}
			}
			i = j
			continue
		}
		b.WriteByte(tmpl[i])
		i++
	}
	return b.String()
}

// Normalize for logging.
func Normalize(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

// HarmlessValue is a safe interpolation for skeleton baselines.
const HarmlessValue = "safeok"
