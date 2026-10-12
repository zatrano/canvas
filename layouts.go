package canvas

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/zatrano/canvas/rt"
)

var (
	reExtends         = regexp.MustCompile(`(?i)@extends\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reSection         = regexp.MustCompile(`(?is)@section\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endsection`)
	reSectionShow     = regexp.MustCompile(`(?is)@section\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@show`)
	reYieldDefault    = regexp.MustCompile(`(?i)@yield\s*\(\s*['"]([^'"]+)['"]\s*,\s*['"]([^'"]*)['"]\s*\)`)
	reYieldDefaultVar = regexp.MustCompile(`(?i)@yield\s*\(\s*['"]([^'"]+)['"]\s*,\s*\$([a-zA-Z0-9_.]+)\s*\)`)
	reYield           = regexp.MustCompile(`(?i)@yield\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reInclude         = regexp.MustCompile(`(?i)@include\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reIncludeIf       = regexp.MustCompile(`(?i)@includeIf\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reIncludeWhen     = regexp.MustCompile(`(?i)@includeWhen\s*\(\s*\$([a-zA-Z0-9_.]+)\s*,\s*['"]([^'"]+)['"]\s*(?:,\s*(\[[^\]]*\])\s*)?\)`)
	reIncludeUnless   = regexp.MustCompile(`(?i)@includeUnless\s*\(\s*\$([a-zA-Z0-9_.]+)\s*,\s*['"]([^'"]+)['"]\s*(?:,\s*(\[[^\]]*\])\s*)?\)`)
	reIncludeFirst    = regexp.MustCompile(`(?i)@includeFirst\s*\(\s*\[([^\]]+)\]\s*\)`)
	reIncludeData     = regexp.MustCompile(`(?i)@include\s*\(\s*['"]([^'"]+)['"]\s*,\s*(\[[^\]]*\])\s*\)`)
	reEach            = regexp.MustCompile(`(?i)@each\s*\(\s*['"]([^'"]+)['"]\s*,\s*\$([a-zA-Z0-9_]+)\s*,\s*['"]([^'"]+)['"]\s*(?:,\s*['"]([^'"]+)['"]\s*)?\)`)
	reOnce            = regexp.MustCompile(`(?is)@once\s*(.*?)@endonce`)
	reOnceKey         = regexp.MustCompile(`(?is)@once\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endonce`)
	reVerbatim        = regexp.MustCompile(`(?is)@verbatim\s*(.*?)@endverbatim`)
	reSectionShort    = regexp.MustCompile(`(?i)@section\s*\(\s*['"]([^'"]+)['"]\s*,\s*['"]([^'"]*)['"]\s*\)`)
	reSectionShortVar = regexp.MustCompile(`(?i)@section\s*\(\s*['"]([^'"]+)['"]\s*,\s*\$([a-zA-Z0-9_.]+)\s*\)`)
	rePush            = regexp.MustCompile(`(?is)@push\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endpush`)
	rePrepend         = regexp.MustCompile(`(?is)@prepend\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endprepend`)
	rePushOnce        = regexp.MustCompile(`(?is)@pushOnce\s*\(\s*['"]([^'"]+)['"]\s*(?:,\s*['"]([^'"]+)['"]\s*)?\)(.*?)@endPushOnce`)
	rePrependOnce     = regexp.MustCompile(`(?is)@prependOnce\s*\(\s*['"]([^'"]+)['"]\s*(?:,\s*['"]([^'"]+)['"]\s*)?\)(.*?)@endPrependOnce`)
	reStack           = regexp.MustCompile(`(?i)@stack\s*\(\s*['"]([^'"]+)['"]\s*(?:,\s*['"]([^'"]*)['"]\s*)?\)`)
	reComponent       = regexp.MustCompile(`(?is)@component\s*\(\s*['"]([^'"]+)['"]\s*(?:,\s*(\[[^\]]*\])\s*)?\)(.*?)@endcomponent`)
	reSlot            = regexp.MustCompile(`(?is)@slot\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endslot`)
	reHasSection      = regexp.MustCompile(`(?is)@hasSection\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endif`)
	reSectionMissing  = regexp.MustCompile(`(?is)@sectionMissing\s*\(\s*['"]([^'"]+)['"]\s*\)(.*?)@endif`)
)

type stackBag struct {
	prepend []string
	append  []string
	once    map[string]bool
}

// resolveView expands includes/extends/sections/stacks into a final template string.
func (e *Engine) resolveView(name string, seen []string) (string, error) {
	bags := map[string]*stackBag{}
	once := map[string]bool{}
	content, err := e.resolveViewWithStacks(name, seen, bags, once)
	if err != nil {
		return "", err
	}
	return applyStacks(content, bags), nil
}

func (e *Engine) resolveViewWithStacks(name string, seen []string, bags map[string]*stackBag, once map[string]bool) (string, error) {
	content, err := e.resolveViewBody(name, seen, bags, once)
	if err != nil {
		return "", err
	}
	content, err = e.expandXTags(content, seen, bags, once, nil)
	if err != nil {
		return "", err
	}
	return expandProps(content), nil
}

func (e *Engine) resolveViewBody(name string, seen []string, bags map[string]*stackBag, once map[string]bool) (string, error) {
	for _, item := range seen {
		if item == name {
			return "", fmt.Errorf("circular template dependency involving [%s]", name)
		}
	}
	seen = append(seen, name)

	raw, err := e.readTemplate(name)
	if err != nil {
		return "", fmt.Errorf("canvas template [%s] not found at %s: %w", name, e.pathFor(name), err)
	}
	content := string(raw)
	if e.escapeMode == rt.EscapeStrict {
		if err = rejectLayoutDirectivesInForbiddenContexts(name, content); err != nil {
			return "", err
		}
	}
	content, err = e.expandIncludes(content, seen, bags, once)
	if err != nil {
		return "", err
	}
	content, err = e.expandComponents(content, seen, bags, once)
	if err != nil {
		return "", err
	}
	content, err = e.expandEach(content, seen, bags, once)
	if err != nil {
		return "", err
	}
	content = applyOnce(content, once)
	content = collectStacks(content, bags)

	if match := reExtends.FindStringSubmatch(content); len(match) == 2 {
		layoutName := match[1]
		parentSections, err := e.buildSectionParentMap(layoutName, seen, bags, once)
		if err != nil {
			return "", err
		}
		sections := applyParentToSections(extractSections(content), parentSections)
		layout, err := e.resolveRootLayout(layoutName, seen, bags, once)
		if err != nil {
			return "", err
		}
		layout = applySectionConditions(layout, sections)
		return applyYields(layout, sections), nil
	}

	return content, nil
}

func (e *Engine) expandIncludes(content string, seen []string, bags map[string]*stackBag, once map[string]bool) (string, error) {
	var firstErr error

	out := reIncludeFirst.ReplaceAllStringFunc(content, func(m string) string {
		match := reIncludeFirst.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		for _, raw := range strings.Split(match[1], ",") {
			name := strings.Trim(strings.TrimSpace(raw), `'"`)
			if name == "" || !e.Exists(name) {
				continue
			}
			partial, err := e.resolveViewWithStacks(name, seen, bags, once)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return "<!-- includeFirst error: " + err.Error() + " -->"
			}
			return partial
		}
		return ""
	})
	if firstErr != nil {
		return out, firstErr
	}

	out = reIncludeWhen.ReplaceAllStringFunc(out, func(m string) string {
		match := reIncludeWhen.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		cond := match[1]
		name := match[2]
		dataExpr := ""
		if len(match) >= 4 {
			dataExpr = match[3]
		}
		return e.wrapConditionalInclude(true, cond, name, dataExpr, seen, bags, once, &firstErr)
	})
	if firstErr != nil {
		return out, firstErr
	}

	out = reIncludeUnless.ReplaceAllStringFunc(out, func(m string) string {
		match := reIncludeUnless.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		cond := match[1]
		name := match[2]
		dataExpr := ""
		if len(match) >= 4 {
			dataExpr = match[3]
		}
		return e.wrapConditionalInclude(false, cond, name, dataExpr, seen, bags, once, &firstErr)
	})
	if firstErr != nil {
		return out, firstErr
	}

	out = reIncludeData.ReplaceAllStringFunc(out, func(m string) string {
		match := reIncludeData.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		partial, err := e.resolveViewWithStacks(match[1], seen, bags, once)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return "<!-- include error: " + err.Error() + " -->"
		}
		dictCall := parseMapExpr(match[2])
		return fmt.Sprintf(`{{ with mergeDict . (%s) }}%s{{ end }}`, dictCall, partial)
	})
	if firstErr != nil {
		return out, firstErr
	}

	out = reIncludeIf.ReplaceAllStringFunc(out, func(m string) string {
		match := reIncludeIf.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		if !e.Exists(match[1]) {
			return ""
		}
		partial, err := e.resolveViewWithStacks(match[1], seen, bags, once)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return "<!-- includeIf error: " + err.Error() + " -->"
		}
		return partial
	})
	if firstErr != nil {
		return out, firstErr
	}

	out = reInclude.ReplaceAllStringFunc(out, func(m string) string {
		match := reInclude.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		partial, err := e.resolveViewWithStacks(match[1], seen, bags, once)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return "<!-- include error: " + err.Error() + " -->"
		}
		return partial
	})
	return out, firstErr
}

