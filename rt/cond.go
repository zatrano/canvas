package rt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zatrano/canvas/ast"
)

// Cond is a lowered @if / @elseif / @unless predicate.
type Cond struct {
	// And / Or are short-circuit chains (|| splits first, then &&).
	And []Cond
	Or  []Cond
	// Not negates the single child in And[0] when set with empty Truthy/Op.
	Not *Cond
	// Empty / Isset are function forms empty($path) / isset($path).
	Empty []string
	Isset []string
	// Truthy path (e.g. $active) when Op is empty and And/Or empty.
	Truthy []string
	// Compare: Left Op RightLit|RightPath
	Op        string
	Left      []string
	RightPath []string
	RightLit  any
	HasLit    bool
}

// ParseCond parses truthy paths, simple compares, ! / empty / isset, parens, and && / ||.
func ParseCond(args string) (Cond, bool) {
	args = unwrapParens(strings.TrimSpace(args))
	if args == "" {
		return Cond{}, false
	}
	if strings.HasPrefix(args, "!") {
		inner, ok := ParseCond(strings.TrimSpace(args[1:]))
		if !ok {
			return Cond{}, false
		}
		return Cond{Not: &inner}, true
	}
	if parts := splitLogic(args, "||"); len(parts) > 1 {
		or := make([]Cond, 0, len(parts))
		for _, p := range parts {
			c, ok := ParseCond(p)
			if !ok {
				return Cond{}, false
			}
			or = append(or, c)
		}
		return Cond{Or: or}, true
	}
	if parts := splitLogic(args, "&&"); len(parts) > 1 {
		and := make([]Cond, 0, len(parts))
		for _, p := range parts {
			c, ok := ParseCond(p)
			if !ok {
				return Cond{}, false
			}
			and = append(and, c)
		}
		return Cond{And: and}, true
	}
	if name, inner, ok := parseCall(args); ok {
		switch name {
		case "empty":
			p := SplitPath(inner)
			if len(p) == 0 {
				return Cond{}, false
			}
			return Cond{Empty: p}, true
		case "isset":
			p := SplitPath(inner)
			if len(p) == 0 {
				return Cond{}, false
			}
			return Cond{Isset: p}, true
		default:
			// count() and others: not yet AOT — fail so caller can miss fast path.
			return Cond{}, false
		}
	}
	if left, op, right, ok := ast.ParseSimpleCompare(args); ok {
		c := Cond{Op: op, Left: SplitPath(left)}
		if len(c.Left) == 0 {
			return Cond{}, false
		}
		right = strings.TrimSpace(right)
		if strings.HasPrefix(right, "$") {
			c.RightPath = SplitPath(right)
			if len(c.RightPath) == 0 {
				return Cond{}, false
			}
			return c, true
		}
		if n, err := strconv.ParseInt(right, 10, 64); err == nil {
			c.HasLit = true
			c.RightLit = n
			return c, true
		}
		if f, err := strconv.ParseFloat(right, 64); err == nil {
			c.HasLit = true
			c.RightLit = f
			return c, true
		}
		if q, ok := ast.Unquote(right); ok {
			c.HasLit = true
			c.RightLit = q
			return c, true
		}
		return Cond{}, false
	}
	p := SplitPath(args)
	if len(p) == 0 {
		return Cond{}, false
	}
	// Reject rich if_expr forms so AOT misses and html/template + CondExprLower runs.
	if strings.ContainsAny(args, "<>!=&|()[],?'\"") {
		return Cond{}, false
	}
	return Cond{Truthy: p}, true
}

