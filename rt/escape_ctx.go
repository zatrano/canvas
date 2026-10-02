package rt

import (
	"fmt"
	"regexp"
	"strings"
)

// EscapeMode selects contextual escaping policy.
type EscapeMode int

const (
	// EscapeStrict is the default: URL/unquoted/on*/script/style rules apply.
	EscapeStrict EscapeMode = iota
	// EscapeLegacy keeps pre-Phase-3 context-blind HTML escaping only.
	EscapeLegacy
)

// EscapeKind is the escape action for one interpolation site.
type EscapeKind int

const (
	EscHTML EscapeKind = iota
	EscURLAttr
	EscUnquoted
	EscJSON
	EscJSONAttr
	EscURLBlock    // mid-URL interpolation without safe static lock → always #unsafe
	EscCSS         // style attr declaration value after ':' — filtered charset
	EscURLUnquoted // URL attr in unquoted context: scheme filter + space escape
	EscForbid      // Strict: reject at compile time
)

// InterpMarker is written into the static prefix after each interpolation so the
// scanner can treat "interpolation-only" value prefixes as still at value-start.
const InterpMarker = "\x01"

func (k EscapeKind) String() string {
	switch k {
	case EscHTML:
		return "html"
	case EscURLAttr:
		return "url"
	case EscUnquoted:
		return "unquoted"
	case EscJSON:
		return "json"
	case EscJSONAttr:
		return "json-attr"
	case EscURLBlock:
		return "url-block"
	case EscCSS:
		return "css"
	case EscURLUnquoted:
		return "url-unquoted"
	case EscForbid:
		return "forbid"
	default:
		return "html"
	}
}

// ContextName is a human-readable context for error messages.
func (k EscapeKind) ContextName(st htmlCtx, attr string) string {
	switch st {
	case ctxScript:
		return "script"
	case ctxStyle:
		return "style"
	case ctxTagOpen:
		return "tag-name"
	case ctxAttrName:
		return "attribute-name"
	case ctxComment:
		return "comment"
	case ctxValueDQ, ctxValueSQ, ctxValueUnquoted, ctxBeforeValue:
		if attr != "" {
			return "attribute:" + attr
		}
		return "attribute"
	default:
		return "text"
	}
}

type htmlCtx int

const (
	ctxText htmlCtx = iota
	ctxTagOpen
	ctxAttrName
	ctxBeforeValue
	ctxValueDQ
	ctxValueSQ
	ctxValueUnquoted
	ctxScript
	ctxStyle
	ctxComment
)

var urlAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"poster": true, "data": true, "srcset": true, "xlink:href": true,
	"background": true, "cite": true, "ping": true, "manifest": true,
}

var forbidAttrs = map[string]bool{
	"srcdoc": true,
	// style: entire-value / property-name → EscForbid; declaration value after ':' → EscCSS
}

