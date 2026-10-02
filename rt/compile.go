package rt

import (
	"fmt"
	"strings"

	"github.com/zatrano/canvas/ast"
)

// Compile turns an AST-lowerable document into a Program.
func Compile(doc *ast.Document, env string) (p *Program, ok bool, err error) {
	if doc == nil {
		return &Program{Env: env}, true, nil
	}
	if !doc.CanASTLower() {
		return nil, false, nil
	}
	c := &compiler{env: env}
	if err := c.emitNodes(doc.Nodes); err != nil {
		return nil, false, err
	}
	return &Program{Ops: c.ops, Statics: c.statics, Env: env}, true, nil
}

type compiler struct {
	ops     []Op
	statics [][]byte
	env     string
}

func (c *compiler) emitNodes(nodes []ast.Node) error {
	for _, n := range nodes {
		if err := c.emit(n); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) emit(n ast.Node) error {
	switch x := n.(type) {
	case ast.Text:
		if x.Value == "" {
			return nil
		}
		idx := len(c.statics)
		c.statics = append(c.statics, []byte(x.Value))
		c.ops = append(c.ops, Op{Kind: OpStatic, A: idx})
	case ast.Comment:
		return nil
	case ast.Echo:
		c.ops = append(c.ops, Op{Kind: OpEchoEsc, Path: SplitPath(x.Expr)})
	case ast.RawEcho:
		c.ops = append(c.ops, Op{Kind: OpEchoRaw, Path: SplitPath(x.Expr)})
	case ast.Directive:
		return c.emitDirective(x)
	case ast.Block:
		return c.emitBlock(x)
	default:
		return fmt.Errorf("canvas rt: unsupported node")
	}
	return nil
}

func (c *compiler) emitDirective(d ast.Directive) error {
	switch d.Name {
	case "csrf":
		c.ops = append(c.ops, Op{Kind: OpCSRF})
	case "csrfMeta":
		c.ops = append(c.ops, Op{Kind: OpCSRFMeta})
	case "method":
		c.ops = append(c.ops, Op{Kind: OpMethod, S: strings.Trim(d.Args, `'" `)})
	case "json":
		c.ops = append(c.ops, Op{Kind: OpJSON, Path: SplitPath(d.Args)})
	case "class":
		c.ops = append(c.ops, Op{Kind: OpClassAttr, Path: SplitPath(d.Args)})
	case "style":
		c.ops = append(c.ops, Op{Kind: OpStyleAttr, Path: SplitPath(d.Args)})
	case "checked", "selected", "disabled", "readonly", "required":
		c.ops = append(c.ops, Op{Kind: OpAttrBool, Path: SplitPath(d.Args), S: d.Name})
	case "lang":
		c.ops = append(c.ops, Op{Kind: OpLang, S: strings.Trim(d.Args, `'" `)})
	case "old":
		parts := splitArgs(d.Args)
		key := strings.Trim(parts[0], `'" `)
		def := ""
		if len(parts) > 1 {
			def = strings.Trim(parts[1], `'" `)
		}
		c.ops = append(c.ops, Op{Kind: OpOld, S: key, Key: def})
	case "choice":
		parts := splitArgs(d.Args)
		c.ops = append(c.ops, Op{Kind: OpChoice, S: strings.Trim(parts[0], `'" `), Path: SplitPath(parts[1])})
	case "elseif", "else", "empty":
		return fmt.Errorf("canvas rt: unexpected @%s", d.Name)
	default:
		return fmt.Errorf("canvas rt: unsupported @%s", d.Name)
	}
	return nil
}

func (c *compiler) emitBlock(b ast.Block) error {
	switch b.Name {
	case "foreach":
		return c.emitForeach(b, false)
	case "forelse":
		return c.emitForeach(b, true)
	case "if":
		return c.emitIf(b, OpIfTruthy)
	case "unless":
		return c.emitIf(b, OpIfNot)
	case "isset":
		return c.emitIf(b, OpIfIsset)
	case "empty":
		return c.emitIf(b, OpIfEmpty)
	case "auth":
		return c.emitCond(OpIfTruthy, []string{"auth"}, "", b.Body)
	case "guest":
		return c.emitCond(OpIfTruthy, []string{"guest"}, "", b.Body)
	case "error":
		return c.emitCond(OpIfHasError, nil, strings.Trim(b.Args, `'" `), b.Body)
	case "can":
		return c.emitCond(OpIfCan, nil, strings.Trim(b.Args, `'" `), b.Body)
	case "cannot":
		return c.emitCond(OpIfCannot, nil, strings.Trim(b.Args, `'" `), b.Body)
	case "env":
		return c.emitCond(OpIfEnv, nil, strings.Trim(b.Args, `'" `), b.Body)
	case "production":
		return c.emitCond(OpIfProduction, nil, "", b.Body)
	default:
		return fmt.Errorf("canvas rt: unsupported block @%s", b.Name)
	}
}

func (c *compiler) emitForeach(b ast.Block, forelse bool) error {
	coll, key, alias, ok := parseForeach(b.Args)
	if !ok {
		return fmt.Errorf("canvas rt: bad @foreach args")
	}
	body := b.Body
	var emptyBody []ast.Node
	if forelse {
		var okSplit bool
		body, emptyBody, okSplit = splitEmpty(body)
		if !okSplit {
			return fmt.Errorf("canvas rt: @forelse missing @empty")
		}
		// if truthy(coll) { range } else { empty }
		return c.emitCondWithBranches(
			OpIfTruthy, SplitPath("$"+coll), "",
			func() error {
				return c.emitRange(coll, key, alias, body)
			},
			func() error {
				return c.emitNodes(emptyBody)
			},
		)
	}
	return c.emitRange(coll, key, alias, body)
}

func (c *compiler) emitRange(coll, key, alias string, body []ast.Node) error {
	rangePC := len(c.ops)
	c.ops = append(c.ops, Op{Kind: OpRange, Path: SplitPath("$" + coll), Key: key, Val: alias})
	if err := c.emitNodes(body); err != nil {
		return err
	}
	c.ops = append(c.ops, Op{Kind: OpRangeEnd})
	c.ops[rangePC].A = len(c.ops) // exclusive end (past RangeEnd)
	return nil
}

func (c *compiler) emitIf(b ast.Block, kind OpKind) error {
	cond, ok := ParseCond(b.Args)
	if !ok {
		return fmt.Errorf("canvas rt: bad if cond %q", b.Args)
	}
	if cond.Op != "" {
		return fmt.Errorf("canvas rt: compare @if requires AOT path")
	}
	return c.emitCond(kind, cond.Truthy, "", b.Body)
}

func (c *compiler) emitCond(kind OpKind, path []string, name string, body []ast.Node) error {
	segs := splitElseIf(body)
	thenEmit := func() error { return c.emitNodes(segs[0].body) }
	if len(segs) == 1 {
		return c.emitCondWithBranches(kind, path, name, thenEmit, nil)
	}
	elseEmit := func() error {
		return c.emitElseChain(segs[1:])
	}
	return c.emitCondWithBranches(kind, path, name, thenEmit, elseEmit)
}

// emitCondWithBranches lays out: If … thenBody [Else elseBody] End
// Op.A = index of Else marker (or End if no else) = exclusive end of then
// Op.B = index past End = exclusive end of else (0 if no else)
func (c *compiler) emitCondWithBranches(kind OpKind, path []string, name string, thenFn, elseFn func() error) error {
	ifOp := len(c.ops)
	c.ops = append(c.ops, Op{Kind: kind, Path: path, S: name})
	if err := thenFn(); err != nil {
		return err
	}
	thenEnd := len(c.ops) // exclusive: first index after then body

	if elseFn == nil {
		c.ops = append(c.ops, Op{Kind: OpEnd})
		c.ops[ifOp].A = thenEnd
		c.ops[ifOp].B = 0
		return nil
	}

	c.ops = append(c.ops, Op{Kind: OpElse}) // marker at thenEnd
	if err := elseFn(); err != nil {
		return err
	}
	endPC := len(c.ops)
	c.ops = append(c.ops, Op{Kind: OpEnd})
	c.ops[ifOp].A = thenEnd
	c.ops[ifOp].B = endPC // exclusive end of else body (OpEnd index)
	return nil
}

func (c *compiler) emitElseChain(segs []elseSeg) error {
	if len(segs) == 0 {
		return nil
	}
	s := segs[0]
	if s.elseif == nil {
		return c.emitNodes(s.body)
	}
	if s.elseif.Op != "" {
		return fmt.Errorf("canvas rt: compare @elseif requires AOT path")
	}
	return c.emitCondWithBranches(
		OpIfTruthy, s.elseif.Truthy, "",
		func() error { return c.emitNodes(s.body) },
		func() error { return c.emitElseChain(segs[1:]) },
	)
}

type elseSeg struct {
	elseif *Cond
	body   []ast.Node
}

func splitElseIf(nodes []ast.Node) []elseSeg {
	var segs []elseSeg
	cur := elseSeg{}
	for _, n := range nodes {
		d, ok := n.(ast.Directive)
		if ok && d.Name == "else" {
			segs = append(segs, cur)
			cur = elseSeg{}
			continue
		}
		if ok && d.Name == "elseif" {
			segs = append(segs, cur)
			c, cok := ParseCond(d.Args)
			if !cok {
				// Fallback: treat as truthy SplitPath for resilience.
				c = Cond{Truthy: SplitPath(d.Args)}
			}
			cur = elseSeg{elseif: &c}
			continue
		}
		cur.body = append(cur.body, n)
	}
	segs = append(segs, cur)
	return segs
}

func splitEmpty(nodes []ast.Node) (main, empty []ast.Node, ok bool) {
	for i, n := range nodes {
		d, isDir := n.(ast.Directive)
		if isDir && d.Name == "empty" && strings.TrimSpace(d.Args) == "" {
			return nodes[:i], nodes[i+1:], true
		}
	}
	return nil, nil, false
}

func parseForeach(args string) (coll, key, alias string, ok bool) {
	args = strings.TrimSpace(args)
	const asSep = " as "
	i := strings.Index(strings.ToLower(args), asSep)
	if i < 0 {
		return "", "", "", false
	}
	left := strings.TrimSpace(args[:i])
	right := strings.TrimSpace(args[i+len(asSep):])
	coll = strings.TrimPrefix(left, "$")
	if arrow := strings.Index(right, "=>"); arrow >= 0 {
		k := strings.TrimSpace(right[:arrow])
		v := strings.TrimSpace(right[arrow+2:])
		return coll, strings.TrimPrefix(k, "$"), strings.TrimPrefix(v, "$"), true
	}
	return coll, "", strings.TrimPrefix(right, "$"), true
}

func splitArgs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}
