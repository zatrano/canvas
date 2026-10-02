package canvas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/rt"
)

// Engine renders Canvas HTML templates.
type Engine struct {
	mu          sync.RWMutex
	directory   string
	extension   string
	environment string
	shared      map[string]any
	funcMap     template.FuncMap
	directives  map[string]func(args string) string
	cache       map[string]*template.Template
	fastCache   map[string]*rt.Compiled
	fastMiss    map[string]struct{}
	cacheOn     bool
	composers   []composerEntry
	escapeMode  rt.EscapeMode
	// langEscapeCatalog when true HTML-escapes @lang catalog strings (tenant-editable catalogs).
	// Default false: catalog markup trusted in text; params always escaped.
	langEscapeCatalog bool
	// preferHTML forces the regex html/template compile path (skip AOT + AST Lower).
	preferHTML bool
	// MaxNestingDepth limits nested @if/@foreach/… (0 → ast.DefaultMaxNestingDepth).
	MaxNestingDepth int
}

// New creates a Canvas template engine rooted at directory.
func New(directory string) *Engine {
	e := &Engine{
		directory:   directory,
		extension:   ".html",
		environment: "local",
		shared:      make(map[string]any),
		funcMap:     defaultFuncs(),
		cache:       make(map[string]*template.Template),
		fastCache:   make(map[string]*rt.Compiled),
		fastMiss:    make(map[string]struct{}),
		cacheOn:     true, // production default: compiled programs stay hot
		escapeMode:  rt.EscapeStrict,
	}
	e.funcMap["viewExists"] = func(name string) bool {
		return e.Exists(name)
	}
	return e
}

// Directory returns the templates root.
func (e *Engine) Directory() string {
	return e.directory
}

// SetExtension sets the template file extension.
func (e *Engine) SetExtension(ext string) {
	e.extension = ext
}

// Share shares data across all templates.
func (e *Engine) Share(key string, value any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shared[key] = value
}

// AddFunc registers a template function.
func (e *Engine) AddFunc(name string, fn any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.funcMap[name] = fn
}

// SetEscapeMode selects Strict (default) or Legacy contextual escaping.
func (e *Engine) SetEscapeMode(mode rt.EscapeMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.escapeMode = mode
	e.cache = make(map[string]*template.Template)
	e.fastCache = make(map[string]*rt.Compiled)
	e.fastMiss = make(map[string]struct{})
}

// SetLangEscapeCatalog controls whether @lang catalog strings are HTML-escaped.
// Default false (HTML in translations works in text). When true,
// the catalog is escaped too (use if tenants can edit translation tables).
// Replacement parameter values are always HTML-escaped regardless of this flag.
func (e *Engine) SetLangEscapeCatalog(escape bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.langEscapeCatalog = escape
	e.cache = make(map[string]*template.Template)
	e.fastCache = make(map[string]*rt.Compiled)
	e.fastMiss = make(map[string]struct{})
}

// LangEscapeCatalog reports whether @lang catalogs are HTML-escaped.
func (e *Engine) LangEscapeCatalog() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.langEscapeCatalog
}

// PreferHTMLPath forces the regex/html-template compile path; for testing and
// path-equivalence checks; output must be identical to the default path.
func (e *Engine) PreferHTMLPath(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.preferHTML = v
	e.cache = make(map[string]*template.Template)
	e.fastCache = make(map[string]*rt.Compiled)
	e.fastMiss = make(map[string]struct{})
}

// EscapeMode returns the current escape mode.
func (e *Engine) EscapeMode() rt.EscapeMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.escapeMode
}

func (e *Engine) nestingLimit() int {
	if e.MaxNestingDepth > 0 {
		return e.MaxNestingDepth
	}
	return ast.DefaultMaxNestingDepth
}

// EnableCache toggles compiled template caching.
func (e *Engine) EnableCache(enabled bool) {
	e.cacheOn = enabled
}

// ClearCache drops compiled templates from memory.
func (e *Engine) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]*template.Template)
	e.fastCache = make(map[string]*rt.Compiled)
	e.fastMiss = make(map[string]struct{})
}

// Exists reports whether a template exists under the engine root.
func (e *Engine) Exists(name string) bool {
	return e.templateExists(name)
}

