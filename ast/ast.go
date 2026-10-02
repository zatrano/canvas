// Package ast is the Canvas abstract syntax tree (lex → parse → lower).
//
// The Engine still uses the regex pipeline for block/complex directives
// (foreach, if, layout, components). AST lowers a growing leaf subset when
// CanASTLower is true. Full cutover replaces compileView regex passes with Lower().
package ast

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/zatrano/canvas/lex"
)

// Node is one Canvas AST node.
type Node interface {
	node()
}

// Text is literal HTML/text.
type Text struct{ Value string }

func (Text) node() {}

// Echo is {{ expr }} (escaped).
type Echo struct{ Expr string }

func (Echo) node() {}

// RawEcho is {!! expr !!}.
type RawEcho struct{ Expr string }

func (RawEcho) node() {}

// Comment is {{-- … --}} (stripped on lower).
type Comment struct{ Body string }

func (Comment) node() {}

// Directive is @name(args).
type Directive struct {
	Name string
	Args string
}

func (Directive) node() {}

// Document is a sequence of top-level nodes.
type Document struct {
	Nodes []Node
}

// Parse builds a Document from lexer tokens.
func Parse(tokens []lex.Token) (*Document, error) {
	doc := &Document{}
	for _, tok := range tokens {
		switch tok.Kind {
		case lex.KindEOF:
			continue
		case lex.KindText:
			if tok.Lit != "" {
				doc.Nodes = append(doc.Nodes, Text{Value: tok.Lit})
			}
		case lex.KindEcho:
			doc.Nodes = append(doc.Nodes, Echo{Expr: tok.Lit})
		case lex.KindRawEcho:
			doc.Nodes = append(doc.Nodes, RawEcho{Expr: tok.Lit})
		case lex.KindComment:
			doc.Nodes = append(doc.Nodes, Comment{Body: tok.Lit})
		case lex.KindDirective:
			doc.Nodes = append(doc.Nodes, Directive{Name: tok.Name, Args: tok.Args})
		default:
			return nil, fmt.Errorf("canvas ast: unknown token %s", tok.Kind)
		}
	}
	return doc, nil
}

// ParseSource lexes, parses, then nests paired blocks (default MaxNestingDepth).
func ParseSource(src string) (*Document, error) {
	return ParseSourceDepth(src, DefaultMaxNestingDepth)
}

// ParseSourceDepth is ParseSource with an explicit nesting limit.
func ParseSourceDepth(src string, maxDepth int) (*Document, error) {
	toks, err := lex.Lex(src)
	if err != nil {
		return nil, err
	}
	doc, err := Parse(toks)
	if err != nil {
		return nil, err
	}
	nested, err := NestDepth(doc.Nodes, maxDepth)
	if err != nil {
		return nil, err
	}
	doc.Nodes = nested
	return doc, nil
}

// CanASTLower reports whether Lower can compile the document without the regex pipeline.
func (d *Document) CanASTLower() bool {
	if d == nil {
		return true
	}
	return canLowerNodes(d.Nodes, nil)
}

