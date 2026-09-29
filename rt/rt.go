// Package rt is the Canvas high-performance render runtime.
//
// Hot path: AST → compiled Program (static segments + typed ops) → Execute.
// No html/template, no regex, minimal reflection (map[string]any fast path).
package rt

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Ctx is one render execution.
type Ctx struct {
	Root    map[string]any
	Aliases map[string]any // nested range bindings
	// Fast single-level range (zero map alloc):
	HasRange bool
	RangeKey string // value alias, e.g. "item"
	RangeVal any
	RangeIdx string // optional key alias
	RangeI   any
}

// Writer is a pooled byte buffer with optional range frame (zero Ctx alloc).
type Writer struct {
	b []byte
	// Hot range frame (top-level foreach); nested uses Aliases on demand.
	hasRV   bool
	rv      any
	rk      string // value alias name
	ri      any
	rki     string // key alias name
	aliases map[string]any
}

func (w *Writer) Reset() {
	w.b = w.b[:0]
	w.hasRV = false
	w.rv = nil
	w.ri = nil
	w.aliases = nil
}
func (w *Writer) Bytes() []byte        { return w.b }
func (w *Writer) String() string       { return string(w.b) }
func (w *Writer) Write(p []byte)       { w.b = append(w.b, p...) }
func (w *Writer) WriteString(s string) { w.b = append(w.b, s...) }
func (w *Writer) AppendByte(c byte)    { w.b = append(w.b, c) }

var writerPool = sync.Pool{New: func() any { return &Writer{b: make([]byte, 0, 1024)} }}

// AcquireWriter returns a pooled writer.
func AcquireWriter() *Writer {
	return writerPool.Get().(*Writer)
}

// ReleaseWriter returns a writer to the pool.
func ReleaseWriter(w *Writer) {
	if w == nil {
		return
	}
	w.Reset()
	if cap(w.b) > 1<<20 {
		return // drop oversized
	}
	writerPool.Put(w)
}

// Op is one compiled instruction.
type Op struct {
	Kind OpKind
	A    int    // jump / body end / static index
	B    int    // secondary jump
	S    string // path / ability / key
	Path []string
	Key  string // range key alias
	Val  string // range value alias
}

// OpKind enumerates opcodes.
type OpKind byte

const (
	OpStatic OpKind = iota
	OpEchoEsc
	OpEchoRaw
	OpJSON
	OpRange
	OpRangeEnd
	OpIfTruthy
	OpIfNot
	OpIfIsset
	OpIfEmpty
	OpIfHasError
	OpIfCan
	OpIfCannot
	OpIfEnv
	OpIfProduction
	OpElse
	OpEnd
	OpCSRF
	OpCSRFMeta
	OpMethod
	OpAttrBool
	OpClassAttr
	OpStyleAttr
	OpLang
	OpOld
	OpChoice
)

// Program is a compiled Canvas template.
type Program struct {
	Ops     []Op
	Statics [][]byte
	Env     string
}

// Execute runs the program into w.
func (p *Program) Execute(w *Writer, root map[string]any) error {
	ctx := Ctx{Root: root, Aliases: nil}
	_, err := p.exec(w, &ctx, 0, len(p.Ops))
	return err
}

// Render allocates from the pool and returns a string.
func (p *Program) Render(root map[string]any) (string, error) {
	w := AcquireWriter()
	defer ReleaseWriter(w)
	if cap(w.b) < p.hintSize() {
		w.b = make([]byte, 0, p.hintSize())
	}
	if err := p.Execute(w, root); err != nil {
		return "", err
	}
	// explicit copy so pooled buffer can be reused
	out := string(w.Bytes())
	return out, nil
}

func (p *Program) hintSize() int {
	n := 0
	for _, s := range p.Statics {
		n += len(s)
	}
	return n + 256
}