// ScanEscapeKind classifies an interpolation immediately after staticBefore.
func ScanEscapeKind(staticBefore string, jsonDirective bool, mode EscapeMode) EscapeKind {
	if mode == EscapeLegacy {
		if jsonDirective {
			return EscJSON
		}
		return EscHTML
	}
	st, attr, atValueStart, valuePrefix := scanHTML(staticBefore)
	_ = valuePrefix
	switch st {
	case ctxTagOpen, ctxAttrName:
		return EscForbid
	case ctxScript:
		// @js / @json are the only allowed interpolations in script bodies.
		if jsonDirective {
			return EscJSON
		}
		return EscForbid
	case ctxStyle:
		// Style element: no {{ }}, @json, or @js (CSS-in-JSON is not a safe subset).
		return EscForbid
	case ctxComment:
		// HTML comments: escape so --> cannot break out.
		if jsonDirective {
			return EscJSON
		}
		return EscHTML
	case ctxValueUnquoted:
		// on*/srcdoc/style-forbid always win — including @json/@js.
		if strings.HasPrefix(attr, "on") || forbidAttrs[attr] {
			return EscForbid
		}
		if attr == "style" {
			// Unquoted style values cannot safely host CSS tokens (spaces split attrs).
			return EscForbid
		}
		if jsonDirective {
			// JSON contains spaces/quotes that break unquoted attrs → fail closed.
			return EscForbid
		}
		if urlAttrs[attr] {
			k := urlKind(atValueStart, valuePrefix)
			if k == EscURLBlock {
				return EscURLBlock
			}
			if k == EscURLAttr {
				return EscURLUnquoted
			}
			// safe static lock → still unquoted, need space escaping
			return EscUnquoted
		}
		return EscUnquoted
	case ctxValueDQ, ctxValueSQ:
		// on*/srcdoc before @json/@js so handlers cannot embed JSON payloads.
		if strings.HasPrefix(attr, "on") || forbidAttrs[attr] {
			return EscForbid
		}
		if attr == "style" {
			if jsonDirective {
				return EscForbid
			}
			return styleAttrKind(atValueStart, valuePrefix)
		}
		if jsonDirective {
			return EscJSONAttr
		}
		if urlAttrs[attr] {
			return urlKind(atValueStart, valuePrefix)
		}
		return EscHTML
	case ctxBeforeValue:
		if strings.HasPrefix(attr, "on") || forbidAttrs[attr] || attr == "style" || jsonDirective {
			// style before-value = property name / whole value; json unquoted = fail closed.
			return EscForbid
		}
		if urlAttrs[attr] {
			// `attr=` with interpolation next (no quote yet) → unquoted URL value.
			return EscURLUnquoted
		}
		return EscUnquoted
	default:
		if jsonDirective {
			return EscJSON
		}
		return EscHTML
	}
}

// ScanEscapeDetail returns kind plus context label for errors.
func ScanEscapeDetail(staticBefore string, jsonDirective bool, mode EscapeMode) (EscapeKind, string) {
	st, attr, _, _ := scanHTML(staticBefore)
	k := ScanEscapeKind(staticBefore, jsonDirective, mode)
	return k, k.ContextName(st, attr)
}

func urlKind(atValueStart bool, valuePrefix string) EscapeKind {
	if atValueStart || interpOnlyPrefix(valuePrefix) {
		return EscURLAttr
	}
	if safeURLStaticLock(valuePrefix) {
		return EscHTML
	}
	return EscURLBlock
}

// styleAttrKind: whole-value / property-name interpolation → Forbid;
// declaration value after a static ':' → EscCSS.
func styleAttrKind(atValueStart bool, valuePrefix string) EscapeKind {
	if atValueStart || interpOnlyPrefix(valuePrefix) || valuePrefix == "" {
		return EscForbid
	}
	if cssValuePosition(valuePrefix) {
		return EscCSS
	}
	return EscForbid
}

func cssValuePosition(prefix string) bool {
	var b strings.Builder
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == InterpMarker[0] {
			continue
		}
		b.WriteByte(prefix[i])
	}
	s := strings.TrimRight(b.String(), " \t\n\r")
	return strings.HasSuffix(s, ":")
}

var cssValueOK = regexp.MustCompile(`^[A-Za-z0-9#%.,+\- ]*$`)