func (e *Engine) wrapConditionalInclude(when bool, cond, name, dataExpr string, seen []string, bags map[string]*stackBag, once map[string]bool, firstErr *error) string {
	partial, err := e.resolveViewWithStacks(name, seen, bags, once)
	if err != nil {
		if *firstErr == nil {
			*firstErr = err
		}
		return "<!-- include conditional error: " + err.Error() + " -->"
	}
	if dataExpr != "" {
		dictCall := parseMapExpr(dataExpr)
		partial = fmt.Sprintf(`{{ with mergeDict . (%s) }}%s{{ end }}`, dictCall, partial)
	}
	if when {
		return fmt.Sprintf(`{{ if dataGet . %s }}%s{{ end }}`, tplLit(cond), partial)
	}
	return fmt.Sprintf(`{{ if not (dataGet . %s) }}%s{{ end }}`, tplLit(cond), partial)
}

func (e *Engine) expandComponents(content string, seen []string, bags map[string]*stackBag, once map[string]bool) (string, error) {
	var firstErr error
	out := reComponent.ReplaceAllStringFunc(content, func(m string) string {
		match := reComponent.FindStringSubmatch(m)
		if len(match) < 4 {
			return m
		}
		name := strings.TrimSpace(match[1])
		name = strings.TrimPrefix(name, "components.")
		dataExpr := match[2]
		body := match[3]

		slots := map[string]string{}
		body = reSlot.ReplaceAllStringFunc(body, func(sm string) string {
			smatch := reSlot.FindStringSubmatch(sm)
			if len(smatch) != 3 {
				return sm
			}
			slots[smatch[1]] = strings.TrimSpace(smatch[2])
			return ""
		})
		slots["slot"] = strings.TrimSpace(body)

		for key, val := range slots {
			expanded, err := e.expandXTags(val, seen, bags, once, nil)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				slots[key] = "<!-- x-component error: " + err.Error() + " -->"
				continue
			}
			slots[key] = expanded
		}

		partial, err := e.resolveViewBody("components."+name, seen, bags, once)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return "<!-- component error: " + err.Error() + " -->"
		}
		partial, err = e.expandXTags(partial, seen, bags, once, nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return "<!-- component error: " + err.Error() + " -->"
		}

		inlined, ok, ierr := inlineComponent(e.escapeMode, partial, dataExpr, slots, "")
		if ierr != nil {
			if firstErr == nil {
				firstErr = ierr
			}
			return "<!-- component error: " + ierr.Error() + " -->"
		}
		if ok {
			return inlined
		}
		partial = expandProps(partial)

		args := []string{}
		if strings.TrimSpace(dataExpr) != "" {
			// flatten parseMapExpr dict call into merge
			args = append(args, parseMapExpr(dataExpr))
		} else {
			args = append(args, "dict")
		}
		for key, val := range slots {
			args[0] = strings.TrimSpace(args[0])
			if args[0] == "dict" {
				args[0] = fmt.Sprintf(`dict %q (safeStr %q)`, key, val)
			} else {
				args[0] = fmt.Sprintf(`%s %q (safeStr %q)`, args[0], key, val)
			}
		}
		return fmt.Sprintf(`{{ with mergeDict . (%s) }}%s{{ end }}`, args[0], partial)
	})
	return out, firstErr
}