// Render renders a template to a string.
func (e *Engine) Render(name string, data map[string]any) (string, error) {
	payload := e.mergeData(name, data)

	if prog, err := e.compileFast(name); err != nil {
		return "", err
	} else if prog != nil {
		return prog.Render(payload)
	}

	tmpl, err := e.compileHTML(name)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, payload); err != nil {
		buf.Reset()
		if err2 := tmpl.Execute(&buf, payload); err2 != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

// RenderTo writes into a pooled runtime writer (zero intermediate string when possible).
func (e *Engine) RenderTo(w *rt.Writer, name string, data map[string]any) error {
	payload := e.mergeData(name, data)
	if prog, err := e.compileFast(name); err != nil {
		return err
	} else if prog != nil {
		return prog.Execute(w, payload)
	}
	s, err := e.Render(name, data)
	if err != nil {
		return err
	}
	w.WriteString(s)
	return nil
}

func (e *Engine) mergeData(name string, data map[string]any) map[string]any {
	e.mu.RLock()
	nShared := len(e.shared)
	nComp := len(e.composers)
	e.mu.RUnlock()
	if nShared == 0 && nComp == 0 {
		if data == nil {
			return map[string]any{}
		}
		return promoteSafeHTML(data)
	}

	e.mu.RLock()
	merged := make(map[string]any, nShared+len(data))
	for key, value := range e.shared {
		merged[key] = value
	}
	e.mu.RUnlock()

	for key, value := range data {
		merged[key] = value
	}
	e.applyComposers(name, merged)
	return promoteSafeHTML(merged)
}

func (e *Engine) compileFast(name string) (*rt.Compiled, error) {
	e.mu.RLock()
	if e.preferHTML {
		e.mu.RUnlock()
		return nil, nil
	}
	if e.cacheOn {
		if p, ok := e.fastCache[name]; ok {
			e.mu.RUnlock()
			return p, nil
		}
		if _, miss := e.fastMiss[name]; miss {
			e.mu.RUnlock()
			return nil, nil
		}
	}
	env := e.environment
	e.mu.RUnlock()

	resolved, err := e.resolveView(name, nil)
	if err != nil {
		return nil, fmt.Errorf("canvas template [%s] (%s): %w", name, e.pathFor(name), err)
	}
	if err = checkUnknownClosingDirectives(name, resolved, e.escapeMode); err != nil {
		return nil, err
	}
	doc, err := ast.ParseSourceDepth(resolved, e.nestingLimit())
	if err != nil {
		return nil, fmt.Errorf("canvas template [%s] (%s): %w", name, e.pathFor(name), err)
	}
	prog, ok, err := rt.CompileFuncModeLang(doc, env, e.escapeMode, e.langEscapeCatalog)
	if err != nil {
		return nil, fmt.Errorf("canvas template [%s] (%s) rt compile: %w", name, e.pathFor(name), err)
	}
	if !ok {
		if e.cacheOn {
			e.mu.Lock()
			e.fastMiss[name] = struct{}{}
			e.mu.Unlock()
		}
		return nil, nil
	}
	if e.cacheOn {
		e.mu.Lock()
		e.fastCache[name] = prog
		e.mu.Unlock()
	}
	return prog, nil
}

func (e *Engine) compileHTML(name string) (*template.Template, error) {
	e.mu.RLock()
	if e.cacheOn {
		if cached, ok := e.cache[name]; ok {
			e.mu.RUnlock()
			return cached, nil
		}
	}
	funcMap := template.FuncMap{}
	for k, v := range e.funcMap {
		funcMap[k] = v
	}
	e.mu.RUnlock()
	e.bindEnvironmentFuncs(funcMap)

	resolved, err := e.resolveView(name, nil)
	if err != nil {
		return nil, fmt.Errorf("canvas template [%s] (%s): %w", name, e.pathFor(name), err)
	}
	if err = checkUnknownClosingDirectives(name, resolved, e.escapeMode); err != nil {
		return nil, err
	}

	parsed, err := e.compileView(resolved)
	if err != nil {
		return nil, fmt.Errorf("canvas template [%s] (%s) compile error: %w", name, e.pathFor(name), err)
	}
	// html/template strips HTML comments; rewrite delimiters so comment bodies
	// (including escaped interpolations) survive into the output.
	parsed = preserveHTMLCommentsForGoTemplate(parsed)
	tmpl, err := template.New(name).Funcs(funcMap).Parse(parsed)
	if err != nil {
		return nil, fmt.Errorf("canvas template [%s] (%s) parse error: %w", name, e.pathFor(name), err)
	}

	if e.cacheOn {
		e.mu.Lock()
		e.cache[name] = tmpl
		e.mu.Unlock()
	}

	return tmpl, nil
}

// compileView converts Canvas directives into Go templates.
func (e *Engine) compileView(input string) (string, error) {
	maxNest := e.nestingLimit()
	// Lex/AST front door: catch unclosed {{ / {!! / {{-- early with line info.
	doc, err := ast.ParseSourceDepth(input, maxNest)
	if err != nil {
		return "", err
	}
	e.mu.RLock()
	preferHTML := e.preferHTML
	e.mu.RUnlock()
	// PreferHTMLPath must use the regex pipeline (EscapeStrict + canvasURL),
	// matching the historical html-path probe that blocked AST Lower.
	if !preferHTML {
		if lowered, ok, lerr := ast.Lower(doc); lerr != nil {
			return "", lerr
		} else if ok {
			return lowered, nil
		}
	}

	out := input

	// Preserve verbatim blocks from further compilation (placeholder must not
	// use "@@" — that sequence is the literal-@ escape).
	verbatim := map[string]string{}
	vi := 0
	out = reVerbatim.ReplaceAllStringFunc(out, func(m string) string {
		match := reVerbatim.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		key := fmt.Sprintf("__CANVAS_VERBATIM_%d__", vi)
		vi++
		verbatim[key] = match[1]
		return key
	})

	// @@ → literal @; protect before directive regexes run.
	const atEsc = "\x00CANVAS_AT\x00"
	out = strings.ReplaceAll(out, "@@", atEsc)

	// Custom directives (after verbatim, before built-in compilation).
	out = e.applyCustomDirectives(out)

	// Strip layout directives already resolved.
	out = reExtends.ReplaceAllString(out, "")
	out = reSection.ReplaceAllString(out, "")
	out = reSectionShort.ReplaceAllString(out, "")
	out = reSectionShortVar.ReplaceAllString(out, "")
	out = reSectionShow.ReplaceAllString(out, "")
	out = reYieldDefault.ReplaceAllString(out, "")
	out = reYieldDefaultVar.ReplaceAllString(out, "")
	out = reYield.ReplaceAllString(out, "")
	out = reInclude.ReplaceAllString(out, "")
	out = reIncludeIf.ReplaceAllString(out, "")
	out = reIncludeWhen.ReplaceAllString(out, "")
	out = reIncludeUnless.ReplaceAllString(out, "")
	out = reIncludeFirst.ReplaceAllString(out, "")
	out = reIncludeData.ReplaceAllString(out, "")
	out = reEach.ReplaceAllString(out, "")
	out = reOnce.ReplaceAllString(out, "")
	out = rePushOnce.ReplaceAllString(out, "")
	out = rePrependOnce.ReplaceAllString(out, "")
	out = reComponent.ReplaceAllString(out, "")
	out = reHasSection.ReplaceAllString(out, "")
	out = reSectionMissing.ReplaceAllString(out, "")
	out = reParent.ReplaceAllString(out, "")
	out = reProps.ReplaceAllString(out, "")
	out = reAware.ReplaceAllString(out, "")

	// Comments
	out = replaceAllRegex(out, `\{\{--.*?--\}\}`, "")

	// @foreach / @forelse must rewrite aliases before generic $var compile.
	out = compileForeachBlocks(out)
	out = compileForelseBlocks(out)

	// Attribute bag must compile before generic $var rewrite.
	out = replaceAllRegex(out, `\{\{\s*\$attributes\s*\}\}`, "{{ attributesHTML (dataGet . `attributes`) }}")
	out = replaceAllRegex(out, `\{!!\s*\$attributes\s*!!\}`, "{{ attributesHTML (dataGet . `attributes`) }}")

	// Escape-aware output: simple $path, ternary, ??, function calls, indexing.
	// Shadow scan expands verbatim placeholders to raw bodies (output context).
	var echoErr error
	out, echoErr = compileEchoExpressions(out, e.escapeMode, verbatim)
	if echoErr != nil {
		return "", echoErr
	}

	// @json / @js with contextual escaping
	var jerr error
	out, jerr = compileJSONDirectives(out, e.escapeMode, verbatim)
	if jerr != nil {
		return "", jerr
	}

	// @attrs($map) — only between attributes inside a tag
	var aerr error
	out, aerr = compileAttrsDirectives(out, e.escapeMode, verbatim)
	if aerr != nil {
		return "", aerr
	}

	// @lang — contextual (trusted catalog in text; attr escape; Strict forbid in script/style/tag)
	var lerr error
	out, lerr = compileLangDirectives(out, e.escapeMode, e.langEscapeCatalog, verbatim)
	if lerr != nil {
		return "", lerr
	}

	// @class / @style from map or slice vars
	out = replaceAllRegex(out, `@class\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ classAttr (dataGet . `$1`) }}")
	out = replaceAllRegex(out, `@style\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ styleAttr (dataGet . `$1`) }}")

	// Form boolean attributes (ZPARENT / ZRV before bare $ — foreach alias rewrite).
	for _, attr := range []string{"checked", "selected", "disabled", "readonly", "required"} {
		out = replaceAllRegex(out, `@`+attr+`\s*\(\s*__ZPARENT__\.([a-zA-Z0-9_.]+)\s*\)`, "{{ attrBool (dataGet $ `$1`) `"+attr+"` }}")
		out = replaceAllRegex(out, `@`+attr+`\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\.([a-zA-Z0-9_]+)\s*\)`, "{{ attrBool (dataGet $$$1 `$2`) `"+attr+"` }}")
		out = replaceAllRegex(out, `@`+attr+`\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\s*\)`, "{{ attrBool $$$1 `"+attr+"` }}")
		out = replaceAllRegex(out, `@`+attr+`\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ attrBool (dataGet . `$1`) `"+attr+"` }}")
	}

	// @choice('key', $count)
	out = replaceAllRegex(out, `@choice\s*\(\s*['"]([^'"]+)['"]\s*,\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ choice `$1` (dataGet . `$2`) }}")
	// @choice('key', 3)
	out = replaceAllRegex(out, `@choice\s*\(\s*['"]([^'"]+)['"]\s*,\s*(-?[0-9]+)\s*\)`, "{{ choice `$1` $2 }}")

	// @switch / @case / @default / @break / @endswitch
	out = compileSwitchBlocks(out)

	out = compileIfDirectives(out)
	if err := rejectUnsupportedIfDirectives(out); err != nil {
		return "", err
	}

	out = compileUnlessIssetEmpty(out)

	// Form / auth directives — @csrfMeta before @csrf (prefix collision).
	out = strings.ReplaceAll(out, "@csrfMeta", "<meta name=\"csrf-token\" content=\"{{ dataGet . `_token` }}\">")
	out = strings.ReplaceAll(out, "@csrf", "<input type=\"hidden\" name=\"_token\" value=\"{{ dataGet . `_token` }}\">")
	out = replaceAllRegex(out, `@method\s*\(\s*['"]([^'"]+)['"]\s*\)`, `<input type="hidden" name="_method" value="$1">`)
	out = replaceAllRegex(out, `@old\s*\(\s*['"]([^'"]+)['"]\s*,\s*['"]([^'"]*)['"]\s*\)`, "{{ old . `$1` `$2` }}")
	out = replaceAllRegex(out, `@old\s*\(\s*['"]([^'"]+)['"]\s*\)`, "{{ old . `$1` }}")
	out = replaceAllRegex(out, `@error\s*\(\s*['"]([^'"]+)['"]\s*,\s*['"]([^'"]+)['"]\s*\)`, "{{ if hasError . `$1` `$2` }}")
	out = replaceAllRegex(out, `@error\s*\(\s*['"]([^'"]+)['"]\s*\)`, "{{ if hasError . `$1` }}")
	out = strings.ReplaceAll(out, "@enderror", "{{ end }}")
	out = strings.ReplaceAll(out, "@auth", "{{ if dataGet . `auth` }}")
	out = strings.ReplaceAll(out, "@endauth", "{{ end }}")
	out = strings.ReplaceAll(out, "@guest", "{{ if dataGet . `guest` }}")
	out = strings.ReplaceAll(out, "@endguest", "{{ end }}")

	out = compileCanDirectives(out)
	out = compileEnvDirectives(out)

	out = strings.ReplaceAll(out, atEsc, "@")

	for key, body := range verbatim {
		out = strings.ReplaceAll(out, key, escapeVerbatim(body))
	}

	return restoreRangeVarTokens(out), nil
}

func compileForeachBlocks(input string) string {
	return compileForeachBlocksScoped(input, nil)
}

func compileForeachBlocksScoped(input string, aliases map[string]bool) string {
	const openTag = "@foreach"
	const closeTag = "@endforeach"
	for {
		start := indexFold(input, openTag)
		if start < 0 {
			return input
		}
		headerEnd := strings.Index(input[start:], ")")
		if headerEnd < 0 {
			return input
		}
		headerEnd += start + 1
		header := input[start:headerEnd]
		reHeader := mustCompile(`(?is)@foreach\s*\(\s*\$([a-zA-Z0-9_.]+)\s+as\s+(?:\$([a-zA-Z0-9_]+)\s*=>\s*)?\$([a-zA-Z0-9_]+)\s*\)`)
		match := reHeader.FindStringSubmatch(header)
		if len(match) != 4 {
			return input
		}
		bodyStart := headerEnd
		depth := 1
		i := bodyStart
		end := -1
		for i < len(input) {
			if strings.EqualFold(substrPrefix(input, i, openTag), openTag) {
				depth++
				i += len(openTag)
				continue
			}
			if strings.EqualFold(substrPrefix(input, i, closeTag), closeTag) {
				depth--
				if depth == 0 {
					end = i
					break
				}
				i += len(closeTag)
				continue
			}
			i++
		}
		if end < 0 {
			return input
		}
		path := match[1]
		keyAlias := match[2]
		alias := match[3]
		body := input[bodyStart:end]
		childAliases := copyForeachAliases(aliases)
		childAliases[alias] = true
		if keyAlias != "" {
			childAliases[keyAlias] = true
		}
		// Nested loops see parent aliases (section.pages → $section); root dotted
		// paths stay dataGet $ "pagination.Links".
		body = compileForeachBlocksScoped(body, childAliases)
		coll := foreachCollectionExpr(path, aliases)
		var compiled string
		if keyAlias != "" {
			if len(alias) >= len(keyAlias) {
				body = rewriteNamedRangeAlias(body, alias)
				body = rewriteNamedRangeAlias(body, keyAlias)
			} else {
				body = rewriteNamedRangeAlias(body, keyAlias)
				body = rewriteNamedRangeAlias(body, alias)
			}
			body = rewriteForeachParentLookups(body, alias, aliases)
			compiled = fmt.Sprintf(`{{ range $%s, $%s := %s }}%s{{ end }}`, keyAlias, alias, coll, body)
		} else {
			body = rewriteNamedRangeAlias(body, alias)
			body = rewriteForeachParentLookups(body, alias, aliases)
			compiled = fmt.Sprintf(`{{ range $__zfi, $%s := %s }}%s{{ end }}`, alias, coll, body)
		}
		input = input[:start] + compiled + input[end+len(closeTag):]
	}
}

func copyForeachAliases(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src)+2)
	for k, v := range src {
		if v {
			dst[k] = true
		}
	}
	return dst
}

