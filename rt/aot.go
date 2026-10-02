package rt

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/zatrano/canvas/ast"
)

// RenderFunc is a fully specialized template — no opcode dispatch, no Ctx alloc.
type RenderFunc func(w *Writer, root map[string]any)

// Compiled is a cached native renderer.
type Compiled struct {
	Fn   RenderFunc
	Hint int
	Env  string
}

func (c *Compiled) Execute(w *Writer, root map[string]any) error {
	if c == nil || c.Fn == nil {
		return nil
	}
	c.Fn(w, root)
	return nil
}

func (c *Compiled) Render(root map[string]any) (string, error) {
	w := AcquireWriter()
	defer ReleaseWriter(w)
	if c.Hint > 0 && cap(w.b) < c.Hint {
		w.b = make([]byte, 0, c.Hint)
	}
	c.Fn(w, root)
	return string(w.Bytes()), nil
}

// CompileFunc lowers an AST-capable document to a single native RenderFunc (Strict escape).
func CompileFunc(doc *ast.Document, env string) (c *Compiled, ok bool, err error) {
	return CompileFuncMode(doc, env, EscapeStrict)
}

// CompileFuncMode is CompileFunc with an explicit escape mode.
func CompileFuncMode(doc *ast.Document, env string, mode EscapeMode) (c *Compiled, ok bool, err error) {
	return CompileFuncModeLang(doc, env, mode, false)
}

// CompileFuncModeLang is CompileFuncMode with @lang catalog escape policy.
// escapeCatalog=false (default): catalog HTML trusted in text; always escaped in attributes.
func CompileFuncModeLang(doc *ast.Document, env string, mode EscapeMode, escapeCatalog bool) (c *Compiled, ok bool, err error) {
	if doc == nil {
		return &Compiled{Fn: func(w *Writer, root map[string]any) {}, Env: env}, true, nil
	}
	if !doc.CanASTLower() {
		return nil, false, nil
	}
	fn, hint, err := buildFunc(doc.Nodes, env, mode, escapeCatalog)
	if err != nil {
		return nil, false, err
	}
	return &Compiled{Fn: fn, Hint: hint + 256, Env: env}, true, nil
}

func buildFunc(nodes []ast.Node, env string, mode EscapeMode, escapeCatalog bool) (RenderFunc, int, error) {
	if fn, hint, ok := tryFuseListPage(nodes); ok {
		return fn, hint, nil
	}
	parts := make([]RenderFunc, 0, len(nodes))
	hint := 0
	var prefix strings.Builder
	for _, n := range nodes {
		fn, h, err := buildNodeMode(n, env, mode, escapeCatalog, &prefix)
		if err != nil {
			return nil, 0, err
		}
		if fn != nil {
			parts = append(parts, fn)
			hint += h
		}
	}
	switch len(parts) {
	case 0:
		return func(w *Writer, root map[string]any) {}, 0, nil
	case 1:
		return parts[0], hint, nil
	default:
		return func(w *Writer, root map[string]any) {
			for i := 0; i < len(parts); i++ {
				parts[i](w, root)
			}
		}, hint, nil
	}
}

func buildNodeMode(n ast.Node, env string, mode EscapeMode, escapeCatalog bool, prefix *strings.Builder) (RenderFunc, int, error) {
	switch x := n.(type) {
	case ast.Text:
		if x.Value == "" {
			return nil, 0, nil
		}
		prefix.WriteString(x.Value)
		static := []byte(x.Value)
		return func(w *Writer, root map[string]any) { w.Write(static) }, len(static), nil
	case ast.Comment:
		return nil, 0, nil
	case ast.Echo:
		kind, ctx := ScanEscapeDetail(prefix.String(), false, mode)
		if kind == EscForbid {
			return nil, 0, FormatForbidError("", 0, 0, ctx)
		}
		path := SplitPath(x.Expr)
		prefix.WriteString(InterpMarker)
		k := kind
		return func(w *Writer, root map[string]any) {
			writeByKind(w, wLookup(w, root, path), k)
		}, 16, nil
	case ast.RawEcho:
		path := SplitPath(x.Expr)
		prefix.WriteString(InterpMarker)
		return func(w *Writer, root map[string]any) {
			writeRaw(w, wLookup(w, root, path))
		}, 16, nil
	case ast.Directive:
		if x.Name == "json" || x.Name == "js" {
			kind, ctx := ScanEscapeDetail(prefix.String(), true, mode)
			if x.Name == "js" && kind == EscHTML {
				kind = EscJSON
			}
			if kind == EscForbid {
				return nil, 0, FormatForbidError("", 0, 0, ctx)
			}
			path := SplitPath(x.Args)
			prefix.WriteString(InterpMarker)
			k := kind
			return func(w *Writer, root map[string]any) {
				writeByKind(w, wLookup(w, root, path), k)
			}, 16, nil
		}
		if x.Name == "lang" {
			return buildLangDirective(x, mode, escapeCatalog, prefix)
		}
		return buildDirective(x)
	case ast.Block:
		return buildBlock(x, env, mode, escapeCatalog)
	default:
		return nil, 0, fmt.Errorf("canvas aot: unsupported node")
	}
}