func parseCall(args string) (name, inner string, ok bool) {
	args = strings.TrimSpace(args)
	i := 0
	for i < len(args) {
		r := args[i]
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (r >= '0' && r <= '9' && i > 0) {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return "", "", false
	}
	name = args[:i]
	rest := strings.TrimSpace(args[i:])
	if !strings.HasPrefix(rest, "(") || !strings.HasSuffix(rest, ")") {
		return "", "", false
	}
	inner = strings.TrimSpace(rest[1 : len(rest)-1])
	if inner == "" || strings.Contains(inner, ",") {
		return "", "", false
	}
	return name, inner, true
}

func unwrapParens(s string) string {
	for {
		s = strings.TrimSpace(s)
		if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
			return s
		}
		inner := s[1 : len(s)-1]
		depth := 0
		inQ := rune(0)
		ok := true
		for i := 0; i < len(s); i++ {
			r := rune(s[i])
			if inQ != 0 {
				if r == inQ {
					inQ = 0
				}
				continue
			}
			if r == '\'' || r == '"' {
				inQ = r
				continue
			}
			switch r {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i != len(s)-1 {
					ok = false
				}
			}
		}
		if !ok || depth != 0 {
			return s
		}
		s = inner
	}
}

func splitLogic(s, op string) []string {
	if !strings.Contains(s, op) {
		return nil
	}
	var parts []string
	var b strings.Builder
	inQ := rune(0)
	depth := 0
	for i := 0; i < len(s); {
		r := rune(s[i])
		if inQ != 0 {
			b.WriteByte(s[i])
			if r == inQ {
				inQ = 0
			}
			i++
			continue
		}
		if r == '\'' || r == '"' {
			inQ = r
			b.WriteByte(s[i])
			i++
			continue
		}
		if r == '(' {
			depth++
			b.WriteByte(s[i])
			i++
			continue
		}
		if r == ')' {
			depth--
			b.WriteByte(s[i])
			i++
			continue
		}
		if depth == 0 && strings.HasPrefix(s[i:], op) {
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()
			i += len(op)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	parts = append(parts, strings.TrimSpace(b.String()))
	if len(parts) < 2 {
		return nil
	}
	for _, p := range parts {
		if p == "" {
			return nil
		}
	}
	return parts
}

// EvalCond evaluates c against the current Writer frame and root.
func EvalCond(w *Writer, root map[string]any, c Cond) bool {
	if c.Not != nil {
		return !EvalCond(w, root, *c.Not)
	}
	if len(c.Or) > 0 {
		for _, x := range c.Or {
			if EvalCond(w, root, x) {
				return true
			}
		}
		return false
	}
	if len(c.And) > 0 {
		for _, x := range c.And {
			if !EvalCond(w, root, x) {
				return false
			}
		}
		return true
	}
	if len(c.Empty) > 0 {
		return isEmpty(wLookup(w, root, c.Empty))
	}
	if len(c.Isset) > 0 {
		return wIsset(w, root, c.Isset)
	}
	if c.Op == "" {
		return truthy(wLookup(w, root, c.Truthy))
	}
	lv := wLookup(w, root, c.Left)
	var rv any
	if c.HasLit {
		rv = c.RightLit
	} else {
		rv = wLookup(w, root, c.RightPath)
	}
	switch c.Op {
	case ">":
		return cmpGt(lv, rv)
	case ">=":
		return cmpGe(lv, rv)
	case "<":
		return cmpLt(lv, rv)
	case "<=":
		return cmpLe(lv, rv)
	case "==":
		return fmt.Sprint(lv) == fmt.Sprint(rv)
	case "!=":
		return fmt.Sprint(lv) != fmt.Sprint(rv)
	default:
		return false
	}
}

func cmpGt(a, b any) bool {
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af > bf
		}
	}
	return fmt.Sprint(a) > fmt.Sprint(b)
}

func cmpGe(a, b any) bool {
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af >= bf
		}
	}
	return fmt.Sprint(a) >= fmt.Sprint(b)
}

func cmpLt(a, b any) bool {
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af < bf
		}
	}
	return fmt.Sprint(a) < fmt.Sprint(b)
}

func cmpLe(a, b any) bool {
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af <= bf
		}
	}
	return fmt.Sprint(a) <= fmt.Sprint(b)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
