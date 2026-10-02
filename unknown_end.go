package canvas

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zatrano/canvas/lex"
	"github.com/zatrano/canvas/rt"
)

// knownClosingDirectives lists recognized @end* names (lowercase).
// Unknown closers are a Strict compile error; Legacy leaves them as literal text.
var knownClosingDirectives = map[string]struct{}{
	"endif": {}, "endforeach": {}, "endforelse": {}, "endunless": {},
	"endisset": {}, "endempty": {}, "endauth": {}, "endguest": {},
	"enderror": {}, "endcan": {}, "endcannot": {}, "endenv": {},
	"endproduction": {}, "endswitch": {}, "endsection": {}, "endpush": {},
	"endonce": {}, "endcomponent": {}, "endslot": {}, "endverbatim": {},
}

// checkUnknownClosingDirectives rejects unrecognized @endName tokens in EscapeStrict.
// Token boundary: start of input, whitespace, or '>' before '@'. Skips @@ escapes
// (lexer), {{-- --}} comments (lexer), and @verbatim bodies.
func checkUnknownClosingDirectives(tmplName, src string, mode rt.EscapeMode) error {
	if mode != rt.EscapeStrict {
		return nil
	}
	toks, lexErr := lex.Lex(src)
	if lexErr == nil {
		verbatimDepth := 0
		for _, tok := range toks {
			if tok.Kind != lex.KindDirective {
				continue
			}
			lower := strings.ToLower(tok.Name)
			switch lower {
			case "verbatim":
				verbatimDepth++
				continue
			case "endverbatim":
				if verbatimDepth > 0 {
					verbatimDepth--
				}
				continue
			}
			if verbatimDepth > 0 {
				continue
			}
			if !strings.HasPrefix(lower, "end") || len(lower) <= 3 {
				continue
			}
			if !lettersOnly(tok.Name[3:]) {
				continue
			}
			if _, ok := knownClosingDirectives[lower]; ok {
				continue
			}
			if !endDirectiveBoundary(src, tok.Offset) {
				continue
			}
			return fmt.Errorf("canvas template [%s] at %d:%d: unknown closing directive @%s; see docs/directives.md",
				tmplName, tok.Line, tok.Column, lower)
		}
	}
	// Lex failures are left to the normal parse path.
	return nil
}

func lettersOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func endDirectiveBoundary(src string, offset int) bool {
	if offset <= 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(src[:offset])
	if r == utf8.RuneError {
		return false
	}
	switch r {
	case '>', ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