func buildLangDirective(d ast.Directive, mode EscapeMode, escapeCatalog bool, prefix *strings.Builder) (RenderFunc, int, error) {
	kind, ctxName := ScanEscapeDetail(prefix.String(), false, mode)
	if kind == EscForbid {
		return nil, 0, FormatForbidError("", 0, 0, ctxName)
	}
	key := strings.Trim(d.Args, `'" `)
	prefix.WriteString(InterpMarker)
	k, ctx, escCat := kind, ctxName, escapeCatalog
	return func(w *Writer, root map[string]any) {
		msg := FormatLang(transRoot(root, key), nil)
		WriteLang(w, msg, k, ctx, escCat)
	}, 8, nil
}

// tryFuseListPage collapses the common SSR shape into one closure:
//
//	[text] {{ $title }} [text] @foreach($items as $item) … $item.field … @endforeach [text]
//
// One map resolve for title + items; zero per-node calls.
func tryFuseListPage(nodes []ast.Node) (RenderFunc, int, bool) {
	type seg struct {
		kind   byte // 0 static, 1 echo key, 2 foreach
		static []byte
		key    string
		coll   []string
		loop   []inlineSeg
	}
	segs := make([]seg, 0, len(nodes))
	hint := 0
	for _, n := range nodes {
		switch x := n.(type) {
		case ast.Comment:
			continue
		case ast.Text:
			if x.Value == "" {
				continue
			}
			b := []byte(x.Value)
			segs = append(segs, seg{kind: 0, static: b})
			hint += len(b)
		case ast.Echo:
			path := SplitPath(x.Expr)
			if len(path) != 1 {
				return nil, 0, false
			}
			segs = append(segs, seg{kind: 1, key: path[0]})
			hint += 16
		case ast.Block:
			if x.Name != "foreach" {
				return nil, 0, false
			}
			coll, _, valAlias, ok := parseForeach(x.Args)
			if !ok {
				return nil, 0, false
			}
			loop, ok := parseInlineSegs(x.Body, valAlias)
			if !ok {
				return nil, 0, false
			}
			segs = append(segs, seg{kind: 2, coll: SplitPath("$" + coll), loop: loop})
			hint += 64
		default:
			return nil, 0, false
		}
	}
	if len(segs) == 0 {
		return nil, 0, false
	}
	hasLoop := false
	for i := range segs {
		if segs[i].kind == 2 {
			hasLoop = true
			break
		}
	}
	if !hasLoop {
		return nil, 0, false
	}
	return func(w *Writer, root map[string]any) {
		for i := 0; i < len(segs); i++ {
			s := &segs[i]
			switch s.kind {
			case 0:
				w.Write(s.static)
			case 1:
				if str, ok := root[s.key].(string); ok {
					writeEscapedString(w, str)
				} else {
					writeEscaped(w, root[s.key])
				}
			case 2:
				raw := wLookup(w, root, s.coll)
				items, ok := raw.([]map[string]any)
				if !ok {
					continue
				}
				loop := s.loop
				if len(loop) == 3 && !loop[0].isField && loop[1].isField && !loop[1].raw && !loop[2].isField {
					pre, field, post := loop[0].static, loop[1].field, loop[2].static
					for j := 0; j < len(items); j++ {
						w.Write(pre)
						if str, ok := items[j][field].(string); ok {
							writeEscapedString(w, str)
						} else {
							writeEscaped(w, items[j][field])
						}
						w.Write(post)
					}
					continue
				}
				for j := 0; j < len(items); j++ {
					it := items[j]
					for k := 0; k < len(loop); k++ {
						seg := &loop[k]
						if !seg.isField {
							w.Write(seg.static)
							continue
						}
						v := it[seg.field]
						if seg.raw {
							writeRaw(w, v)
						} else if str, ok := v.(string); ok {
							writeEscapedString(w, str)
						} else {
							writeEscaped(w, v)
						}
					}
				}
			}
		}
	}, hint, true
}

