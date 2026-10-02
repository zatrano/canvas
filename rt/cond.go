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
	// Truthy path (e.g. $active) when Op is empty and And/Or empty.
	Truthy []string
	// Compare: Left Op RightLit|RightPath
	Op        string
	Left      []string
	RightPath []string
	RightLit  any
	HasLit    bool
}

// ParseCond parses truthy paths, simple compares, and && / || chains (no parens).
func ParseCond(args string) (Cond, bool) {
	args = strings.TrimSpace(args)
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
	if strings.ContainsAny(args, "<>!=&|") {
		return Cond{}, false
	}
	return Cond{Truthy: p}, true
}

func splitLogic(s, op string) []string {
	if !strings.Contains(s, op) {
		return nil
	}
	var parts []string
	var b strings.Builder
	inQ := rune(0)
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
		if strings.HasPrefix(s[i:], op) {
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
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}
