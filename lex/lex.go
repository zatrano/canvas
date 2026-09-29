// Package lex is the Canvas tokenizer foundation.
//
// Goal: replace the Engine regex pipeline with lex → AST → compile.
// Usable today for tooling; remaining regex paths shrink as AST coverage grows.
package lex

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind classifies a Canvas token.
type Kind int

const (
	KindText Kind = iota
	KindDirective
	KindEcho
	KindRawEcho
	KindComment
	KindEOF
)

func (k Kind) String() string {
	switch k {
	case KindText:
		return "Text"
	case KindDirective:
		return "Directive"
	case KindEcho:
		return "Echo"
	case KindRawEcho:
		return "RawEcho"
	case KindComment:
		return "Comment"
	case KindEOF:
		return "EOF"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// Token is one Canvas lexeme.
type Token struct {
	Kind   Kind
	Name   string // directive name without @ (csrf, extends, …)
	Args   string // raw args inside (…) if present
	Lit    string // full matched text / echo body
	Line   int
	Column int
	Offset int
}

// Lex scans Canvas source into tokens (text, @directives, {{ }}, {!! !!}, {{-- --}}).
func Lex(src string) ([]Token, error) {
	l := &lexer{src: src, line: 1, col: 1}
	return l.run()
}

type lexer struct {
	src    string
	pos    int
	line   int
	col    int
	tokens []Token
}

func (l *lexer) run() ([]Token, error) {
	for l.pos < len(l.src) {
		if strings.HasPrefix(l.src[l.pos:], "{{--") {
			if err := l.scanComment(); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(l.src[l.pos:], "{!!") {
			if err := l.scanRawEcho(); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(l.src[l.pos:], "{{") {
			if err := l.scanEcho(); err != nil {
				return nil, err
			}
			continue
		}
		if l.src[l.pos] == '@' {
			if err := l.scanDirective(); err != nil {
				return nil, err
			}
			continue
		}
		l.scanText()
	}
	l.emit(KindEOF, "", "", "", l.pos)
	return l.tokens, nil
}

func (l *lexer) scanText() {
	start := l.pos
	startLine, startCol := l.line, l.col
	for l.pos < len(l.src) {
		if l.src[l.pos] == '@' || strings.HasPrefix(l.src[l.pos:], "{{") || strings.HasPrefix(l.src[l.pos:], "{!!") {
			break
		}
		l.advance()
	}
	if l.pos > start {
		l.tokens = append(l.tokens, Token{
			Kind: KindText, Lit: l.src[start:l.pos],
			Line: startLine, Column: startCol, Offset: start,
		})
	}
}

func (l *lexer) scanDirective() error {
	start := l.pos
	startLine, startCol := l.line, l.col
	l.advance() // @
	nameStart := l.pos
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		l.pos += size
		l.col++
	}
	if l.pos == nameStart {
		// Lone @ — treat as text.
		l.tokens = append(l.tokens, Token{
			Kind: KindText, Lit: "@",
			Line: startLine, Column: startCol, Offset: start,
		})
		return nil
	}
	name := l.src[nameStart:l.pos]
	args := ""
	l.skipSpace()
	if l.pos < len(l.src) && l.src[l.pos] == '(' {
		argStart := l.pos
		if err := l.skipBalanced('(', ')'); err != nil {
			return fmt.Errorf("canvas lex line %d:%d: %w", startLine, startCol, err)
		}
		inner := l.src[argStart+1 : l.pos-1]
		args = strings.TrimSpace(inner)
	}
	lit := l.src[start:l.pos]
	l.tokens = append(l.tokens, Token{
		Kind: KindDirective, Name: name, Args: args, Lit: lit,
		Line: startLine, Column: startCol, Offset: start,
	})
	return nil
}

func (l *lexer) scanEcho() error {
	start := l.pos
	startLine, startCol := l.line, l.col
	l.pos += 2
	l.col += 2
	bodyStart := l.pos
	for l.pos < len(l.src) {
		if strings.HasPrefix(l.src[l.pos:], "}}") {
			body := strings.TrimSpace(l.src[bodyStart:l.pos])
			l.pos += 2
			l.col += 2
			l.tokens = append(l.tokens, Token{
				Kind: KindEcho, Lit: body,
				Line: startLine, Column: startCol, Offset: start,
			})
			return nil
		}
		l.advance()
	}
	return fmt.Errorf("canvas lex line %d:%d: unclosed {{", startLine, startCol)
}

func (l *lexer) scanRawEcho() error {
	start := l.pos
	startLine, startCol := l.line, l.col
	l.pos += 3
	l.col += 3
	bodyStart := l.pos
	for l.pos < len(l.src) {
		if strings.HasPrefix(l.src[l.pos:], "!!}") {
			body := strings.TrimSpace(l.src[bodyStart:l.pos])
			l.pos += 3
			l.col += 3
			l.tokens = append(l.tokens, Token{
				Kind: KindRawEcho, Lit: body,
				Line: startLine, Column: startCol, Offset: start,
			})
			return nil
		}
		l.advance()
	}
	return fmt.Errorf("canvas lex line %d:%d: unclosed raw echo", startLine, startCol)
}

func (l *lexer) scanComment() error {
	start := l.pos
	startLine, startCol := l.line, l.col
	l.pos += 4
	l.col += 4
	bodyStart := l.pos
	for l.pos < len(l.src) {
		if strings.HasPrefix(l.src[l.pos:], "--}}") {
			body := l.src[bodyStart:l.pos]
			l.pos += 4
			l.col += 4
			l.tokens = append(l.tokens, Token{
				Kind: KindComment, Lit: body,
				Line: startLine, Column: startCol, Offset: start,
			})
			return nil
		}
		l.advance()
	}
	return fmt.Errorf("canvas lex line %d:%d: unclosed {{--", startLine, startCol)
}

func (l *lexer) skipBalanced(open, close byte) error {
	if l.pos >= len(l.src) || l.src[l.pos] != open {
		return fmt.Errorf("expected %q", open)
	}
	depth := 0
	inSingle, inDouble := false, false
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if inSingle {
			if ch == '\\' && l.pos+1 < len(l.src) {
				l.advance()
				l.advance()
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			l.advance()
			continue
		}
		if inDouble {
			if ch == '\\' && l.pos+1 < len(l.src) {
				l.advance()
				l.advance()
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			l.advance()
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case open:
			depth++
		case close:
			depth--
			l.advance()
			if depth == 0 {
				return nil
			}
			continue
		}
		l.advance()
	}
	return fmt.Errorf("unbalanced %q", open)
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if r != ' ' && r != '\t' {
			return
		}
		l.pos += size
		l.col++
	}
}

func (l *lexer) advance() {
	if l.pos >= len(l.src) {
		return
	}
	r, size := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += size
	if r == '\n' {
		l.line++
		l.col = 1
		return
	}
	l.col++
}

func (l *lexer) emit(kind Kind, name, args, lit string, offset int) {
	l.tokens = append(l.tokens, Token{
		Kind: kind, Name: name, Args: args, Lit: lit,
		Line: l.line, Column: l.col, Offset: offset,
	})
}

// KnownDirectives lists golden directive names from docs/directives.md (lexer recognition set).
var KnownDirectives = []string{
	"extends", "section", "endsection", "yield", "parent", "include",
	"includeIf", "includeWhen", "includeUnless", "includeFirst",
	"push", "endpush", "stack", "once", "endonce",
	"if", "elseif", "else", "endif", "unless", "endunless",
	"isset", "empty", "switch", "case", "default", "endswitch",
	"foreach", "forelse", "endforelse", "each",
	"component", "endcomponent", "slot", "endslot", "props", "aware",
	"class", "style",
	"csrf", "csrfMeta", "method", "auth", "guest", "can",
	"json", "lang", "choice", "verbatim", "endverbatim", "php",
}