func parseInlineSegs(body []ast.Node, alias string) ([]inlineSeg, bool) {
	segs := make([]inlineSeg, 0, len(body))
	for _, n := range body {
		switch x := n.(type) {
		case ast.Text:
			segs = append(segs, inlineSeg{static: []byte(x.Value)})
		case ast.Comment:
			continue
		case ast.Echo:
			path := SplitPath(x.Expr)
			if len(path) != 2 || path[0] != alias {
				return nil, false
			}
			segs = append(segs, inlineSeg{field: path[1], isField: true})
		case ast.RawEcho:
			path := SplitPath(x.Expr)
			if len(path) != 2 || path[0] != alias {
				return nil, false
			}
			segs = append(segs, inlineSeg{field: path[1], raw: true, isField: true})
		default:
			return nil, false
		}
	}
	return segs, true
}

func buildDirective(d ast.Directive) (RenderFunc, int, error) {
	switch d.Name {
	case "csrf":
		return func(w *Writer, root map[string]any) {
			tok, _ := root["_token"].(string)
			w.WriteString(`<input type="hidden" name="_token" value="`)
			writeEscapedString(w, tok)
			w.WriteString(`">`)
		}, 64, nil
	case "csrfMeta":
		return func(w *Writer, root map[string]any) {
			tok, _ := root["_token"].(string)
			w.WriteString(`<meta name="csrf-token" content="`)
			writeEscapedString(w, tok)
			w.WriteString(`">`)
		}, 64, nil
	case "method":
		m := strings.Trim(d.Args, `'" `)
		return func(w *Writer, root map[string]any) {
			w.WriteString(`<input type="hidden" name="_method" value="`)
			writeEscapedString(w, m)
			w.WriteString(`">`)
		}, 48, nil
	case "json":
		path := SplitPath(d.Args)
		return func(w *Writer, root map[string]any) { writeJSON(w, wLookup(w, root, path)) }, 16, nil
	case "class":
		path := SplitPath(d.Args)
		return func(w *Writer, root map[string]any) { writeClassAttr(w, wLookup(w, root, path)) }, 16, nil
	case "style":
		path := SplitPath(d.Args)
		return func(w *Writer, root map[string]any) { writeStyleAttr(w, wLookup(w, root, path)) }, 16, nil
	case "checked", "selected", "disabled", "readonly", "required":
		path := SplitPath(d.Args)
		name := d.Name
		return func(w *Writer, root map[string]any) {
			if truthy(wLookup(w, root, path)) {
				w.AppendByte(' ')
				w.WriteString(name)
			}
		}, 8, nil
	case "old":
		parts := strings.Split(d.Args, ",")
		key := strings.Trim(parts[0], `'" `)
		def := ""
		if len(parts) > 1 {
			def = strings.Trim(parts[1], `'" `)
		}
		return func(w *Writer, root map[string]any) {
			writeEscaped(w, oldRoot(root, key, def))
		}, 8, nil
	case "choice":
		parts := strings.Split(d.Args, ",")
		key := strings.Trim(parts[0], `'" `)
		return func(w *Writer, root map[string]any) {
			writeEscaped(w, transRoot(root, key))
		}, 8, nil
	default:
		return nil, 0, fmt.Errorf("canvas aot: unsupported @%s", d.Name)
	}
}