// foreachCollectionExpr compiles $nav / $pagination.Links / $section.pages.
// If the dotted head is an in-scope range alias, use dataGet $head "rest";
// otherwise treat the full path as root data (dataGet $ "path").
func foreachCollectionExpr(path string, aliases map[string]bool) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return `dataGet $ ""`
	}
	if i := strings.IndexByte(path, '.'); i > 0 {
		head := path[:i]
		rest := path[i+1:]
		if aliases[head] {
			return fmt.Sprintf(`dataGet $%s %s`, head, tplLit(rest))
		}
		return fmt.Sprintf(`dataGet $ %s`, tplLit(path))
	}
	return fmt.Sprintf(`dataGet $ %s`, tplLit(path))
}

func indexFold(s, sub string) int {
	lower := strings.ToLower(s)
	return strings.Index(lower, strings.ToLower(sub))
}

func substrPrefix(s string, i int, prefix string) string {
	if i+len(prefix) > len(s) {
		return ""
	}
	return s[i : i+len(prefix)]
}

func rewriteNamedRangeAlias(body, alias string) string {
	if alias == "" {
		return body
	}
	token := rangeVarToken(alias)
	reFieldBrace := mustCompile(`\{\{\s*\$` + regexp.QuoteMeta(alias) + `\.([a-zA-Z0-9_]+)\s*\}\}`)
	body = reFieldBrace.ReplaceAllString(body, "{{ dataGet "+token+" `$1` }}")
	reRawField := mustCompile(`\{!!\s*\$` + regexp.QuoteMeta(alias) + `\.([a-zA-Z0-9_]+)\s*!!\}`)
	body = reRawField.ReplaceAllString(body, "{{ dataGet "+token+" `$1` }}")
	reWhole := mustCompile(`\{\{\s*\$` + regexp.QuoteMeta(alias) + `\s*\}\}`)
	body = reWhole.ReplaceAllString(body, "{{ "+token+" }}")
	reRawWhole := mustCompile(`\{!!\s*\$` + regexp.QuoteMeta(alias) + `\s*!!\}`)
	body = reRawWhole.ReplaceAllString(body, "{{ "+token+" }}")
	// Remaining $alias.field / $alias (for @if($alias.x), comparisons, etc.)
	reAnyField := mustCompile(`\$` + regexp.QuoteMeta(alias) + `\.([a-zA-Z0-9_]+)`)
	body = reAnyField.ReplaceAllString(body, token+".$1")
	reAnyWhole := mustCompile(`\$` + regexp.QuoteMeta(alias) + `\b`)
	body = reAnyWhole.ReplaceAllString(body, token)
	return body
}

