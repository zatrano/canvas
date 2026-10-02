package canvas

import (
	"fmt"
	"strings"

	"github.com/zatrano/canvas/ast"
)

// extractSections prefers AST when the child view parses; regex remains fallback
// for templates that still rely on regex-only constructs during the V3 cutover.
func extractSections(content string) map[string]string {
	doc, err := ast.ParseSource(content)
	if err != nil {
		return extractSectionsRegex(content)
	}
	sections := extractSectionsFromNodes(doc.Nodes)
	if len(sections) == 0 {
		return extractSectionsRegex(content)
	}
	return sections
}

func extractSectionsFromNodes(nodes []ast.Node) map[string]string {
	out := map[string]string{}
	for _, n := range nodes {
		switch t := n.(type) {
		case ast.Block:
			name := strings.ToLower(t.Name)
			if name != "section" {
				continue
			}
			key, rest, ok := splitSectionArgs(t.Args)
			if !ok || key == "" {
				continue
			}
			if rest != "" && len(t.Body) == 0 {
				out[key] = sectionInlineValue(rest)
				continue
			}
			out[key] = strings.TrimSpace(nodesToSource(t.Body))
		case ast.Directive:
			if strings.ToLower(t.Name) != "section" {
				continue
			}
			key, rest, ok := splitSectionArgs(t.Args)
			if !ok || key == "" || rest == "" {
				continue
			}
			out[key] = sectionInlineValue(rest)
		}
	}
	return out
}

func sectionInlineValue(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "$") {
		return "{{ " + rest + " }}"
	}
	return strings.Trim(rest, `'"`)
}

// splitSectionArgs parses section('name') / section('name', …) args string.
func splitSectionArgs(args string) (name, rest string, ok bool) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", "", false
	}
	i := 0
	if i < len(args) && (args[i] == '\'' || args[i] == '"') {
		q := args[i]
		i++
		var b strings.Builder
		for i < len(args) && args[i] != q {
			if args[i] == '\\' && i+1 < len(args) {
				b.WriteByte(args[i+1])
				i += 2
				continue
			}
			b.WriteByte(args[i])
			i++
		}
		if i >= len(args) || args[i] != q {
			return "", "", false
		}
		i++
		name = b.String()
	} else {
		return "", "", false
	}
	for i < len(args) && (args[i] == ' ' || args[i] == '\t') {
		i++
	}
	if i < len(args) && args[i] == ',' {
		i++
		rest = strings.TrimSpace(args[i:])
	}
	return name, rest, true
}

func applyYields(layout string, sections map[string]string) string {
	doc, err := ast.ParseSource(layout)
	if err != nil {
		return applyYieldsRegex(layout, sections)
	}
	replaced := replaceYieldsInNodes(doc.Nodes, sections)
	replaced = replaceSectionShowsInNodes(replaced, sections)
	src := nodesToSource(replaced)
	if strings.TrimSpace(src) == "" && strings.TrimSpace(layout) != "" {
		return applyYieldsRegex(layout, sections)
	}
	return src
}

func replaceSectionShowsInNodes(nodes []ast.Node, sections map[string]string) []ast.Node {
	out := make([]ast.Node, 0, len(nodes))
	for _, n := range nodes {
		switch t := n.(type) {
		case ast.Block:
			if strings.ToLower(t.Name) == "section" && t.End == "show" {
				key, _, ok := splitSectionArgs(t.Args)
				if ok {
					if body, found := sections[key]; found {
						out = append(out, ast.Text{Value: body})
						continue
					}
					// No child section: render the @show default body.
					out = append(out, t.Body...)
					continue
				}
			}
			if strings.ToLower(t.Name) == "section" && t.End != "show" {
				// Definition-only leftovers in a layout after extract — drop.
				continue
			}
			cp := t
			cp.Body = replaceSectionShowsInNodes(t.Body, sections)
			out = append(out, cp)
		default:
			out = append(out, n)
		}
	}
	return out
}

func replaceYieldsInNodes(nodes []ast.Node, sections map[string]string) []ast.Node {
	out := make([]ast.Node, 0, len(nodes))
	for _, n := range nodes {
		switch t := n.(type) {
		case ast.Directive:
			if strings.ToLower(t.Name) != "yield" {
				out = append(out, t)
				continue
			}
			key, rest, ok := splitSectionArgs(t.Args)
			if !ok {
				out = append(out, t)
				continue
			}
			if body, found := sections[key]; found {
				out = append(out, ast.Text{Value: body})
				continue
			}
			if rest != "" {
				out = append(out, ast.Text{Value: sectionInlineValue(rest)})
				continue
			}
			out = append(out, ast.Text{Value: ""})
		case ast.Block:
			cp := t
			cp.Body = replaceYieldsInNodes(t.Body, sections)
			out = append(out, cp)
		default:
			out = append(out, n)
		}
	}
	return out
}

func nodesToSource(nodes []ast.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		switch t := n.(type) {
		case ast.Text:
			// Lexer emits a single "@" Text token for "@@"; re-double so a later
			// ParseSource does not treat the following letters as a directive.
			if t.Value == "@" {
				b.WriteString("@@")
			} else {
				b.WriteString(t.Value)
			}
		case ast.Echo:
			b.WriteString("{{ ")
			b.WriteString(t.Expr)
			b.WriteString(" }}")
		case ast.RawEcho:
			b.WriteString("{!! ")
			b.WriteString(t.Expr)
			b.WriteString(" !!}")
		case ast.Comment:
			b.WriteString("{{-- ")
			b.WriteString(t.Body)
			b.WriteString(" --}}")
		case ast.Directive:
			b.WriteByte('@')
			b.WriteString(t.Name)
			if strings.TrimSpace(t.Args) != "" {
				b.WriteByte('(')
				b.WriteString(t.Args)
				b.WriteByte(')')
			}
		case ast.Block:
			writeBlockSource(&b, t)
		default:
			b.WriteString(fmt.Sprintf("<!-- unsupported node %T -->", n))
		}
	}
	return b.String()
}

func writeBlockSource(b *strings.Builder, blk ast.Block) {
	name := blk.Name
	b.WriteByte('@')
	b.WriteString(name)
	if strings.TrimSpace(blk.Args) != "" {
		b.WriteByte('(')
		b.WriteString(blk.Args)
		b.WriteByte(')')
	}
	b.WriteByte('\n')
	b.WriteString(nodesToSource(blk.Body))
	b.WriteByte('\n')
	if blk.End == "show" {
		b.WriteString("@show")
		return
	}
	b.WriteString("@end")
	b.WriteString(name)
}