func buildBlock(b ast.Block, env string, mode EscapeMode, escapeCatalog bool) (RenderFunc, int, error) {
	switch b.Name {
	case "foreach":
		return buildForeach(b, env, mode, escapeCatalog, false)
	case "forelse":
		return buildForeach(b, env, mode, escapeCatalog, true)
	case "if":
		return buildIfAOT(b, env, mode, escapeCatalog, true)
	case "unless":
		return buildIfAOT(b, env, mode, escapeCatalog, false)
	case "isset":
		path := SplitPath(b.Args)
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if wIsset(w, root, path) {
				body(w, root)
			}
		}, h, nil
	case "empty":
		path := SplitPath(b.Args)
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if isEmpty(wLookup(w, root, path)) {
				body(w, root)
			}
		}, h, nil
	case "auth":
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if truthy(root["auth"]) {
				body(w, root)
			}
		}, h, nil
	case "guest":
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if truthy(root["guest"]) {
				body(w, root)
			}
		}, h, nil
	case "error":
		key := strings.Trim(b.Args, `'" `)
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if hasErrorRoot(root, key) {
				body(w, root)
			}
		}, h, nil
	case "can":
		ability := strings.Trim(b.Args, `'" `)
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if canRoot(root, ability) {
				body(w, root)
			}
		}, h, nil
	case "cannot":
		ability := strings.Trim(b.Args, `'" `)
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if !canRoot(root, ability) {
				body(w, root)
			}
		}, h, nil
	case "env":
		name := strings.Trim(b.Args, `'" `)
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if env == name {
				body(w, root)
			}
		}, h, nil
	case "production":
		body, h, err := buildFunc(b.Body, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
		return func(w *Writer, root map[string]any) {
			if env == "production" {
				body(w, root)
			}
		}, h, nil
	default:
		return nil, 0, fmt.Errorf("canvas aot: unsupported block @%s", b.Name)
	}
}