// rewriteForeachParentLookups rewrites root $vars inside a foreach body to read
// from `$` (Execute root). Go keeps `$` stable across range; `.` is the element.
func rewriteForeachParentLookups(body, itemAlias string, parentAliases map[string]bool) string {
	rangeVars := map[string]bool{itemAlias: true, "__zfi": true}
	for _, re := range []*regexp.Regexp{
		mustCompile(`\{\{\s*range\s+\$__zfi,\s*\$([a-zA-Z0-9_]+)\s*:=`),
		mustCompile(`\{\{\s*range\s+\$([a-zA-Z0-9_]+),\s*\$([a-zA-Z0-9_]+)\s*:=`),
	} {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			for i := 1; i < len(m); i++ {
				if m[i] != "" {
					rangeVars[m[i]] = true
				}
			}
		}
	}
	skip := func(path string) bool {
		if path == itemAlias || strings.HasPrefix(path, itemAlias+".") || strings.HasPrefix(path, "__ZRV_") {
			return true
		}
		head, _, _ := strings.Cut(path, ".")
		if rangeVars[head] || parentAliases[head] {
			return true
		}
		return false
	}

	reBrace := mustCompile(`\{\{\s*\$([a-zA-Z0-9_.]+)\s*\}\}`)
	body = reBrace.ReplaceAllStringFunc(body, func(m string) string {
		match := reBrace.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		path := match[1]
		if skip(path) {
			return m
		}
		return fmt.Sprintf(`{{ dataGet $ %s }}`, tplLit(path))
	})
	reRaw := mustCompile(`\{!!\s*\$([a-zA-Z0-9_.]+)\s*!!\}`)
	body = reRaw.ReplaceAllStringFunc(body, func(m string) string {
		match := reRaw.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		path := match[1]
		if skip(path) {
			return m
		}
		return fmt.Sprintf(`{{ safeStr (dataGet $ %s) }}`, tplLit(path))
	})
	// Directive / comparison paths: $foo → __ZPARENT__.foo (compiled later).
	reAny := mustCompile(`\$([a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_]+)*)`)
	body = reAny.ReplaceAllStringFunc(body, func(m string) string {
		path := strings.TrimPrefix(m, "$")
		if skip(path) {
			return m
		}
		if strings.HasPrefix(path, "__ZPARENT__") || path == "__zparent" || path == "__zfi" {
			return m
		}
		return "__ZPARENT__." + path
	})
	body = strings.ReplaceAll(body, "@csrfMeta", "<meta name=\"csrf-token\" content=\"{{ dataGet $ `_token` }}\">")
	body = strings.ReplaceAll(body, "@csrf", "<input type=\"hidden\" name=\"_token\" value=\"{{ dataGet $ `_token` }}\">")
	return body
}