// inlineComponent substitutes props, slots, @props defaults, and the attribute
// bag into the component source. Literals are escaped for the echo's HTML
// context. The result stays Canvas syntax, so the AOT compiler renders it.
func inlineComponent(mode rt.EscapeMode, partial, dataExpr string, slots map[string]string, attrHTML string) (string, bool, error) {
	body, defaults, ok := peelComponentDirectives(partial)
	if !ok || componentNeedsDict(body) {
		return "", false, nil
	}
	props, ok := simpleComponentProps(dataExpr)
	if !ok {
		return "", false, nil
	}
	for k, v := range defaults {
		if _, exists := props[k]; !exists {
			props[k] = v
		}
	}
	return inlinePrepared(mode, body, props, slots, attrHTML)
}

func inlinePrepared(mode rt.EscapeMode, body string, props map[string]componentProp, slots map[string]string, attrHTML string) (string, bool, error) {
	if strings.Contains(body, "$attributes") && !plainEchoUsesOnly(body, "attributes") {
		return "", false, nil
	}
	names := make([]string, 0, len(props)+len(slots))
	for name := range props {
		names = append(names, name)
	}
	for name := range slots {
		if name == "attributes" {
			continue
		}
		names = append(names, name)
	}
	for _, name := range names {
		if !plainEchoUsesOnly(body, name) {
			return "", false, nil
		}
	}
	out, err := rewriteComponentEchoes(mode, body, props, slots, attrHTML)
	if err != nil {
		return "", false, err
	}
	return out, true, nil
}