func buildForeach(b ast.Block, env string, mode EscapeMode, escapeCatalog bool, forelse bool) (RenderFunc, int, error) {
	coll, keyAlias, valAlias, ok := parseForeach(b.Args)
	if !ok {
		return nil, 0, fmt.Errorf("canvas aot: bad foreach")
	}
	bodyNodes := b.Body
	var emptyNodes []ast.Node
	if forelse {
		var okSplit bool
		bodyNodes, emptyNodes, okSplit = splitEmpty(bodyNodes)
		if !okSplit {
			return nil, 0, fmt.Errorf("canvas aot: forelse missing @empty")
		}
	}
	var emptyFn RenderFunc
	eh := 0
	var err error
	if forelse {
		emptyFn, eh, err = buildFunc(emptyNodes, env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
	}
	collPath := SplitPath("$" + coll)

	if inl, ok := tryInlineMapLoop(bodyNodes, valAlias, collPath, emptyFn); ok {
		return inl, eh + 64, nil
	}

	body, bh, err := buildFunc(bodyNodes, env, mode, escapeCatalog)
	if err != nil {
		return nil, 0, err
	}
	ka, va := keyAlias, valAlias
	return func(w *Writer, root map[string]any) {
		raw := wLookup(w, root, collPath)
		if forelse && isEmpty(raw) {
			if emptyFn != nil {
				emptyFn(w, root)
			}
			return
		}
		if items, ok := raw.([]map[string]any); ok {
			if len(items) == 0 {
				if emptyFn != nil {
					emptyFn(w, root)
				}
				return
			}
			runMapSliceW(w, root, ka, va, items, body)
			return
		}
		runGenericRangeW(w, root, ka, va, raw, body, emptyFn)
	}, bh + eh + 64, nil
}

type inlineSeg struct {
	static  []byte
	field   string
	raw     bool
	isField bool
}

func tryInlineMapLoop(body []ast.Node, alias string, collPath []string, emptyFn RenderFunc) (RenderFunc, bool) {
	segs, ok := parseInlineSegs(body, alias)
	if !ok {
		return nil, false
	}
	cp := append([]string(nil), collPath...)
	// Specialize the common `<li>{{ $item.name }}</li>` shape (3 segs).
	if len(segs) == 3 && !segs[0].isField && segs[1].isField && !segs[1].raw && !segs[2].isField {
		pre, field, post := segs[0].static, segs[1].field, segs[2].static
		return func(w *Writer, root map[string]any) {
			raw := wLookup(w, root, cp)
			items, ok := raw.([]map[string]any)
			if !ok {
				if emptyFn != nil && isEmpty(raw) {
					emptyFn(w, root)
				}
				return
			}
			n := len(items)
			if n == 0 {
				if emptyFn != nil {
					emptyFn(w, root)
				}
				return
			}
			for i := 0; i < n; i++ {
				w.Write(pre)
				if str, ok := items[i][field].(string); ok {
					writeEscapedString(w, str)
				} else {
					writeEscaped(w, items[i][field])
				}
				w.Write(post)
			}
		}, true
	}
	return func(w *Writer, root map[string]any) {
		raw := wLookup(w, root, cp)
		items, ok := raw.([]map[string]any)
		if !ok {
			if emptyFn != nil && isEmpty(raw) {
				emptyFn(w, root)
			}
			return
		}
		n := len(items)
		if n == 0 {
			if emptyFn != nil {
				emptyFn(w, root)
			}
			return
		}
		for i := 0; i < n; i++ {
			it := items[i]
			for s := 0; s < len(segs); s++ {
				seg := &segs[s]
				if !seg.isField {
					w.Write(seg.static)
					continue
				}
				v := it[seg.field]
				if seg.raw {
					writeRaw(w, v)
				} else if str, ok := v.(string); ok {
					writeEscapedString(w, str)
				} else {
					writeEscaped(w, v)
				}
			}
		}
	}, true
}

func runMapSliceW(w *Writer, root map[string]any, keyAlias, valAlias string, items []map[string]any, body RenderFunc) {
	if w.aliases == nil && !w.hasRV {
		prev := w.hasRV
		prevRV, prevRK, prevRI, prevRKI := w.rv, w.rk, w.ri, w.rki
		w.hasRV = true
		w.rk = valAlias
		w.rki = keyAlias
		for i := 0; i < len(items); i++ {
			if keyAlias != "" {
				w.ri = i
			}
			w.rv = items[i]
			body(w, root)
		}
		w.hasRV = prev
		w.rv, w.rk, w.ri, w.rki = prevRV, prevRK, prevRI, prevRKI
		return
	}
	// nested
	prev := w.aliases
	child := make(map[string]any, 4)
	for k, v := range prev {
		child[k] = v
	}
	if w.hasRV {
		child[w.rk] = w.rv
		if w.rki != "" {
			child[w.rki] = w.ri
		}
		w.hasRV = false
	}
	w.aliases = child
	for i := 0; i < len(items); i++ {
		if keyAlias != "" {
			child[keyAlias] = i
		}
		child[valAlias] = items[i]
		body(w, root)
	}
	w.aliases = prev
}

func runGenericRangeW(w *Writer, root map[string]any, keyAlias, valAlias string, coll any, body, emptyFn RenderFunc) {
	if coll == nil || isEmpty(coll) {
		if emptyFn != nil {
			emptyFn(w, root)
		}
		return
	}
	switch c := coll.(type) {
	case []any:
		runAnySliceW(w, root, keyAlias, valAlias, c, body)
	case []string:
		for i := 0; i < len(c); i++ {
			_ = i
		}
		items := make([]any, len(c))
		for i := range c {
			items[i] = c[i]
		}
		runAnySliceW(w, root, keyAlias, valAlias, items, body)
	default:
		rv := reflect.ValueOf(coll)
		for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
			if rv.IsNil() {
				return
			}
			rv = rv.Elem()
		}
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			n := rv.Len()
			items := make([]any, n)
			for i := 0; i < n; i++ {
				items[i] = rv.Index(i).Interface()
			}
			runAnySliceW(w, root, keyAlias, valAlias, items, body)
		}
	}
}

func runAnySliceW(w *Writer, root map[string]any, keyAlias, valAlias string, items []any, body RenderFunc) {
	if w.aliases == nil && !w.hasRV {
		prevRV, prevRK, prevRI, prevRKI := w.rv, w.rk, w.ri, w.rki
		w.hasRV = true
		w.rk = valAlias
		w.rki = keyAlias
		for i := 0; i < len(items); i++ {
			if keyAlias != "" {
				w.ri = i
			}
			w.rv = items[i]
			body(w, root)
		}
		w.hasRV = false
		w.rv, w.rk, w.ri, w.rki = prevRV, prevRK, prevRI, prevRKI
		return
	}
	prev := w.aliases
	child := make(map[string]any, 4)
	for k, v := range prev {
		child[k] = v
	}
	w.aliases = child
	for i := 0; i < len(items); i++ {
		if keyAlias != "" {
			child[keyAlias] = i
		}
		child[valAlias] = items[i]
		body(w, root)
	}
	w.aliases = prev
}

