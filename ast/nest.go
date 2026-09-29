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

// Nest folds flat directive pairs into Block nodes.
func Nest(nodes []Node) ([]Node, error) {
	out, rest, err := nestUntil(nodes, "")
	if err != nil {
		return nil, err
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("canvas ast: unexpected tokens after nest")
	}
	return out, nil
}

func nestUntil(nodes []Node, endName string) (body []Node, rest []Node, err error) {
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
			inner, after, nerr := nestUntil(nodes, closer)
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
