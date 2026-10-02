package ast

import (
	"fmt"
	"strings"
)

// Block is a paired directive with a nested body (foreach, if, …).
type Block struct {
	Name string
	Args string
	Body []Node
	// End is the closer that terminated the block ("endif", "show", …).
	End string
}

func (Block) node() {}

// openers that require a matching end* closer.
var blockEnd = map[string]string{
	"foreach":    "endforeach",
	"forelse":    "endforelse",
	"if":         "endif",
	"unless":     "endunless",
	"isset":      "endisset",
	"empty":      "endempty",
	"auth":       "endauth",
	"guest":      "endguest",
	"error":      "enderror",
	"can":        "endcan",
	"cannot":     "endcannot",
	"env":        "endenv",
	"production": "endproduction",
	"section":    "endsection",
	"push":       "endpush",
	"prepend":    "endprepend",
	"once":       "endonce",
	"component":  "endcomponent",
	"slot":       "endslot",
}

// Alternate closers for a primary end name (Blade @section … @show).
var blockEndAlt = map[string][]string{
	"endsection": {"show"},
}

// DefaultMaxNestingDepth is used when Nest / ParseSource get maxDepth <= 0.
const DefaultMaxNestingDepth = 200

// Nest folds flat directive pairs into Block nodes (default depth limit).
func Nest(nodes []Node) ([]Node, error) {
	return NestDepth(nodes, DefaultMaxNestingDepth)
}

// NestDepth is Nest with an explicit max nesting depth for @if/@foreach/….
func NestDepth(nodes []Node, maxDepth int) ([]Node, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxNestingDepth
	}
	out, rest, _, err := nestUntil(nodes, "", 0, maxDepth)
	if err != nil {
		return nil, err
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("canvas ast: unexpected tokens after nest")
	}
	return out, nil
}

// nestUntil returns body, remaining nodes, and the closer name that ended the
// region (empty at top level).
func nestUntil(nodes []Node, endName string, depth, maxDepth int) (body []Node, rest []Node, closedBy string, err error) {
	for len(nodes) > 0 {
		n := nodes[0]
		nodes = nodes[1:]
		d, ok := n.(Directive)
		if !ok {
			body = append(body, n)
			continue
		}
		if endName != "" && (d.Name == endName || isAltCloser(endName, d.Name)) {
			return body, nodes, d.Name, nil
		}
		if closer, isOpen := blockEnd[d.Name]; isOpen {
			// Bare @empty is the @forelse empty-section marker, not @empty($x).
			if d.Name == "empty" && strings.TrimSpace(d.Args) == "" {
				body = append(body, d)
				continue
			}
			// @section('name', 'value') / @section('name', $var) — inline, no body.
			if d.Name == "section" && sectionShortArgs(d.Args) {
				body = append(body, d)
				continue
			}
			if depth+1 > maxDepth {
				return nil, nil, "", fmt.Errorf("canvas ast: nesting depth exceeds MaxNestingDepth (%d)", maxDepth)
			}
			inner, after, usedCloser, nerr := nestUntil(nodes, closer, depth+1, maxDepth)
			if nerr != nil {
				return nil, nil, "", nerr
			}
			body = append(body, Block{Name: d.Name, Args: d.Args, Body: inner, End: usedCloser})
			nodes = after
			continue
		}
		body = append(body, d)
	}
	if endName != "" {
		return nil, nil, "", fmt.Errorf("canvas ast: missing @%s", endName)
	}
	return body, nil, "", nil
}

func isAltCloser(endName, name string) bool {
	for _, alt := range blockEndAlt[endName] {
		if alt == name {
			return true
		}
	}
	return false
}

// sectionShortArgs reports Blade inline @section('name', …) (no @endsection/@show).
func sectionShortArgs(args string) bool {
	args = strings.TrimSpace(args)
	if args == "" {
		return false
	}
	// Skip first quoted name, then look for a comma-separated second arg.
	i := 0
	if i < len(args) && (args[i] == '\'' || args[i] == '"') {
		q := args[i]
		i++
		for i < len(args) && args[i] != q {
			if args[i] == '\\' && i+1 < len(args) {
				i += 2
				continue
			}
			i++
		}
		if i < len(args) && args[i] == q {
			i++
		}
	} else {
		return false
	}
	for i < len(args) && (args[i] == ' ' || args[i] == '\t') {
		i++
	}
	return i < len(args) && args[i] == ','
}