func buildIfAOT(b ast.Block, env string, mode EscapeMode, escapeCatalog bool, positive bool) (RenderFunc, int, error) {
	segs := splitElseIf(b.Body)
	cond, ok := ParseCond(b.Args)
	if !ok {
		return nil, 0, fmt.Errorf("canvas aot: bad if cond %q", b.Args)
	}
	thenFn, th, err := buildFunc(segs[0].body, env, mode, escapeCatalog)
	if err != nil {
		return nil, 0, err
	}
	var elseFn RenderFunc
	eh := 0
	if len(segs) > 1 {
		elseFn, eh, err = buildElseChainAOT(segs[1:], env, mode, escapeCatalog)
		if err != nil {
			return nil, 0, err
		}
	}
	return func(w *Writer, root map[string]any) {
		ok := EvalCond(w, root, cond)
		if !positive {
			ok = !ok
		}
		if ok {
			thenFn(w, root)
		} else if elseFn != nil {
			elseFn(w, root)
		}
	}, th + eh, nil
}

func buildElseChainAOT(segs []elseSeg, env string, mode EscapeMode, escapeCatalog bool) (RenderFunc, int, error) {
	if len(segs) == 0 {
		return nil, 0, nil
	}
	s := segs[0]
	if s.elseif == nil {
		return buildFunc(s.body, env, mode, escapeCatalog)
	}
	thenFn, th, err := buildFunc(s.body, env, mode, escapeCatalog)
	if err != nil {
		return nil, 0, err
	}
	elseFn, eh, err := buildElseChainAOT(segs[1:], env, mode, escapeCatalog)
	if err != nil {
		return nil, 0, err
	}
	cond := *s.elseif
	return func(w *Writer, root map[string]any) {
		if EvalCond(w, root, cond) {
			thenFn(w, root)
		} else if elseFn != nil {
			elseFn(w, root)
		}
	}, th + eh, nil
}

// --- lookups on Writer frame + root ---

func wLookup(w *Writer, root map[string]any, path []string) any {
	if len(path) == 0 {
		return nil
	}
	head := path[0]
	var cur any
	found := false
	if w.hasRV && head == w.rk {
		cur, found = w.rv, true
	} else if w.hasRV && w.rki != "" && head == w.rki {
		cur, found = w.ri, true
	} else if w.aliases != nil {
		if v, ok := w.aliases[head]; ok {
			cur, found = v, true
		}
	}
	if !found {
		if root == nil {
			return nil
		}
		v, ok := root[head]
		if !ok {
			return nil
		}
		cur = v
	}
	for _, part := range path[1:] {
		switch m := cur.(type) {
		case map[string]any:
			var ok bool
			cur, ok = m[part]
			if !ok {
				return nil
			}
		case map[string]string:
			v, ok := m[part]
			if !ok {
				return nil
			}
			cur = v
		default:
			return nil
		}
	}
	return cur
}

func wIsset(w *Writer, root map[string]any, path []string) bool {
	if len(path) == 0 {
		return false
	}
	if len(path) == 1 {
		head := path[0]
		if w.hasRV && head == w.rk {
			return true
		}
		if w.aliases != nil {
			if _, ok := w.aliases[head]; ok {
				return true
			}
		}
		if root != nil {
			_, ok := root[head]
			return ok
		}
		return false
	}
	head := wLookup(w, root, path[:len(path)-1])
	m, ok := head.(map[string]any)
	if !ok {
		return wLookup(w, root, path) != nil
	}
	_, ok = m[path[len(path)-1]]
	return ok
}

func transRoot(root map[string]any, key string) string {
	if m, ok := root["__trans"].(map[string]string); ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

func oldRoot(root map[string]any, key, def string) any {
	if old, ok := root["old"].(map[string]any); ok {
		if v, ok := old[key]; ok {
			return v
		}
	}
	if def != "" {
		return def
	}
	return ""
}

func hasErrorRoot(root map[string]any, key string) bool {
	errs, _ := root["errors"].(map[string]any)
	if errs == nil {
		return false
	}
	_, ok := errs[key]
	return ok
}

func canRoot(root map[string]any, ability string) bool {
	gates, _ := root["__can"].(map[string]bool)
	if gates != nil {
		return gates[ability]
	}
	return truthy(root["auth"]) && ability != ""
}
