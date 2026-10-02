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
	out, rest, err := nestUntil(nodes, "", 0, maxDepth)
	if err != nil {
		return nil, err
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("canvas ast: unexpected tokens after nest")
	}
	return out, nil
}

func nestUntil(nodes []Node, endName string, depth, maxDepth int) (body []Node, rest []Node, err error) {
	for len(nodes) > 0 {
		n := nodes[0]
		nodes = nodes[1:]
		d, ok := n.(Directive)
		if !ok {
			body = append(body, n)
			continue
		}
		if endName != "" && d.Name == endName {
			return body, nodes, nil
		}
		if closer, isOpen := blockEnd[d.Name]; isOpen {
			// Bare @empty is the @forelse empty-section marker, not @empty($x).
			if d.Name == "empty" && strings.TrimSpace(d.Args) == "" {
				body = append(body, d)
				continue
			}
			if depth+1 > maxDepth {
				return nil, nil, fmt.Errorf("canvas ast: nesting depth exceeds MaxNestingDepth (%d)", maxDepth)
			}
			inner, after, nerr := nestUntil(nodes, closer, depth+1, maxDepth)
			if nerr != nil {
				return nil, nil, nerr
			}
			body = append(body, Block{Name: d.Name, Args: d.Args, Body: inner})
			nodes = after
			continue
		}
		body = append(body, d)
	}
	if endName != "" {
		return nil, nil, fmt.Errorf("canvas ast: missing @%s", endName)
	}
	return body, nil, nil
}