func peelComponentDirectives(partial string) (string, map[string]componentProp, bool) {
	partial = strings.TrimSpace(partial)
	defaults := map[string]componentProp{}
	if strings.HasPrefix(partial, "{{ with mergeDefaults . (") {
		body, defs, ok := unwrapMergeDefaults(partial)
		if !ok {
			return "", nil, false
		}
		partial = body
		defaults = defs
	}
	if match := reProps.FindStringSubmatch(partial); len(match) == 2 {
		defs, ok := simpleComponentProps(match[1])
		if !ok {
			return "", nil, false
		}
		for k, v := range defs {
			if _, exists := defaults[k]; !exists {
				defaults[k] = v
			}
		}
		partial = reProps.ReplaceAllString(partial, "")
	}
	partial = reAware.ReplaceAllString(partial, "")
	return strings.TrimSpace(partial), defaults, true
}

func unwrapMergeDefaults(partial string) (string, map[string]componentProp, bool) {
	const open = "{{ with mergeDefaults . ("
	rest := strings.TrimSpace(partial)[len(open):]
	closeArgs := strings.Index(rest, ") }}")
	if closeArgs < 0 {
		return "", nil, false
	}
	defs, ok := parseCompiledDict(strings.TrimSpace(rest[:closeArgs]))
	if !ok {
		return "", nil, false
	}
	body := strings.TrimSpace(rest[closeArgs+len(") }}"):])
	body = strings.TrimSpace(strings.TrimSuffix(body, "{{ end }}"))
	return body, defs, true
}