func (p *Program) exec(w *Writer, ctx *Ctx, pc, end int) (int, error) {
	for pc < end {
		op := p.Ops[pc]
		switch op.Kind {
		case OpStatic:
			w.Write(p.Statics[op.A])
			pc++
		case OpEchoEsc:
			writeEscaped(w, resolveFast(ctx, op.Path))
			pc++
		case OpEchoRaw:
			writeRaw(w, resolveFast(ctx, op.Path))
			pc++
		case OpJSON:
			writeJSON(w, resolveFast(ctx, op.Path))
			pc++
		case OpCSRF:
			tok, _ := resolve(ctx, []string{"_token"}).(string)
			w.WriteString(`<input type="hidden" name="_token" value="`)
			writeEscapedString(w, tok)
			w.WriteString(`">`)
			pc++
		case OpCSRFMeta:
			tok, _ := resolve(ctx, []string{"_token"}).(string)
			w.WriteString(`<meta name="csrf-token" content="`)
			writeEscapedString(w, tok)
			w.WriteString(`">`)
			pc++
		case OpMethod:
			w.WriteString(`<input type="hidden" name="_method" value="`)
			writeEscapedString(w, op.S)
			w.WriteString(`">`)
			pc++
		case OpLang:
			// locale from root; translation table optional via root["__trans"]
			w.WriteString(trans(ctx, op.S))
			pc++
		case OpOld:
			writeEscaped(w, oldValue(ctx, op.S, op.Key))
			pc++
		case OpChoice:
			writeEscaped(w, choiceValue(ctx, op.S, op.Path))
			pc++
		case OpAttrBool:
			v := resolve(ctx, op.Path)
			if truthy(v) {
				w.AppendByte(' ')
				w.WriteString(op.S)
			}
			pc++
		case OpClassAttr:
			writeClassAttr(w, resolve(ctx, op.Path))
			pc++
		case OpStyleAttr:
			writeStyleAttr(w, resolve(ctx, op.Path))
			pc++
		case OpRange:
			coll := resolve(ctx, op.Path)
			bodyEnd := op.A
			if err := p.execRange(w, ctx, pc+1, bodyEnd, op.Key, op.Val, coll); err != nil {
				return pc, err
			}
			pc = bodyEnd
		case OpIfTruthy, OpIfNot, OpIfIsset, OpIfEmpty, OpIfHasError, OpIfCan, OpIfCannot, OpIfEnv, OpIfProduction:
			thenEnd := op.A // exclusive end of then / index of Else or End
			elseEnd := op.B // exclusive end of else body (OpEnd index); 0 = no else
			ok := evalCond(p, ctx, op)
			if ok {
				if _, err := p.exec(w, ctx, pc+1, thenEnd); err != nil {
					return pc, err
				}
				if elseEnd > 0 {
					pc = elseEnd + 1 // skip OpEnd
				} else {
					pc = thenEnd + 1 // skip OpEnd
				}
			} else if elseEnd > 0 {
				// else body is after OpElse marker at thenEnd
				if _, err := p.exec(w, ctx, thenEnd+1, elseEnd); err != nil {
					return pc, err
				}
				pc = elseEnd + 1
			} else {
				pc = thenEnd + 1
			}
		case OpElse, OpEnd, OpRangeEnd:
			return pc, nil
		default:
			return pc, fmt.Errorf("canvas rt: unknown op %d", op.Kind)
		}
	}
	return pc, nil
}