// EscapeCSSValue filters a CSS declaration value for Strict style attrs.
// Returns "unsafe" when the value is not a safe token.
func EscapeCSSValue(s string) string {
	if !cssValueOK.MatchString(s) {
		return "unsafe"
	}
	lower := strings.ToLower(s)
	for _, bad := range []string{"/*", "url(", "expression(", `\`, ";", "}"} {
		if strings.Contains(lower, bad) || strings.Contains(s, bad) {
			return "unsafe"
		}
	}
	return s
}

func interpOnlyPrefix(p string) bool {
	for i := 0; i < len(p); i++ {
		if p[i] != InterpMarker[0] {
			return false
		}
	}
	return len(p) > 0
}

func safeURLStaticLock(p string) bool {
	// Strip interp markers for scheme lock check.
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == InterpMarker[0] {
			continue
		}
		b.WriteByte(p[i])
	}
	s := stripURLPrefixNoise(b.String())
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "/") ||
		strings.HasPrefix(lower, "#") ||
		strings.HasPrefix(lower, "?") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:")
}

func scanHTML(s string) (st htmlCtx, attr string, atValueStart bool, valuePrefix string) {
	st = ctxText
	var tag, curAttr string
	atValueStart = false
	var val strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch st {
		case ctxText:
			if c == '<' {
				if strings.HasPrefix(s[i:], "<!--") {
					st = ctxComment
					i += 4
					continue
				}
				st = ctxTagOpen
				tag = ""
				curAttr = ""
			}
			i++
		case ctxComment:
			if strings.HasPrefix(s[i:], "-->") {
				st = ctxText
				i += 3
				continue
			}
			i++
		case ctxTagOpen:
			if c == '>' {
				t := strings.ToLower(strings.TrimSpace(tag))
				switch t {
				case "script":
					st = ctxScript
				case "style":
					st = ctxStyle
				default:
					st = ctxText
				}
				i++
				continue
			}
			if c == '/' && i+1 < len(s) && s[i+1] == '>' {
				st = ctxText
				i += 2
				continue
			}
			if isHTMLSpace(c) {
				st = ctxAttrName
				curAttr = ""
				i++
				continue
			}
			tag += string(c)
			i++
		case ctxAttrName:
			if c == '=' {
				attr = strings.ToLower(strings.TrimSpace(curAttr))
				st = ctxBeforeValue
				i++
				continue
			}
			if c == '>' {
				t := strings.ToLower(strings.TrimSpace(tag))
				switch t {
				case "script":
					st = ctxScript
				case "style":
					st = ctxStyle
				default:
					st = ctxText
				}
				i++
				continue
			}
			if isHTMLSpace(c) {
				if curAttr != "" {
					// boolean attr finished — still in attr-name region for next attr
					curAttr = ""
				}
				i++
				continue
			}
			curAttr += string(c)
			i++
		case ctxBeforeValue:
			if c == '"' {
				st = ctxValueDQ
				atValueStart = true
				val.Reset()
				i++
				continue
			}
			if c == '\'' {
				st = ctxValueSQ
				atValueStart = true
				val.Reset()
				i++
				continue
			}
			if isHTMLSpace(c) {
				i++
				continue
			}
			st = ctxValueUnquoted
			atValueStart = true
			val.Reset()
			continue
		case ctxValueDQ:
			if c == '"' {
				st = ctxAttrName
				curAttr = ""
				atValueStart = false
				val.Reset()
				i++
				continue
			}
			val.WriteByte(c)
			atValueStart = interpOnlyPrefix(val.String())
			i++
		case ctxValueSQ:
			if c == '\'' {
				st = ctxAttrName
				curAttr = ""
				atValueStart = false
				val.Reset()
				i++
				continue
			}
			val.WriteByte(c)
			atValueStart = interpOnlyPrefix(val.String())
			i++
		case ctxValueUnquoted:
			if isHTMLSpace(c) {
				st = ctxAttrName
				curAttr = ""
				atValueStart = false
				val.Reset()
				i++
				continue
			}
			if c == '>' {
				t := strings.ToLower(strings.TrimSpace(tag))
				switch t {
				case "script":
					st = ctxScript
				case "style":
					st = ctxStyle
				default:
					st = ctxText
				}
				atValueStart = false
				val.Reset()
				i++
				continue
			}
			val.WriteByte(c)
			atValueStart = interpOnlyPrefix(val.String())
			i++
		case ctxScript:
			if n, ok := consumeEndTag(s, i, "script"); ok {
				st = ctxText
				i = n
				continue
			}
			i++
		case ctxStyle:
			if n, ok := consumeEndTag(s, i, "style"); ok {
				st = ctxText
				i = n
				continue
			}
			i++
		default:
			i++
		}
	}
	return st, attr, atValueStart, val.String()
}

func consumeEndTag(s string, i int, name string) (int, bool) {
	if i+2+len(name) > len(s) || s[i] != '<' || s[i+1] != '/' {
		return 0, false
	}
	j := i + 2
	if !strings.EqualFold(s[j:j+len(name)], name) {
		return 0, false
	}
	j += len(name)
	for j < len(s) && isHTMLSpace(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '>' {
		return j + 1, true
	}
	return 0, false
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// EscapeURLAttr normalizes a URL attribute value for Strict mode.
func EscapeURLAttr(s string, atStart bool) string {
	if !atStart {
		return htmlEscapeString(s)
	}
	trimmed := stripURLPrefixNoise(s)
	lower := strings.ToLower(trimmed)
	switch {
	case lower == "" || strings.HasPrefix(lower, "#") || strings.HasPrefix(lower, "?"):
		return htmlEscapeString(trimmed)
	case strings.HasPrefix(lower, "/"):
		return htmlEscapeString(trimmed)
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return htmlEscapeString(trimmed)
	case strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:"):
		return htmlEscapeString(trimmed)
	case strings.Contains(lower, ":"):
		return "#unsafe"
	default:
		return htmlEscapeString(trimmed)
	}
}

func stripURLPrefixNoise(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	started := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == InterpMarker[0] {
			continue
		}
		if !started {
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
				continue
			}
			started = true
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// EscapeUnquotedAttr escapes a value for an unquoted HTML attribute.
func EscapeUnquotedAttr(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ':
			b.WriteString("&#32;")
		case '\t':
			b.WriteString("&#9;")
		case '\n':
			b.WriteString("&#10;")
		case '\v':
			b.WriteString("&#11;")
		case '\f':
			b.WriteString("&#12;")
		case '\r':
			b.WriteString("&#13;")
		case '=':
			b.WriteString("&#61;")
		case '`':
			b.WriteString("&#96;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func htmlEscapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// FormatForbidError builds a Strict compile error with location and fix hint.
func FormatForbidError(tmplName string, line, col int, context string) error {
	if line <= 0 {
		line = 1
	}
	if col <= 0 {
		col = 1
	}
	if tmplName == "" {
		tmplName = "?"
	}
	if context == "" {
		context = "forbidden"
	}
	return fmt.Errorf("canvas template [%s] at %d:%d: unsafe interpolation in %s context; use @js($x), @attrs($map), a data-* attribute, or rt.SafeHTML for trusted markup",
		tmplName, line, col, context)
}

// AttrsPositionOK reports whether @attrs($map) is legal at the end of staticBefore
// (inside a start tag, at attribute-name position).
func AttrsPositionOK(staticBefore string) (ok bool, context string) {
	st, attr, _, _ := scanHTML(staticBefore)
	switch st {
	case ctxAttrName, ctxTagOpen:
		return true, "attribute"
	case ctxScript:
		return false, "script"
	case ctxStyle:
		return false, "style"
	case ctxValueDQ, ctxValueSQ, ctxValueUnquoted, ctxBeforeValue:
		if attr != "" {
			return false, "attribute:" + attr
		}
		return false, "attribute-value"
	default:
		return false, htmlCtxName(st)
	}
}

func htmlCtxName(st htmlCtx) string {
	switch st {
	case ctxText:
		return "text"
	case ctxComment:
		return "comment"
	case ctxTagOpen:
		return "tag"
	case ctxAttrName:
		return "attribute"
	case ctxBeforeValue, ctxValueDQ, ctxValueSQ, ctxValueUnquoted:
		return "attribute-value"
	case ctxScript:
		return "script"
	case ctxStyle:
		return "style"
	default:
		return "forbidden"
	}
}

// LineCol returns 1-based line and column for offset in src.
func LineCol(src string, offset int) (line, col int) {
	line, col = 1, 1
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