func rangeVarToken(alias string) string {
	return "__ZRV_" + alias + "__"
}

func restoreRangeVarTokens(input string) string {
	re := mustCompile(`__ZRV_([a-zA-Z0-9_]+)__`)
	return re.ReplaceAllStringFunc(input, func(m string) string {
		match := re.FindStringSubmatch(m)
		if len(match) != 2 {
			return m
		}
		return "$" + match[1]
	})
}

func compileForelseBlocks(input string) string {
	re := mustCompile(`(?is)@forelse\s*\(\s*\$([a-zA-Z0-9_.]+)\s+as\s+\$([a-zA-Z0-9_]+)\s*\)(.*?)@endforelse`)
	return re.ReplaceAllStringFunc(input, func(m string) string {
		match := re.FindStringSubmatch(m)
		if len(match) != 4 {
			return m
		}
		path := match[1]
		alias := match[2]
		main, empty := splitForelseEmpty(match[3])
		main = rewriteNamedRangeAlias(main, alias)
		main = rewriteForeachParentLookups(main, alias, nil)
		coll := foreachCollectionExpr(path, nil)
		var b strings.Builder
		b.WriteString(fmt.Sprintf(`{{ if not (empty (%s)) }}{{ range $__zfi, $%s := %s }}%s{{ end }}{{ else }}%s{{ end }}`, coll, alias, coll, main, empty))
		return b.String()
	})
}

func splitForelseEmpty(body string) (main, empty string) {
	idx := 0
	for {
		i := strings.Index(body[idx:], "@empty")
		if i < 0 {
			return strings.TrimSpace(body), ""
		}
		i += idx
		rest := strings.TrimSpace(body[i+len("@empty"):])
		if strings.HasPrefix(rest, "(") {
			idx = i + len("@empty")
			continue
		}
		return strings.TrimSpace(body[:i]), strings.TrimSpace(body[i+len("@empty"):])
	}
}

func compileSwitchBlocks(input string) string {
	re := mustCompile(`(?is)@switch\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)(.*?)@endswitch`)
	return re.ReplaceAllStringFunc(input, func(m string) string {
		match := re.FindStringSubmatch(m)
		if len(match) != 3 {
			return m
		}
		path := match[1]
		body := match[2]
		body = strings.ReplaceAll(body, "@break", "")
		var b strings.Builder
		first := true
		caseRe := mustCompile(`(?is)@case\s*\(\s*(['"][^'"]*['"]|[0-9]+)\s*\)`)
		defaultRe := mustCompile(`(?is)@default`)
		// Split by @case / @default while preserving markers.
		tokens := mustCompile(`(?is)(@case\s*\(\s*(?:['"][^'"]*['"]|[0-9]+)\s*\)|@default)`).Split(body, -1)
		markers := mustCompile(`(?is)(@case\s*\(\s*(?:['"][^'"]*['"]|[0-9]+)\s*\)|@default)`).FindAllString(body, -1)
		if len(markers) == 0 {
			return ""
		}
		for i, marker := range markers {
			content := ""
			if i+1 < len(tokens) {
				content = strings.TrimSpace(tokens[i+1])
			}
			if defaultRe.MatchString(marker) {
				if first {
					b.WriteString(`{{ if false }}`)
					first = false
				}
				b.WriteString(`{{ else }}`)
				b.WriteString(content)
				continue
			}
			cm := caseRe.FindStringSubmatch(marker)
			if len(cm) != 2 {
				continue
			}
			raw := strings.TrimSpace(cm[1])
			var cmp string
			if strings.HasPrefix(raw, "'") || strings.HasPrefix(raw, `"`) {
				cmp = tplLit(strings.Trim(raw, `"'`))
			} else {
				cmp = raw
			}
			if first {
				b.WriteString(fmt.Sprintf("{{ if eq (printf `%%v` (dataGet . %s)) (printf `%%v` %s) }}", tplLit(path), cmp))
				first = false
			} else {
				b.WriteString(fmt.Sprintf("{{ else if eq (printf `%%v` (dataGet . %s)) (printf `%%v` %s) }}", tplLit(path), cmp))
			}
			b.WriteString(content)
		}
		b.WriteString(`{{ end }}`)
		return b.String()
	})
}

func escapeVerbatim(body string) string {
	body = strings.ReplaceAll(body, "{{", "\x00OB\x00")
	body = strings.ReplaceAll(body, "}}", "\x00CB\x00")
	body = strings.ReplaceAll(body, "\x00OB\x00", "{{`{{`}}")
	body = strings.ReplaceAll(body, "\x00CB\x00", "{{`}}`}}")
	return body
}

// preserveHTMLCommentsForGoTemplate rewrites <!-- ... --> so html/template
// does not strip them (stdlib discards HTML comments at parse time).
func preserveHTMLCommentsForGoTemplate(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 32)
	i := 0
	for i < len(s) {
		if !strings.HasPrefix(s[i:], "<!--") {
			b.WriteByte(s[i])
			i++
			continue
		}
		endRel := strings.Index(s[i+4:], "-->")
		if endRel < 0 {
			b.WriteString(s[i:])
			break
		}
		inner := s[i+4 : i+4+endRel]
		b.WriteString(`{{safe "<!--"}}`)
		b.WriteString(inner)
		b.WriteString(`{{safe "-->"}}`)
		i = i + 4 + endRel + 3
	}
	return b.String()
}

