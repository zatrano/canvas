package lex_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/lex"
)

func TestLexCSRFAndEcho(t *testing.T) {
	src := `<form>@csrf {{ $name }} {!! $html !!}</form>`
	toks, err := lex.Lex(src)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	var csrfArgs string
	for _, tok := range toks {
		if tok.Kind == lex.KindEOF {
			continue
		}
		kinds = append(kinds, tok.Kind.String())
		if tok.Kind == lex.KindDirective && tok.Name == "csrf" {
			csrfArgs = tok.Args
		}
		if tok.Kind == lex.KindEcho && tok.Lit != "$name" {
			t.Fatalf("echo lit=%q", tok.Lit)
		}
		if tok.Kind == lex.KindRawEcho && tok.Lit != "$html" {
			t.Fatalf("raw echo lit=%q", tok.Lit)
		}
	}
	joined := strings.Join(kinds, ",")
	if !strings.Contains(joined, "Directive") || !strings.Contains(joined, "Echo") || !strings.Contains(joined, "RawEcho") {
		t.Fatalf("kinds=%s", joined)
	}
	if csrfArgs != "" {
		t.Fatalf("csrf args should be empty, got %q", csrfArgs)
	}
}

func TestLexExtendsArgs(t *testing.T) {
	toks, err := lex.Lex(`@extends('layouts.app')`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tok := range toks {
		if tok.Kind == lex.KindDirective && tok.Name == "extends" {
			found = true
			if tok.Args != "'layouts.app'" {
				t.Fatalf("args=%q", tok.Args)
			}
		}
	}
	if !found {
		t.Fatal("missing extends")
	}
}

func TestLexComment(t *testing.T) {
	toks, err := lex.Lex(`a{{-- hide --}}b`)
	if err != nil {
		t.Fatal(err)
	}
	var sawComment bool
	for _, tok := range toks {
		if tok.Kind == lex.KindComment {
			sawComment = true
			if !strings.Contains(tok.Lit, "hide") {
				t.Fatalf("comment=%q", tok.Lit)
			}
		}
	}
	if !sawComment {
		t.Fatal("expected comment token")
	}
}

func TestLexUnclosedEcho(t *testing.T) {
	_, err := lex.Lex(`{{ $x`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestKnownDirectivesCoverGolden(t *testing.T) {
	need := []string{"csrf", "extends", "foreach", "component", "json", "verbatim"}
	set := map[string]bool{}
	for _, d := range lex.KnownDirectives {
		set[d] = true
	}
	for _, n := range need {
		if !set[n] {
			t.Fatalf("KnownDirectives missing %q", n)
		}
	}
}