func canLowerNodes(nodes []Node, aliases map[string]bool) bool {
	for _, n := range nodes {
		switch x := n.(type) {
		case Text, Comment:
			continue
		case Echo:
			if !echoOK(x.Expr, aliases) {
				return false
			}
		case RawEcho:
			if !echoOK(x.Expr, aliases) {
				return false
			}
		case Directive:
			if !leafDirectiveOK(x) {
				return false
			}
		case Block:
			if !blockOK(x, aliases) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func echoOK(expr string, aliases map[string]bool) bool {
	if isAttributesExpr(expr) {
		return false
	}
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "$") {
		return false
	}
	path := strings.TrimPrefix(expr, "$")
	head, _, _ := strings.Cut(path, ".")
	if aliases[head] {
		return identOK(path)
	}
	return simpleDollarPath(expr)
}

func identOK(path string) bool {
	if path == "" {
		return false
	}
	for _, r := range path {
		if !(r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func blockOK(b Block, aliases map[string]bool) bool {
	switch b.Name {
	case "foreach":
		coll, key, alias, ok := parseForeachArgs(b.Args)
		if !ok || !simpleDollarPath("$"+coll) {
			return false
		}
		child := copyAliases(aliases)
		child[alias] = true
		if key != "" {
			child[key] = true
		}
		return canLowerNodes(b.Body, child)
	case "forelse":
		coll, key, alias, ok := parseForeachArgs(b.Args)
		if !ok || !simpleDollarPath("$"+coll) {
			return false
		}
		main, emptyBody, ok := splitForelseBody(b.Body)
		if !ok {
			return false
		}
		child := copyAliases(aliases)
		child[alias] = true
		if key != "" {
			child[key] = true
		}
		return canLowerNodes(main, child) && canLowerNodes(emptyBody, aliases)
	case "if", "unless":
		if !condOK(strings.TrimSpace(b.Args)) {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	case "isset", "empty":
		if !simpleDollarPath(strings.TrimSpace(b.Args)) {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	case "auth", "guest":
		if strings.TrimSpace(b.Args) != "" {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	case "error":
		_, ok := unquote(strings.TrimSpace(b.Args))
		if !ok {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	case "can", "cannot":
		_, ok := unquote(strings.TrimSpace(b.Args))
		if !ok {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	case "env":
		_, ok := unquote(strings.TrimSpace(b.Args))
		if !ok {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	case "production":
		if strings.TrimSpace(b.Args) != "" {
			return false
		}
		return canLowerNodes(b.Body, aliases)
	default:
		return false
	}
}

// splitForelseBody splits on bare @empty marker Directive.
func splitForelseBody(nodes []Node) (main, empty []Node, ok bool) {
	for i, n := range nodes {
		d, isDir := n.(Directive)
		if !isDir || d.Name != "empty" || strings.TrimSpace(d.Args) != "" {
			continue
		}
		return nodes[:i], nodes[i+1:], true
	}
	return nil, nil, false
}

func copyAliases(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src)+2)
	for k, v := range src {
		if v {
			dst[k] = true
		}
	}
	return dst
}

// parseForeachArgs parses `$items as $item` or `$items as $k => $v`.
func parseForeachArgs(args string) (coll, key, alias string, ok bool) {
	args = strings.TrimSpace(args)
	// $path as $alias  |  $path as $key => $alias
	const asSep = " as "
	i := strings.Index(strings.ToLower(args), asSep)
	if i < 0 {
		return "", "", "", false
	}
	left := strings.TrimSpace(args[:i])
	right := strings.TrimSpace(args[i+len(asSep):])
	if !strings.HasPrefix(left, "$") {
		return "", "", "", false
	}
	coll = strings.TrimPrefix(left, "$")
	if !identOK(coll) {
		return "", "", "", false
	}
	if arrow := strings.Index(right, "=>"); arrow >= 0 {
		k := strings.TrimSpace(right[:arrow])
		v := strings.TrimSpace(right[arrow+2:])
		if !strings.HasPrefix(k, "$") || !strings.HasPrefix(v, "$") {
			return "", "", "", false
		}
		key = strings.TrimPrefix(k, "$")
		alias = strings.TrimPrefix(v, "$")
		if !identOK(key) || strings.Contains(key, ".") || !identOK(alias) || strings.Contains(alias, ".") {
			return "", "", "", false
		}
		return coll, key, alias, true
	}
	if !strings.HasPrefix(right, "$") {
		return "", "", "", false
	}
	alias = strings.TrimPrefix(right, "$")
	if !identOK(alias) || strings.Contains(alias, ".") {
		return "", "", "", false
	}
	return coll, "", alias, true
}

// CSRFOnly is kept for tests; prefer CanASTLower.
func (d *Document) CSRFOnly() bool {
	if d == nil {
		return true
	}
	for _, n := range d.Nodes {
		switch x := n.(type) {
		case Text, Comment:
			continue
		case Directive:
			switch x.Name {
			case "csrf", "csrfMeta", "method":
				continue
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

func leafDirectiveOK(x Directive) bool {
	switch x.Name {
	case "csrf", "csrfMeta", "else":
		return strings.TrimSpace(x.Args) == ""
	case "method":
		_, ok := unquote(strings.TrimSpace(x.Args))
		return ok
	case "json", "js", "class", "style",
		"checked", "selected", "disabled", "readonly", "required":
		return simpleDollarPath(strings.TrimSpace(x.Args))
	case "elseif":
		return condOK(strings.TrimSpace(x.Args))
	case "lang":
		args := strings.TrimSpace(x.Args)
		if strings.Contains(args, ",") {
			return false
		}
		_, ok := unquote(args)
		return ok
	case "old":
		return oldArgsOK(strings.TrimSpace(x.Args))
	case "choice":
		return choiceArgsOK(strings.TrimSpace(x.Args))
	default:
		return false
	}
}

func oldArgsOK(args string) bool {
	parts := splitTopComma(args)
	if len(parts) == 0 || len(parts) > 2 {
		return false
	}
	if _, ok := unquote(parts[0]); !ok {
		return false
	}
	if len(parts) == 2 {
		if _, ok := unquote(parts[1]); !ok {
			return false
		}
	}
	return true
}

func choiceArgsOK(args string) bool {
	parts := splitTopComma(args)
	if len(parts) != 2 {
		return false
	}
	if _, ok := unquote(parts[0]); !ok {
		return false
	}
	second := strings.TrimSpace(parts[1])
	if simpleDollarPath(second) {
		return true
	}
	_, err := strconv.Atoi(second)
	return err == nil
}

// ParseSimpleCompare parses `$path OP lit|$path` where OP is one of
// >= <= == != > <. Used by AST CanASTLower/Lower and rt AOT.
func ParseSimpleCompare(expr string) (left, op, right string, ok bool) {
	expr = strings.TrimSpace(expr)
	for _, cand := range []string{">=", "<=", "==", "!=", ">", "<"} {
		i := strings.Index(expr, cand)
		if i < 0 {
			continue
		}
		left = strings.TrimSpace(expr[:i])
		right = strings.TrimSpace(expr[i+len(cand):])
		if !simpleDollarPath(left) {
			continue
		}
		if simpleDollarPath(right) {
			return left, cand, right, true
		}
		if _, err := strconv.ParseInt(right, 10, 64); err == nil {
			return left, cand, right, true
		}
		if _, err := strconv.ParseFloat(right, 64); err == nil {
			return left, cand, right, true
		}
		if _, uok := unquote(right); uok {
			return left, cand, right, true
		}
	}
	return "", "", "", false
}

func condOK(args string) bool {
	args = strings.TrimSpace(args)
	if simpleDollarPath(args) {
		return true
	}
	_, _, _, ok := ParseSimpleCompare(args)
	return ok
}

func simpleDollarPath(expr string) bool {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "$") {
		return false
	}
	path := strings.TrimPrefix(expr, "$")
	if path == "" {
		return false
	}
	for _, r := range path {
		if !(r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func isAttributesExpr(expr string) bool {
	expr = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(expr), "$"))
	return expr == "attributes" || strings.HasPrefix(expr, "attributes.")
}

// Unquote strips a single- or double-quoted string literal.
func Unquote(s string) (string, bool) {
	return unquote(s)
}

func unquote(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return "", false
	}
	q := s[0]
	if q != '\'' && q != '"' {
		return "", false
	}
	if s[len(s)-1] != q {
		return "", false
	}
	inner := s[1 : len(s)-1]
	qr := rune(q)
	for _, r := range inner {
		if r == qr || r == '\n' || r == '\r' {
			return "", false
		}
	}
	return inner, true
}

func splitTopComma(s string) []string {
	var parts []string
	var b strings.Builder
	inQ := rune(0)
	for _, r := range s {
		switch {
		case inQ != 0:
			b.WriteRune(r)
			if r == inQ {
				inQ = 0
			}
		case r == '\'' || r == '"':
			inQ = r
			b.WriteRune(r)
		case r == ',':
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			if unicode.IsSpace(r) && b.Len() == 0 {
				continue
			}
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 || len(parts) > 0 {
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return parts
}

// LeafOnly reports whether the document can be AST-lowered (alias of CanASTLower).
func (d *Document) LeafOnly() bool {
	return d.CanASTLower()
}

// Lower turns a CanASTLower document into Go html/template source.
// Returns ok=false when the document needs the legacy regex pipeline.
func Lower(d *Document) (out string, ok bool, err error) {
	if d == nil {
		return "", true, nil
	}
	if !d.CanASTLower() {
		return "", false, nil
	}
	var b strings.Builder
	if !lowerNodes(&b, d.Nodes, nil) {
		return "", false, nil
	}
	return b.String(), true, nil
}

func lowerNodes(b *strings.Builder, nodes []Node, aliases map[string]bool) bool {
	for _, n := range nodes {
		switch x := n.(type) {
		case Text:
			b.WriteString(x.Value)
		case Comment:
			// stripped
		case Echo:
			b.WriteString("{{ ")
			b.WriteString(lowerExprIn(x.Expr, aliases))
			b.WriteString(" }}")
		case RawEcho:
			b.WriteString("{{ safeStr (")
			b.WriteString(lowerExprIn(x.Expr, aliases))
			b.WriteString(") }}")
		case Directive:
			if !lowerDirective(b, x, aliases) {
				return false
			}
		case Block:
			if !lowerBlock(b, x, aliases) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func lowerBlock(b *strings.Builder, blk Block, aliases map[string]bool) bool {
	switch blk.Name {
	case "foreach":
		coll, key, alias, ok := parseForeachArgs(blk.Args)
		if !ok {
			return false
		}
		child := copyAliases(aliases)
		child[alias] = true
		if key != "" {
			child[key] = true
		}
		collExpr := foreachCollExpr(coll, aliases)
		if key != "" {
			b.WriteString("{{ range $")
			b.WriteString(key)
			b.WriteString(", $")
			b.WriteString(alias)
			b.WriteString(" := ")
			b.WriteString(collExpr)
			b.WriteString(" }}")
		} else {
			b.WriteString("{{ range $__zfi, $")
			b.WriteString(alias)
			b.WriteString(" := ")
			b.WriteString(collExpr)
			b.WriteString(" }}")
		}
		if !lowerNodes(b, blk.Body, child) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "forelse":
		coll, key, alias, ok := parseForeachArgs(blk.Args)
		if !ok {
			return false
		}
		main, emptyBody, ok := splitForelseBody(blk.Body)
		if !ok {
			return false
		}
		child := copyAliases(aliases)
		child[alias] = true
		if key != "" {
			child[key] = true
		}
		collExpr := foreachCollExpr(coll, aliases)
		b.WriteString("{{ if not (empty (")
		b.WriteString(collExpr)
		b.WriteString(")) }}")
		if key != "" {
			b.WriteString("{{ range $")
			b.WriteString(key)
			b.WriteString(", $")
			b.WriteString(alias)
			b.WriteString(" := ")
			b.WriteString(collExpr)
			b.WriteString(" }}")
		} else {
			b.WriteString("{{ range $__zfi, $")
			b.WriteString(alias)
			b.WriteString(" := ")
			b.WriteString(collExpr)
			b.WriteString(" }}")
		}
		if !lowerNodes(b, main, child) {
			return false
		}
		b.WriteString("{{ end }}{{ else }}")
		if !lowerNodes(b, emptyBody, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "if":
		b.WriteString("{{ if ")
		b.WriteString(lowerCond(strings.TrimSpace(blk.Args), aliases))
		b.WriteString(" }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "unless":
		b.WriteString("{{ if not (")
		b.WriteString(lowerCond(strings.TrimSpace(blk.Args), aliases))
		b.WriteString(") }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "isset":
		path := strings.TrimPrefix(strings.TrimSpace(blk.Args), "$")
		head, rest, hasDot := strings.Cut(path, ".")
		if aliases[head] {
			if !hasDot {
				b.WriteString("{{ if $")
				b.WriteString(head)
				b.WriteString(" }}")
			} else {
				b.WriteString("{{ if issetPath $")
				b.WriteString(head)
				b.WriteString(" `")
				b.WriteString(rest)
				b.WriteString("` }}")
			}
		} else if len(aliases) > 0 {
			b.WriteString("{{ if issetPath $ `")
			b.WriteString(path)
			b.WriteString("` }}")
		} else {
			b.WriteString("{{ if issetPath . `")
			b.WriteString(path)
			b.WriteString("` }}")
		}
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "empty":
		b.WriteString("{{ if empty (")
		b.WriteString(lowerExprIn(strings.TrimSpace(blk.Args), aliases))
		b.WriteString(") }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "auth":
		b.WriteString("{{ if dataGet . `auth` }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "guest":
		b.WriteString("{{ if dataGet . `guest` }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "error":
		key, ok := unquote(strings.TrimSpace(blk.Args))
		if !ok {
			return false
		}
		b.WriteString("{{ if hasError . `")
		b.WriteString(key)
		b.WriteString("` }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "can":
		ability, ok := unquote(strings.TrimSpace(blk.Args))
		if !ok {
			return false
		}
		b.WriteString("{{ if can . `")
		b.WriteString(ability)
		b.WriteString("` }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "cannot":
		ability, ok := unquote(strings.TrimSpace(blk.Args))
		if !ok {
			return false
		}
		b.WriteString("{{ if not (can . `")
		b.WriteString(ability)
		b.WriteString("`) }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "env":
		name, ok := unquote(strings.TrimSpace(blk.Args))
		if !ok {
			return false
		}
		b.WriteString("{{ if env `")
		b.WriteString(name)
		b.WriteString("` }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	case "production":
		b.WriteString("{{ if production }}")
		if !lowerNodes(b, blk.Body, aliases) {
			return false
		}
		b.WriteString("{{ end }}")
		return true
	default:
		return false
	}
}

func foreachCollExpr(path string, aliases map[string]bool) string {
	if i := strings.IndexByte(path, '.'); i > 0 {
		head := path[:i]
		rest := path[i+1:]
		if aliases[head] {
			return "dataGet $" + head + " `" + rest + "`"
		}
	}
	return "dataGet $ `" + path + "`"
}

func lowerDirective(b *strings.Builder, x Directive, aliases map[string]bool) bool {
	switch x.Name {
	case "csrf":
		// Always read from Execute root (`$`) so @csrf works inside @foreach.
		b.WriteString(`<input type="hidden" name="_token" value="{{ dataGet $ ` + "`_token`" + ` }}">`)
	case "csrfMeta":
		b.WriteString(`<meta name="csrf-token" content="{{ dataGet $ ` + "`_token`" + ` }}">`)
	case "method":
		method, ok := unquote(strings.TrimSpace(x.Args))
		if !ok || method == "" {
			return false
		}
		b.WriteString(`<input type="hidden" name="_method" value="`)
		b.WriteString(templateEscape(method))
		b.WriteString(`">`)
	case "json":
		b.WriteString("{{ json (")
		b.WriteString(lowerExprIn(strings.TrimSpace(x.Args), aliases))
		b.WriteString(") }}")
	case "class":
		b.WriteString("{{ classAttr (")
		b.WriteString(lowerExprIn(strings.TrimSpace(x.Args), aliases))
		b.WriteString(") }}")
	case "style":
		b.WriteString("{{ styleAttr (")
		b.WriteString(lowerExprIn(strings.TrimSpace(x.Args), aliases))
		b.WriteString(") }}")
	case "checked", "selected", "disabled", "readonly", "required":
		b.WriteString("{{ attrBool (")
		b.WriteString(lowerExprIn(strings.TrimSpace(x.Args), aliases))
		b.WriteString(") `")
		b.WriteString(x.Name)
		b.WriteString("` }}")
	case "lang":
		key, ok := unquote(strings.TrimSpace(x.Args))
		if !ok {
			return false
		}
		b.WriteString("{{ canvasTrans $ `")
		b.WriteString(key)
		b.WriteString("` }}")
	case "old":
		parts := splitTopComma(strings.TrimSpace(x.Args))
		key, ok := unquote(parts[0])
		if !ok {
			return false
		}
		if len(parts) == 1 {
			b.WriteString("{{ old . `")
			b.WriteString(key)
			b.WriteString("` }}")
		} else {
			def, ok := unquote(parts[1])
			if !ok {
				return false
			}
			b.WriteString("{{ old . `")
			b.WriteString(key)
			b.WriteString("` `")
			b.WriteString(def)
			b.WriteString("` }}")
		}
	case "choice":
		parts := splitTopComma(strings.TrimSpace(x.Args))
		key, ok := unquote(parts[0])
		if !ok {
			return false
		}
		second := strings.TrimSpace(parts[1])
		b.WriteString("{{ choice `")
		b.WriteString(key)
		b.WriteString("` ")
		if simpleDollarPath(second) {
			b.WriteString(lowerExprIn(second, aliases))
		} else {
			b.WriteString(second)
		}
		b.WriteString(" }}")
	case "elseif":
		b.WriteString("{{ else if ")
		b.WriteString(lowerCond(strings.TrimSpace(x.Args), aliases))
		b.WriteString(" }}")
	case "else":
		b.WriteString("{{ else }}")
	default:
		return false
	}
	return true
}

func lowerCond(args string, aliases map[string]bool) string {
	args = strings.TrimSpace(args)
	if left, op, right, ok := ParseSimpleCompare(args); ok {
		lf := lowerExprIn(left, aliases)
		rf := right
		if simpleDollarPath(right) {
			rf = lowerExprIn(right, aliases)
		} else if q, uok := unquote(right); uok {
			rf = "`" + strings.ReplaceAll(q, "`", "") + "`"
		}
		switch op {
		case "==":
			return fmt.Sprintf("(eq (printf `%%v` %s) (printf `%%v` %s))", lf, rf)
		case "!=":
			return fmt.Sprintf("(ne (printf `%%v` %s) (printf `%%v` %s))", lf, rf)
		case ">":
			return "(cmpGt " + lf + " " + rf + ")"
		case ">=":
			return "(cmpGe " + lf + " " + rf + ")"
		case "<":
			return "(cmpLt " + lf + " " + rf + ")"
		case "<=":
			return "(cmpLe " + lf + " " + rf + ")"
		}
	}
	return lowerExprIn(args, aliases)
}

func lowerExprIn(expr string, aliases map[string]bool) string {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "$") {
		return expr
	}
	path := strings.TrimPrefix(expr, "$")
	head, rest, hasDot := strings.Cut(path, ".")
	if aliases[head] {
		if !hasDot {
			return "$" + head
		}
		return "dataGet $" + head + " `" + rest + "`"
	}
	if len(aliases) > 0 {
		return "dataGet $ `" + path + "`"
	}
	return "dataGet . `" + path + "`"
}

func templateEscape(s string) string {
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