func replaceAllRegex(input, pattern, repl string) string {
	re := mustCompile(pattern)
	return re.ReplaceAllString(input, repl)
}

func compileIfDirectives(out string) string {
	var b strings.Builder
	i := 0
	for {
		at := findNextIfCall(out, i)
		if at < 0 {
			b.WriteString(out[i:])
			break
		}
		b.WriteString(out[i:at])
		kind, expr, end, ok := readIfCall(out, at)
		if !ok {
			b.WriteByte(out[at])
			i = at + 1
			continue
		}
		compiled, err := compileIfInner(expr)
		if err != nil {
			b.WriteString(out[at:end])
			i = end
			continue
		}
		if kind == "elseif" {
			b.WriteString("{{ else if ")
			b.WriteString(compiled)
			b.WriteString(" }}")
		} else {
			b.WriteString("{{ if ")
			b.WriteString(compiled)
			b.WriteString(" }}")
		}
		i = end
	}
	out = b.String()
	// @else must not match leftover @elseif (no word boundary between else and if).
	out = replaceAllRegex(out, `@else\b`, `{{ else }}`)
	out = strings.ReplaceAll(out, "@endif", "{{ end }}")
	return out
}

// compileUnlessIssetEmpty compiles @unless / @isset / @empty with the same
// ZPARENT / ZRV / $ precedence as @if (foreach alias rewrite leaves tokens).
func compileUnlessIssetEmpty(out string) string {
	type rule struct {
		pattern string
		repl    string
	}
	rules := []rule{
		{`@unless\s*\(\s*__ZPARENT__\.([a-zA-Z0-9_.]+)\s*\)`, "{{ if not (dataGet $ `$1`) }}"},
		{`@unless\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\.([a-zA-Z0-9_]+)\s*\)`, "{{ if not (dataGet $$$1 `$2`) }}"},
		{`@unless\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\s*\)`, `{{ if not $$$1 }}`},
		{`@unless\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ if not (dataGet . `$1`) }}"},

		{`@isset\s*\(\s*__ZPARENT__\.([a-zA-Z0-9_.]+)\s*\)`, "{{ if issetPath $ `$1` }}"},
		{`@isset\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\.([a-zA-Z0-9_]+)\s*\)`, "{{ if issetPath $$$1 `$2` }}"},
		{`@isset\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\s*\)`, `{{ if $$$1 }}`},
		{`@isset\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ if issetPath . `$1` }}"},

		{`@empty\s*\(\s*__ZPARENT__\.([a-zA-Z0-9_.]+)\s*\)`, "{{ if empty (dataGet $ `$1`) }}"},
		{`@empty\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\.([a-zA-Z0-9_]+)\s*\)`, "{{ if empty (dataGet $$$1 `$2`) }}"},
		{`@empty\s*\(\s*__ZRV_([a-zA-Z0-9_]+)__\s*\)`, `{{ if empty $$$1 }}`},
		{`@empty\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`, "{{ if empty (dataGet . `$1`) }}"},
	}
	for _, r := range rules {
		out = replaceAllRegex(out, r.pattern, r.repl)
	}
	out = strings.ReplaceAll(out, "@endunless", "{{ end }}")
	out = strings.ReplaceAll(out, "@endisset", "{{ end }}")
	out = strings.ReplaceAll(out, "@endempty", "{{ end }}")
	return out
}

func rejectUnsupportedIfDirectives(out string) error {
	re := mustCompile(`(?i)@(?:else)?if\s*\(`)
	loc := re.FindStringIndex(out)
	if loc == nil {
		return nil
	}
	end := loc[1] + 80
	if end > len(out) {
		end = len(out)
	}
	snippet := strings.TrimSpace(out[loc[0]:end])
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	return fmt.Errorf("unsupported @if/@elseif expression: %s", snippet)
}