func (p *Program) execRange(w *Writer, ctx *Ctx, bodyStart, bodyEnd int, keyAlias, valAlias string, coll any) error {
	if coll == nil {
		return nil
	}
	// Fast path: top-level range, no nested aliases → zero map allocation.
	if ctx.Aliases == nil && !ctx.HasRange {
		prevKey, prevVal, prevIdx, prevI := ctx.RangeKey, ctx.RangeVal, ctx.RangeIdx, ctx.RangeI
		ctx.HasRange = true
		ctx.RangeKey = valAlias
		ctx.RangeIdx = keyAlias
		defer func() {
			ctx.HasRange = false
			ctx.RangeKey, ctx.RangeVal = prevKey, prevVal
			ctx.RangeIdx, ctx.RangeI = prevIdx, prevI
		}()
		return p.execRangeFast(w, ctx, bodyStart, bodyEnd, keyAlias, coll)
	}

	prev := ctx.Aliases
	var child map[string]any
	if prev == nil {
		child = make(map[string]any, 2)
	} else {
		child = make(map[string]any, len(prev)+2)
		for k, v := range prev {
			child[k] = v
		}
	}
	if ctx.HasRange {
		child[ctx.RangeKey] = ctx.RangeVal
		if ctx.RangeIdx != "" {
			child[ctx.RangeIdx] = ctx.RangeI
		}
		ctx.HasRange = false
	}
	ctx.Aliases = child
	defer func() { ctx.Aliases = prev }()

	switch c := coll.(type) {
	case []map[string]any:
		for i := 0; i < len(c); i++ {
			if keyAlias != "" {
				child[keyAlias] = i
			}
			child[valAlias] = c[i]
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for i := 0; i < len(c); i++ {
			if keyAlias != "" {
				child[keyAlias] = i
			}
			child[valAlias] = c[i]
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	case []string:
		for i := 0; i < len(c); i++ {
			if keyAlias != "" {
				child[keyAlias] = i
			}
			child[valAlias] = c[i]
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for k, item := range c {
			if keyAlias != "" {
				child[keyAlias] = k
			}
			child[valAlias] = item
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	}

	rv := reflect.ValueOf(coll)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		n := rv.Len()
		for i := 0; i < n; i++ {
			if keyAlias != "" {
				child[keyAlias] = i
			}
			child[valAlias] = rv.Index(i).Interface()
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if keyAlias != "" {
				child[keyAlias] = iter.Key().Interface()
			}
			child[valAlias] = iter.Value().Interface()
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Program) execRangeFast(w *Writer, ctx *Ctx, bodyStart, bodyEnd int, keyAlias string, coll any) error {
	switch c := coll.(type) {
	case []map[string]any:
		for i := 0; i < len(c); i++ {
			if keyAlias != "" {
				ctx.RangeI = i
			}
			ctx.RangeVal = c[i]
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for i := 0; i < len(c); i++ {
			if keyAlias != "" {
				ctx.RangeI = i
			}
			ctx.RangeVal = c[i]
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	case []string:
		for i := 0; i < len(c); i++ {
			if keyAlias != "" {
				ctx.RangeI = i
			}
			ctx.RangeVal = c[i]
			if _, err := p.exec(w, ctx, bodyStart, bodyEnd); err != nil {
				return err
			}
		}
		return nil
	}
	ctx.HasRange = false
	return p.execRange(w, ctx, bodyStart, bodyEnd, keyAlias, ctx.RangeKey, coll)
}

func evalCond(p *Program, ctx *Ctx, op Op) bool {
	switch op.Kind {
	case OpIfTruthy:
		return truthy(resolveFast(ctx, op.Path))
	case OpIfNot:
		return !truthy(resolveFast(ctx, op.Path))
	case OpIfIsset:
		return isset(ctx, op.Path)
	case OpIfEmpty:
		return isEmpty(resolveFast(ctx, op.Path))
	case OpIfHasError:
		return hasError(ctx, op.S)
	case OpIfCan:
		return can(ctx, op.S)
	case OpIfCannot:
		return !can(ctx, op.S)
	case OpIfEnv:
		return p.Env == op.S
	case OpIfProduction:
		return p.Env == "production"
	default:
		return false
	}
}

func resolveFast(ctx *Ctx, path []string) any {
	switch len(path) {
	case 0:
		return nil
	case 1:
		if ctx.HasRange && path[0] == ctx.RangeKey {
			return ctx.RangeVal
		}
		if ctx.HasRange && ctx.RangeIdx != "" && path[0] == ctx.RangeIdx {
			return ctx.RangeI
		}
		if ctx.Aliases != nil {
			if v, ok := ctx.Aliases[path[0]]; ok {
				return v
			}
		}
		if ctx.Root != nil {
			return ctx.Root[path[0]]
		}
		return nil
	case 2:
		var head any
		ok := false
		if ctx.HasRange && path[0] == ctx.RangeKey {
			head, ok = ctx.RangeVal, true
		} else if ctx.Aliases != nil {
			head, ok = ctx.Aliases[path[0]]
		}
		if !ok && ctx.Root != nil {
			head, ok = ctx.Root[path[0]]
		}
		if !ok {
			return nil
		}
		switch m := head.(type) {
		case map[string]any:
			return m[path[1]]
		case map[string]string:
			return m[path[1]]
		default:
			return resolve(ctx, path)
		}
	default:
		return resolve(ctx, path)
	}
}

func resolve(ctx *Ctx, path []string) any {
	if len(path) == 0 {
		return nil
	}
	head := path[0]
	var cur any
	found := false
	if ctx.HasRange && head == ctx.RangeKey {
		cur, found = ctx.RangeVal, true
	} else if ctx.HasRange && ctx.RangeIdx != "" && head == ctx.RangeIdx {
		cur, found = ctx.RangeI, true
	} else if ctx.Aliases != nil {
		if v, ok := ctx.Aliases[head]; ok {
			cur, found = v, true
		}
	}
	if !found {
		if ctx.Root == nil {
			return nil
		}
		v, ok := ctx.Root[head]
		if !ok {
			return nil
		}
		cur = v
	}
	path = path[1:]
	for _, part := range path {
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
			rv := reflect.ValueOf(cur)
			for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
				if rv.IsNil() {
					return nil
				}
				rv = rv.Elem()
			}
			switch rv.Kind() {
			case reflect.Map:
				if rv.Type().Key().Kind() != reflect.String {
					return nil
				}
				val := rv.MapIndex(reflect.ValueOf(part))
				if !val.IsValid() {
					return nil
				}
				cur = val.Interface()
			case reflect.Struct:
				f := rv.FieldByName(part)
				if !f.IsValid() || !f.CanInterface() {
					return nil
				}
				cur = f.Interface()
			default:
				return nil
			}
		}
	}
	return cur
}

func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case []map[string]any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		return !rv.IsNil()
	case reflect.Slice, reflect.Map, reflect.Array, reflect.String:
		return rv.Len() > 0
	case reflect.Bool:
		return rv.Bool()
	case reflect.Int, reflect.Int64, reflect.Int32:
		return rv.Int() != 0
	}
	return true
}

func isEmpty(v any) bool {
	return !truthy(v)
}

func isset(ctx *Ctx, path []string) bool {
	if len(path) == 0 {
		return false
	}
	head := path[0]
	var cur any
	ok := false
	if ctx.HasRange && head == ctx.RangeKey {
		cur, ok = ctx.RangeVal, true
	} else if ctx.Aliases != nil {
		cur, ok = ctx.Aliases[head]
	}
	if !ok {
		if ctx.Root == nil {
			return false
		}
		cur, ok = ctx.Root[head]
		if !ok {
			return false
		}
	}
	for _, part := range path[1:] {
		switch m := cur.(type) {
		case map[string]any:
			cur, ok = m[part]
			if !ok {
				return false
			}
		case map[string]string:
			_, ok = m[part]
			return ok
		default:
			rv := reflect.ValueOf(cur)
			for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
				if rv.IsNil() {
					return false
				}
				rv = rv.Elem()
			}
			switch rv.Kind() {
			case reflect.Map:
				if rv.Type().Key().Kind() != reflect.String {
					return false
				}
				val := rv.MapIndex(reflect.ValueOf(part))
				if !val.IsValid() {
					return false
				}
				cur = val.Interface()
			case reflect.Struct:
				f := rv.FieldByName(part)
				if !f.IsValid() {
					return false
				}
				cur = f.Interface()
			default:
				return false
			}
		}
	}
	return true
}

func hasError(ctx *Ctx, key string) bool {
	errs, _ := ctx.Root["errors"].(map[string]any)
	if errs == nil {
		return false
	}
	_, ok := errs[key]
	return ok
}

func can(ctx *Ctx, ability string) bool {
	gates, _ := ctx.Root["__can"].(map[string]bool)
	if gates != nil {
		return gates[ability]
	}
	auth, _ := ctx.Root["auth"].(bool)
	return auth && ability != ""
}

func trans(ctx *Ctx, key string) string {
	locale, _ := ctx.Root["locale"].(map[string]any)
	if locale == nil {
		if m, ok := ctx.Root["__trans"].(map[string]string); ok {
			if v, ok := m[key]; ok {
				return v
			}
		}
		return key
	}
	if v, ok := locale[key]; ok {
		return fmt.Sprint(v)
	}
	return key
}

func oldValue(ctx *Ctx, key, def string) any {
	old, _ := ctx.Root["old"].(map[string]any)
	if old != nil {
		if v, ok := old[key]; ok {
			return v
		}
	}
	if def != "" {
		return def
	}
	return ""
}

func choiceValue(ctx *Ctx, key string, countPath []string) any {
	_ = resolve(ctx, countPath)
	return trans(ctx, key)
}

func writeRaw(w *Writer, v any) {
	if v == nil {
		return
	}
	switch x := v.(type) {
	case string:
		w.WriteString(x)
	case []byte:
		w.Write(x)
	default:
		w.WriteString(fmt.Sprint(x))
	}
}

func writeEscaped(w *Writer, v any) {
	if v == nil {
		return
	}
	switch x := v.(type) {
	case string:
		writeEscapedString(w, x)
	case int:
		w.WriteString(strconv.Itoa(x))
	case int64:
		w.WriteString(strconv.FormatInt(x, 10))
	case float64:
		w.WriteString(strconv.FormatFloat(x, 'f', -1, 64))
	case bool:
		if x {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case []byte:
		writeEscapedString(w, string(x))
	default:
		writeEscapedString(w, fmt.Sprint(x))
	}
}

// WriteEscaped HTML-escapes s into w. Used by canvas gen typed streams.
func WriteEscaped(w *Writer, s string) { writeEscapedString(w, s) }

// writeEscapedString HTML-escapes s into w (hot path).
func writeEscapedString(w *Writer, s string) {
	// Fast path: no escapable bytes → single append.
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&', '<', '>', '"', '\'':
			writeEscapedStringSlow(w, s)
			return
		}
	}
	w.WriteString(s)
}

func writeEscapedStringSlow(w *Writer, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			w.WriteString("&amp;")
		case '<':
			w.WriteString("&lt;")
		case '>':
			w.WriteString("&gt;")
		case '"':
			w.WriteString("&#34;")
		case '\'':
			w.WriteString("&#39;")
		default:
			w.AppendByte(s[i])
		}
	}
}