func parseCompiledDict(src string) (map[string]componentProp, bool) {
	src = strings.TrimSpace(src)
	if src == "dict" {
		return map[string]componentProp{}, true
	}
	if !strings.HasPrefix(src, "dict ") {
		return nil, false
	}
	rest := strings.TrimSpace(src[len("dict "):])
	out := map[string]componentProp{}
	for rest != "" {
		key, next, ok := takeTplString(rest)
		if !ok || !identToken(key) {
			return nil, false
		}
		rest = strings.TrimSpace(next)
		prop, next, ok := takeDictValue(rest)
		if !ok {
			return nil, false
		}
		out[key] = prop
		rest = strings.TrimSpace(next)
	}
	return out, true
}

func takeTplString(s string) (string, string, bool) {
	if s == "" {
		return "", "", false
	}
	if s[0] == '`' {
		end := strings.IndexByte(s[1:], '`')
		if end < 0 {
			return "", "", false
		}
		return s[1 : 1+end], s[2+end:], true
	}
	if s[0] == '"' {
		end := 1
		for end < len(s) {
			if s[end] == '\\' {
				end += 2
				continue
			}
			if s[end] == '"' {
				lit, err := strconv.Unquote(s[:end+1])
				if err != nil {
					return "", "", false
				}
				return lit, s[end+1:], true
			}
			end++
		}
	}
	return "", "", false
}

func takeDictValue(s string) (componentProp, string, bool) {
	if strings.HasPrefix(s, "(dataGet . ") {
		rest := s[len("(dataGet . "):]
		path, next, ok := takeTplString(rest)
		if !ok {
			return componentProp{}, "", false
		}
		next = strings.TrimSpace(next)
		if !strings.HasPrefix(next, ")") {
			return componentProp{}, "", false
		}
		return componentProp{expr: "$" + path}, strings.TrimSpace(next[1:]), true
	}
	if strings.HasPrefix(s, "true") && (len(s) == 4 || s[4] == ' ') {
		return componentProp{lit: "true", isLit: true}, strings.TrimSpace(s[4:]), true
	}
	if strings.HasPrefix(s, "false") && (len(s) == 5 || s[5] == ' ') {
		return componentProp{lit: "false", isLit: true}, strings.TrimSpace(s[5:]), true
	}
	lit, next, ok := takeTplString(s)
	if !ok {
		return componentProp{}, "", false
	}
	return componentProp{lit: lit, isLit: true}, next, true
}

func componentNeedsDict(partial string) bool {
	return strings.Contains(partial, "mergeDict") || strings.Contains(partial, "mergeDefaults") || strings.Contains(partial, "{{ with ") || strings.Contains(partial, "@props") || strings.Contains(partial, "@aware")
}

type componentProp struct {
	expr  string
	lit   string
	isLit bool
}

func simpleComponentProps(expr string) (map[string]componentProp, bool) {
	out := map[string]componentProp{}
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return out, true
	}
	expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expr, "["), "]"))
	if expr == "" {
		return out, true
	}
	for _, part := range splitMapEntries(expr) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=>", 2)
		if len(kv) != 2 {
			return nil, false
		}
		key := strings.Trim(strings.TrimSpace(kv[0]), `'"`)
		if !identToken(key) {
			return nil, false
		}
		val := strings.TrimSpace(kv[1])
		if simpleDollarExpr(val) {
			out[key] = componentProp{expr: val}
			continue
		}
		lit, ok := canvasQuoted(val)
		if !ok {
			return nil, false
		}
		out[key] = componentProp{lit: lit, isLit: true}
	}
	return out, true
}

func simpleDollarExpr(s string) bool {
	if len(s) < 2 || s[0] != '$' {
		return false
	}
	for _, part := range strings.Split(s[1:], ".") {
		if !identToken(part) {
			return false
		}
	}
	return true
}