func defaultFuncs() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"trim":  strings.TrimSpace,
		"join":  strings.Join,
		"safe": func(s string) template.HTML {
			return template.HTML(s)
		},
		"safeStr": safeStr,
		"dataGet": dataGet,
		"json":    toJSON,
		"canvasURL": func(v any) template.HTML {
			return template.HTML(rt.EscapeURLAttr(fmt.Sprint(v), true))
		},
		"canvasURLBlock": func(v any) template.HTML {
			return template.HTML("#unsafe")
		},
		"canvasUnquoted": func(v any) template.HTML {
			return template.HTML(rt.EscapeUnquotedAttr(fmt.Sprint(v)))
		},
		"canvasCSS": func(v any) template.HTML {
			return template.HTML(rt.EscapeCSSValue(fmt.Sprint(v)))
		},
		"canvasURLUnquoted": func(v any) template.HTML {
			s := rt.EscapeURLAttr(fmt.Sprint(v), true)
			if s != "#unsafe" {
				s = rt.EscapeUnquotedAttr(s)
			}
			return template.HTML(s)
		},
		"canvasJSON": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return "null"
			}
			return template.JS(b)
		},
		"canvasJSONAttr": func(v any) template.HTML {
			b, err := json.Marshal(v)
			if err != nil {
				return "null"
			}
			return template.HTML(template.HTMLEscapeString(string(b)))
		},
		"canvasAttrs": func(v any) template.HTMLAttr {
			return template.HTMLAttr(rt.FormatAttrs(v))
		},
		"cmpGt":          cmpGt,
		"cmpGe":          cmpGe,
		"cmpLt":          cmpLt,
		"cmpLe":          cmpLe,
		"numAdd":         numAdd,
		"numSub":         numSub,
		"numMul":         numMul,
		"numDiv":         numDiv,
		"numMod":         numMod,
		"numNeg":         numNeg,
		"dataIndex":      dataIndex,
		"ifCount":        ifCount,
		"ifIsset":        ifIsset,
		"ifNull":         ifNull,
		"ifTernary":      ifTernary,
		"ifElvis":        ifElvis,
		"ifCoalesce":     ifCoalesce,
		"ifXor":          ifXor,
		"strConcat":      strConcat,
		"numPow":         numPow,
		"isNull":         isNull,
		"isNumeric":      isNumeric,
		"isString":       isString,
		"isArray":        isArray,
		"isBool":         isBool,
		"isInt":          isInt,
		"isFloat":        isFloat,
		"isObject":       isObject,
		"isCountable":    isCountable,
		"isScalar":       isScalar,
		"ifFilled":       ifFilled,
		"ifBlank":        ifBlank,
		"inArray":        inArray,
		"arrayKeyExists": arrayKeyExists,
		"strContains":    strContains,
		"strStartsWith":  strStartsWith,
		"strEndsWith":    strEndsWith,
		"ifStrlen":       ifStrlen,
		"ifMbStrlen":     ifMbStrlen,
		"ifLower":        ifLower,
		"ifUpper":        ifUpper,
		"ifTrim":         ifTrim,
		"ifLtrim":        ifLtrim,
		"ifRtrim":        ifRtrim,
		"ifUcfirst":      ifUcfirst,
		"ifLcfirst":      ifLcfirst,
		"ifUcwords":      ifUcwords,
		"ifAbs":          ifAbs,
		"ifRound":        ifRound,
		"ifFloor":        ifFloor,
		"ifCeil":         ifCeil,
		"ifIntval":       ifIntval,
		"ifFloatval":     ifFloatval,
		"toString":       toString,
		"ifBoolval":      ifBoolval,
		"ifMin":          ifMin,
		"ifMax":          ifMax,
		"ifImplode":      ifImplode,
		"ifExplode":      ifExplode,
		"ifStrReplace":   ifStrReplace,
		"ifSubstr":       ifSubstr,
		"ifStrpos":       ifStrpos,
		"classAttr":      classAttr,
		"styleAttr":      styleAttr,
		"attrBool":       attrBool,
		"dict":           dict,
		"mergeDict":      mergeDict,
		"mergeDefaults":  mergeDefaults,
		"isset": func(data map[string]any, key string) bool {
			if data == nil {
				return false
			}
			_, ok := data[key]
			return ok
		},
		"issetPath": func(data any, path string) bool {
			if data == nil || path == "" {
				return false
			}
			parts := strings.Split(path, ".")
			if len(parts) == 1 {
				switch m := data.(type) {
				case map[string]any:
					_, ok := m[parts[0]]
					return ok
				case map[string]string:
					_, ok := m[parts[0]]
					return ok
				default:
					return dataGet(data, path) != nil
				}
			}
			parent := dataGet(data, strings.Join(parts[:len(parts)-1], "."))
			if parent == nil {
				return false
			}
			key := parts[len(parts)-1]
			switch m := parent.(type) {
			case map[string]any:
				_, ok := m[key]
				return ok
			case map[string]string:
				_, ok := m[key]
				return ok
			default:
				return dataGet(data, path) != nil
			}
		},
		"empty": isEmptyValue,
		"old": func(data map[string]any, key string, fallback ...string) string {
			if data == nil {
				if len(fallback) > 0 {
					return fallback[0]
				}
				return ""
			}
			if old, ok := data["old"].(map[string]string); ok {
				if value, exists := old[key]; exists {
					return value
				}
			}
			if len(fallback) > 0 {
				return fallback[0]
			}
			return ""
		},
		"hasError": func(data map[string]any, key string, bagName ...string) bool {
			return hasErrorIn(lookupErrorBag(data, bagName...), key)
		},
		"error": func(data map[string]any, key string, bagName ...string) string {
			return errorIn(lookupErrorBag(data, bagName...), key)
		},
		"trans": func(args ...any) string {
			if len(args) == 0 {
				return ""
			}
			if len(args) == 1 {
				return fmt.Sprint(args[0])
			}
			// locale, key [, replace map]
			return fmt.Sprint(args[1])
		},
		"canvasTrans": func(data map[string]any, key string, repl ...any) string {
			msg := key
			if data != nil {
				if m, ok := data["__trans"].(map[string]string); ok {
					if v, ok := m[key]; ok {
						msg = v
					}
				}
			}
			var replMap map[string]any
			if len(repl) > 0 {
				if m, ok := repl[0].(map[string]any); ok {
					replMap = m
				}
			}
			return rt.FormatLang(msg, replMap)
		},
		"can": func(data map[string]any, ability string, args ...any) bool {
			if data == nil {
				return false
			}
			if fn, ok := data["__can"].(func(string, ...any) bool); ok && fn != nil {
				return fn(ability, args...)
			}
			return false
		},
		"attributesBag":  attributesBag,
		"attributesHTML": attributesHTML,
		"env": func(name string) bool {
			return name == "local"
		},
		"production": func() bool {
			return false
		},
		"viewExists": func(name string) bool {
			return false
		},
	}
}

func lookupErrorBag(data map[string]any, bagName ...string) any {
	if data == nil {
		return nil
	}
	if len(bagName) > 0 && bagName[0] != "" {
		name := bagName[0]
		if bags, ok := data["errorBags"].(map[string]any); ok {
			if bag, exists := bags[name]; exists {
				return bag
			}
		}
		if bag, ok := data[name]; ok {
			return bag
		}
		return nil
	}
	return data["errors"]
}

func hasErrorIn(bag any, key string) bool {
	switch b := bag.(type) {
	case interface{ Has(string) bool }:
		return b.Has(key)
	case map[string]string:
		return b[key] != ""
	case map[string][]string:
		return len(b[key]) > 0
	default:
		return false
	}
}

func errorIn(bag any, key string) string {
	switch b := bag.(type) {
	case interface{ First(string) string }:
		return b.First(key)
	case map[string]string:
		return b[key]
	case map[string][]string:
		if len(b[key]) > 0 {
			return b[key][0]
		}
	}
	return ""
}

func (e *Engine) bindEnvironmentFuncs(funcMap template.FuncMap) {
	env := e.environmentName()
	funcMap["env"] = func(name string) bool {
		return env == name
	}
	funcMap["production"] = func() bool {
		return env == "production"
	}
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) == ""
	case bool:
		return !x
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.Chan:
		return rv.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

