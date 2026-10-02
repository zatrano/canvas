package canvas

import (
	"sort"
	"strings"
	"unicode"

	"github.com/zatrano/canvas/ast"
)

func init() {
	ast.CondExprLower = lowerCondViaIfExpr
}

// lowerCondViaIfExpr compiles Blade-like @if / {{ }} expressions (count, ternary,
// in_array, index, …) through the shared if_expr compiler, rewriting foreach
// aliases to __ZRV_* so output uses $alias / dataGet $alias.
func lowerCondViaIfExpr(expr string, aliases map[string]bool) (string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}
	rewritten := rewriteIfAliases(expr, aliases)
	out, err := compileIfInner(rewritten)
	if err != nil {
		return "", false
	}
	return out, true
}

func rewriteIfAliases(expr string, aliases map[string]bool) string {
	var b strings.Builder
	b.Grow(len(expr) + 16)
	names := make([]string, 0, len(aliases))
	for n, ok := range aliases {
		if ok && n != "" {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	for i := 0; i < len(expr); {
		if expr[i] != '$' {
			b.WriteByte(expr[i])
			i++
			continue
		}
		j := i + 1
		for j < len(expr) {
			r := rune(expr[j])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				j++
				continue
			}
			break
		}
		if j == i+1 {
			b.WriteByte('$')
			i++
			continue
		}
		head := expr[i+1 : j]
		matched := ""
		for _, n := range names {
			if head == n {
				matched = n
				break
			}
		}
		if matched != "" {
			b.WriteString("__ZRV_")
			b.WriteString(matched)
			b.WriteString("__")
			i = j
			continue
		}
		// Inside foreach, free $vars must read the Execute root (`$`), not `.`
		// (dot is the range element). Mirror regex __ZPARENT__ rewrite.
		if len(aliases) > 0 {
			b.WriteString("__ZPARENT__.")
			b.WriteString(head)
			i = j
			continue
		}
		b.WriteString(expr[i:j])
		i = j
	}
	return b.String()
}