func identToken(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func canvasQuoted(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return "", false
	}
	q := s[0]
	if (q != '\'' && q != '"') || s[len(s)-1] != q {
		return "", false
	}
	inner := s[1 : len(s)-1]
	if strings.ContainsRune(inner, rune(q)) || strings.Contains(inner, `\`) {
		return "", false
	}
	return inner, true
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// plainEchoUsesOnly reports that every $name in src is exactly {{ $name }} or {!! $name !!}.
func plainEchoUsesOnly(src, name string) bool {
	needle := "$" + name
	for from := 0; from < len(src); {
		i := strings.Index(src[from:], needle)
		if i < 0 {
			return true
		}
		i += from
		if i > 0 && isIdentByte(src[i-1]) {
			from = i + len(needle)
			continue
		}
		end := i + len(needle)
		if end < len(src) && (isIdentByte(src[end]) || src[end] == '.') {
			return false
		}
		if !wrappedPlainEcho(src, i, end) {
			return false
		}
		from = end
	}
	return true
}

func wrappedPlainEcho(src string, dollarAt, end int) bool {
	i := dollarAt
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t') {
		i--
	}
	raw := false
	if i >= 2 && src[i-2:i] == "{{" {
		raw = false
	} else if i >= 3 && src[i-3:i] == "{!!" {
		raw = true
	} else {
		return false
	}
	j := end
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	if raw {
		return j+3 <= len(src) && src[j:j+3] == "!!}"
	}
	return j+2 <= len(src) && src[j:j+2] == "}}"
}

func rewriteComponentEchoes(mode rt.EscapeMode, partial string, props map[string]componentProp, slots map[string]string, attrHTML string) (string, error) {
	var b strings.Builder
	b.Grow(len(partial))
	i := 0
	for i < len(partial) {
		raw := false
		open := 0
		closer := ""
		switch {
		case strings.HasPrefix(partial[i:], "{!!"):
			raw = true
			open = 3
			closer = "!!}"
		case strings.HasPrefix(partial[i:], "{{"):
			open = 2
			closer = "}}"
		default:
			b.WriteByte(partial[i])
			i++
			continue
		}
		rest := partial[i+open:]
		idx := strings.Index(rest, closer)
		if idx < 0 {
			b.WriteString(partial[i:])
			break
		}
		inner := strings.TrimSpace(rest[:idx])
		if strings.HasPrefix(inner, "$") && identToken(inner[1:]) {
			name := inner[1:]
			if name == "attributes" {
				b.WriteString(attrHTML)
				i += open + idx + len(closer)
				continue
			}
			if body, ok := slots[name]; ok {
				b.WriteString(body)
				i += open + idx + len(closer)
				continue
			}
			if prop, ok := props[name]; ok {
				if prop.isLit {
					if raw {
						b.WriteString(prop.lit)
					} else {
						esc, err := rt.EscapeInContext(b.String(), prop.lit, mode)
						if err != nil {
							return "", err
						}
						b.WriteString(esc)
					}
				} else if raw {
					b.WriteString("{!! ")
					b.WriteString(prop.expr)
					b.WriteString(" !!}")
				} else {
					b.WriteString("{{ ")
					b.WriteString(prop.expr)
					b.WriteString(" }}")
				}
				i += open + idx + len(closer)
				continue
			}
		}
		b.WriteString(partial[i : i+open+idx+len(closer)])
		i += open + idx + len(closer)
	}
	return b.String(), nil
}

func (e *Engine) expandEach(content string, seen []string, bags map[string]*stackBag, once map[string]bool) (string, error) {
	var firstErr error
	out := reEach.ReplaceAllStringFunc(content, func(m string) string {
		match := reEach.FindStringSubmatch(m)
		if len(match) < 4 {
			return m
		}
		partialName := match[1]
		collection := match[2]
		alias := match[3]
		emptyName := ""
		if len(match) >= 5 {
			emptyName = match[4]
		}
		partial, err := e.resolveViewWithStacks(partialName, seen, bags, once)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return "<!-- each error: " + err.Error() + " -->"
		}
		partial = rewriteNamedRangeAlias(partial, alias)
		partial = rewriteForeachParentLookups(partial, alias, nil)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf(`{{ if not (empty (dataGet . %s)) }}`, tplLit(collection)))
		sb.WriteString(fmt.Sprintf(`{{ range $__zfi, $%s := dataGet $ "%s" }}`, alias, collection))
		sb.WriteString(partial)
		sb.WriteString(`{{ end }}`)
		if emptyName != "" {
			emptyPartial, err := e.resolveViewWithStacks(emptyName, seen, bags, once)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return "<!-- each empty error: " + err.Error() + " -->"
			}
			sb.WriteString(`{{ else }}`)
			sb.WriteString(emptyPartial)
		}
		sb.WriteString(`{{ end }}`)
		return sb.String()
	})
	return out, firstErr
}

func applyOnce(content string, once map[string]bool) string {
	if once == nil {
		once = map[string]bool{}
	}
	content = reOnceKey.ReplaceAllStringFunc(content, func(m string) string {
		match := reOnceKey.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		key := "once:" + match[1]
		body := strings.TrimSpace(match[2])
		if once[key] {
			return ""
		}
		once[key] = true
		return body
	})
	return reOnce.ReplaceAllStringFunc(content, func(m string) string {
		match := reOnce.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		body := strings.TrimSpace(match[1])
		if once[body] {
			return ""
		}
		once[body] = true
		return body
	})
}

func collectStacks(content string, bags map[string]*stackBag) string {
	content = rePrependOnce.ReplaceAllStringFunc(content, func(m string) string {
		match := rePrependOnce.FindStringSubmatch(m)
		if len(match) < 4 {
			return m
		}
		name := match[1]
		key := match[2]
		body := strings.TrimSpace(match[3])
		if key == "" {
			key = body
		}
		bag := stackBagFor(bags, name)
		if bag.once[name+"|pre|"+key] {
			return ""
		}
		bag.once[name+"|pre|"+key] = true
		bag.prepend = append(bag.prepend, body)
		return ""
	})
	content = rePushOnce.ReplaceAllStringFunc(content, func(m string) string {
		match := rePushOnce.FindStringSubmatch(m)
		if len(match) < 4 {
			return m
		}
		name := match[1]
		key := match[2]
		body := strings.TrimSpace(match[3])
		if key == "" {
			key = body
		}
		bag := stackBagFor(bags, name)
		if bag.once[name+"|push|"+key] {
			return ""
		}
		bag.once[name+"|push|"+key] = true
		bag.append = append(bag.append, body)
		return ""
	})
	content = rePrepend.ReplaceAllStringFunc(content, func(m string) string {
		match := rePrepend.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		bag := stackBagFor(bags, match[1])
		bag.prepend = append(bag.prepend, strings.TrimSpace(match[2]))
		return ""
	})
	content = rePush.ReplaceAllStringFunc(content, func(m string) string {
		match := rePush.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		bag := stackBagFor(bags, match[1])
		bag.append = append(bag.append, strings.TrimSpace(match[2]))
		return ""
	})
	return content
}

func stackBagFor(bags map[string]*stackBag, name string) *stackBag {
	if bag, ok := bags[name]; ok {
		return bag
	}
	bag := &stackBag{once: map[string]bool{}}
	bags[name] = bag
	return bag
}

func applyStacks(content string, bags map[string]*stackBag) string {
	return reStack.ReplaceAllStringFunc(content, func(m string) string {
		match := reStack.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		bag := bags[match[1]]
		if bag == nil {
			if len(match) >= 3 {
				return match[2]
			}
			return ""
		}
		parts := make([]string, 0, len(bag.prepend)+len(bag.append))
		parts = append(parts, bag.prepend...)
		parts = append(parts, bag.append...)
		joined := strings.Join(parts, "\n")
		if joined == "" && len(match) >= 3 {
			return match[2]
		}
		return joined
	})
}

func extractSectionsRegex(content string) map[string]string {
	sections := map[string]string{}
	for _, match := range reSectionShort.FindAllStringSubmatch(content, -1) {
		if len(match) == 3 {
			sections[match[1]] = match[2]
		}
	}
	for _, match := range reSectionShortVar.FindAllStringSubmatch(content, -1) {
		if len(match) == 3 {
			// Keep {{ $var }} so later compile turns it into dataGet.
			sections[match[1]] = "{{ $" + match[2] + " }}"
		}
	}
	for _, match := range reSectionShow.FindAllStringSubmatch(content, -1) {
		if len(match) == 3 {
			sections[match[1]] = strings.TrimSpace(match[2])
		}
	}
	for _, match := range reSection.FindAllStringSubmatch(content, -1) {
		if len(match) == 3 {
			sections[match[1]] = strings.TrimSpace(match[2])
		}
	}
	return sections
}

func applySectionConditions(layout string, sections map[string]string) string {
	out := reHasSection.ReplaceAllStringFunc(layout, func(m string) string {
		match := reHasSection.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		if _, ok := sections[match[1]]; ok {
			return match[2]
		}
		return ""
	})
	out = reSectionMissing.ReplaceAllStringFunc(out, func(m string) string {
		match := reSectionMissing.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		if _, ok := sections[match[1]]; !ok {
			return match[2]
		}
		return ""
	})
	return out
}

func applyYieldsRegex(layout string, sections map[string]string) string {
	out := reYieldDefault.ReplaceAllStringFunc(layout, func(m string) string {
		match := reYieldDefault.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		if value, ok := sections[match[1]]; ok {
			return value
		}
		return match[2]
	})
	out = reYieldDefaultVar.ReplaceAllStringFunc(out, func(m string) string {
		match := reYieldDefaultVar.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		if value, ok := sections[match[1]]; ok {
			return value
		}
		// Keep {{ $var }} so compile applies contextual escape.
		return "{{ $" + match[2] + " }}"
	})
	out = reYield.ReplaceAllStringFunc(out, func(m string) string {
		match := reYield.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		if value, ok := sections[match[1]]; ok {
			return value
		}
		return ""
	})
	// @section … @show is an inline yield-with-default.
	out = reSectionShow.ReplaceAllStringFunc(out, func(m string) string {
		match := reSectionShow.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		if value, ok := sections[match[1]]; ok {
			return value
		}
		return strings.TrimSpace(match[2])
	})
	return out
}

// ViewName converts a filesystem path under the templates root into a dotted name.
func (e *Engine) ViewName(path string) string {
	rel, err := filepath.Rel(e.directory, path)
	if err != nil {
		return filepath.Base(path)
	}
	rel = strings.TrimSuffix(rel, e.extension)
	return strings.ReplaceAll(filepath.ToSlash(rel), "/", ".")
}

// Layout/include/yield/stack call sites must not appear in script, style, tag,
// attribute, or HTML-comment contexts: section/push bodies are authored for text
// but land in the call-site context after expansion.
var reLayoutDirective = regexp.MustCompile(`(?i)@(extends|include|includeIf|includeWhen|includeUnless|includeFirst|component|yield|stack)\s*\(`)

// {{ $slot }} / {!! $slot !!} placement (component default slot).
var reSlotEcho = regexp.MustCompile(`(?i)\{\s*\{\s*\$slot\b|\{\s*!!\s*\$slot\b`)

func rejectLayoutDirectivesInForbiddenContexts(tmplName, content string) error {
	check := func(loc []int) error {
		kind, ctx := rt.ScanEscapeDetail(content[:loc[0]], false, rt.EscapeStrict)
		if !placementContextForbidden(kind, ctx) {
			return nil
		}
		line, col := rt.LineCol(content, loc[0])
		return rt.FormatForbidError(tmplName, line, col, ctx)
	}
	for _, loc := range reLayoutDirective.FindAllStringIndex(content, -1) {
		if err := check(loc); err != nil {
			return err
		}
	}
	for _, loc := range reSlotEcho.FindAllStringIndex(content, -1) {
		if err := check(loc); err != nil {
			return err
		}
	}
	return nil
}

func placementContextForbidden(kind rt.EscapeKind, ctx string) bool {
	if kind == rt.EscForbid {
		return true
	}
	switch ctx {
	case "script":
		return true
	case "style":
		return true
	case "tag-name", "tag":
		return true
	case "attribute-name", "attribute", "attribute-value":
		return true
	case "comment":
		return true
	}
	return strings.HasPrefix(ctx, "attribute:")
}