var reJSONDir = regexp.MustCompile(`@(json|js)\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`)

// Locale always from Execute root (`$`): inside @foreach `.` is the item and has no locale.
var (
	reLangReplLit = regexp.MustCompile(`@lang\s*\(\s*['"]([^'"]+)['"]\s*,\s*\[\s*['"]([^'"]+)['"]\s*=>\s*['"]([^'"]*)['"]\s*\]\s*\)`)
	reLangReplVar = regexp.MustCompile(`@lang\s*\(\s*['"]([^'"]+)['"]\s*,\s*\[\s*['"]([^'"]+)['"]\s*=>\s*\$([a-zA-Z0-9_.]+)\s*\]\s*\)`)
	reLangBare    = regexp.MustCompile(`@lang\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

func compileLangDirectives(out string, mode rt.EscapeMode, escapeCatalog bool, verbatim map[string]string) (string, error) {
	expand := func(s string) string {
		for k, v := range verbatim {
			s = strings.ReplaceAll(s, k, v)
		}
		return s
	}
	type hit struct {
		start, end int
		call       string // canvasTrans … args without braces
	}
	var hits []hit
	add := func(re *regexp.Regexp, build func([]string) string) {
		for _, m := range re.FindAllStringSubmatchIndex(out, -1) {
			subs := make([]string, (len(m)/2)-1)
			for i := 1; i < len(m)/2; i++ {
				if m[2*i] >= 0 {
					subs[i-1] = out[m[2*i]:m[2*i+1]]
				}
			}
			hits = append(hits, hit{start: m[0], end: m[1], call: build(subs)})
		}
	}
	add(reLangReplLit, func(s []string) string {
		return fmt.Sprintf("canvasTrans $ `%s` (dict `%s` `%s`)", s[0], s[1], s[2])
	})
	add(reLangReplVar, func(s []string) string {
		return fmt.Sprintf("canvasTrans $ `%s` (dict `%s` (dataGet . `%s`))", s[0], s[1], s[2])
	})
	add(reLangBare, func(s []string) string {
		return fmt.Sprintf("canvasTrans $ `%s`", s[0])
	})
	if len(hits) == 0 {
		return out, nil
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].start < hits[j].start })
	var b strings.Builder
	last := 0
	for _, h := range hits {
		if h.start < last {
			continue // overlapping (shouldn't)
		}
		b.WriteString(out[last:h.start])
		kind, ctx := rt.ScanEscapeDetail(expand(out[:h.start]), false, mode)
		if kind == rt.EscForbid {
			line, col := rt.LineCol(out, h.start)
			return "", rt.FormatForbidError("", line, col, ctx)
		}
		switch kind {
		case rt.EscURLAttr:
			b.WriteString("{{ canvasURL (")
			b.WriteString(h.call)
			b.WriteString(") }}")
		case rt.EscURLBlock:
			b.WriteString("{{ canvasURLBlock (")
			b.WriteString(h.call)
			b.WriteString(") }}")
		case rt.EscUnquoted:
			b.WriteString("{{ canvasUnquoted (")
			b.WriteString(h.call)
			b.WriteString(") }}")
		case rt.EscCSS:
			b.WriteString("{{ canvasCSS (")
			b.WriteString(h.call)
			b.WriteString(") }}")
		case rt.EscURLUnquoted:
			b.WriteString("{{ canvasURLUnquoted (")
			b.WriteString(h.call)
			b.WriteString(") }}")
		default:
			if rt.LangTrustedText(ctx, escapeCatalog) {
				b.WriteString("{{ safeStr (")
				b.WriteString(h.call)
				b.WriteString(") }}")
			} else {
				b.WriteString("{{ ")
				b.WriteString(h.call)
				b.WriteString(" }}")
			}
		}
		last = h.end
	}
	b.WriteString(out[last:])
	return b.String(), nil
}

func compileJSONDirectives(out string, mode rt.EscapeMode, verbatim map[string]string) (string, error) {
	expand := func(s string) string {
		for k, v := range verbatim {
			s = strings.ReplaceAll(s, k, v)
		}
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range reJSONDir.FindAllStringSubmatchIndex(out, -1) {
		b.WriteString(out[last:loc[0]])
		name := out[loc[2]:loc[3]]
		path := out[loc[4]:loc[5]]
		kind, ctx := rt.ScanEscapeDetail(expand(out[:loc[0]]), true, mode)
		if name == "js" && kind == rt.EscHTML {
			kind = rt.EscJSON
		}
		if kind == rt.EscForbid {
			line, col := rt.LineCol(out, loc[0])
			return "", rt.FormatForbidError("", line, col, ctx)
		}
		fn := "canvasJSON"
		if kind == rt.EscJSONAttr {
			fn = "canvasJSONAttr"
		}
		b.WriteString("{{ ")
		b.WriteString(fn)
		b.WriteString(" (dataGet . `")
		b.WriteString(path)
		b.WriteString("`) }}")
		last = loc[1]
	}
	b.WriteString(out[last:])
	return b.String(), nil
}

var reAttrsDir = regexp.MustCompile(`@attrs\s*\(\s*\$([a-zA-Z0-9_.]+)\s*\)`)

func compileAttrsDirectives(out string, mode rt.EscapeMode, verbatim map[string]string) (string, error) {
	expand := func(s string) string {
		for k, v := range verbatim {
			s = strings.ReplaceAll(s, k, v)
		}
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range reAttrsDir.FindAllStringSubmatchIndex(out, -1) {
		b.WriteString(out[last:loc[0]])
		path := out[loc[2]:loc[3]]
		prefix := expand(out[:loc[0]])
		if mode == rt.EscapeStrict {
			ok, ctx := rt.AttrsPositionOK(prefix)
			if !ok {
				line, col := rt.LineCol(out, loc[0])
				return "", rt.FormatForbidError("", line, col, ctx)
			}
		}
		b.WriteString("{{ canvasAttrs (dataGet . `")
		b.WriteString(path)
		b.WriteString("`) }}")
		last = loc[1]
	}
	b.WriteString(out[last:])
	return b.String(), nil
}