func writeJSON(w *Writer, v any) {
	// Minimal JSON for common types; fall back to fmt for others.
	switch x := v.(type) {
	case nil:
		w.WriteString("null")
	case string:
		w.AppendByte('"')
		writeJSONString(w, x)
		w.AppendByte('"')
	case int:
		w.WriteString(strconv.Itoa(x))
	case bool:
		if x {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	default:
		w.WriteString(strconv.Quote(fmt.Sprint(x)))
	}
}

func writeJSONString(w *Writer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			w.AppendByte('\\')
			w.AppendByte(c)
		case '\n':
			w.WriteString(`\n`)
		case '\r':
			w.WriteString(`\r`)
		case '\t':
			w.WriteString(`\t`)
		default:
			w.AppendByte(c)
		}
	}
}

func writeClassAttr(w *Writer, v any) {
	switch x := v.(type) {
	case string:
		w.WriteString(x)
	case []string:
		w.WriteString(strings.Join(x, " "))
	case map[string]any:
		first := true
		for k, val := range x {
			if !truthy(val) {
				continue
			}
			if !first {
				w.AppendByte(' ')
			}
			first = false
			w.WriteString(k)
		}
	}
}

func writeStyleAttr(w *Writer, v any) {
	switch x := v.(type) {
	case string:
		w.WriteString(x)
	case map[string]any:
		first := true
		for k, val := range x {
			if !truthy(val) {
				continue
			}
			if !first {
				w.AppendByte(';')
			}
			first = false
			w.WriteString(k)
			w.AppendByte(':')
			w.WriteString(fmt.Sprint(val))
		}
	}
}

// SplitPath splits "a.b.c" into ["a","b","c"].
func SplitPath(p string) []string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "$")
	if p == "" {
		return nil
	}
	return strings.Split(p, ".")
}
