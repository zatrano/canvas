package rt_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zatrano/canvas/rt"
)

// TestEscapeUnquotedAttr_PerChar covers ASCII 0x00–0xFF plus Unicode whitespace
// code points. Oracle: after render into an unquoted attribute value, an HTML5
// attribute tokenizer must not invent a new attribute.
func TestEscapeUnquotedAttr_PerChar(t *testing.T) {
	var payloads []string
	for i := 0; i < 256; i++ {
		payloads = append(payloads, string(byte(i)))
	}
	for _, r := range []rune{
		0x0085, 0x00A0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200A,
		0x2028, 0x2029, 0x3000,
	} {
		payloads = append(payloads, string(r))
	}

	contexts := []struct {
		name     string
		wrap     func(escaped string) string
		escape   func(string) string
		wantKeys []string
	}{
		{
			name: "unquoted-attr",
			wrap: func(v string) string { return `<div id=pre` + v + `suf class=x>` },
			escape: func(s string) string {
				return rt.EscapeUnquotedAttr(s)
			},
			wantKeys: []string{"id", "class"},
		},
		{
			name: "unquoted-url",
			wrap: func(v string) string { return `<a href=pre` + v + `suf class=x>` },
			escape: func(s string) string {
				u := rt.EscapeURLAttr(s, true)
				if u == "#unsafe" {
					return u
				}
				return rt.EscapeUnquotedAttr(u)
			},
			wantKeys: []string{"href", "class"},
		},
		{
			name: "unquoted-style",
			wrap: func(v string) string { return `<div style=width:` + v + ` class=x>` },
			escape: func(s string) string {
				return rt.EscapeUnquotedAttr(s)
			},
			wantKeys: []string{"style", "class"},
		},
	}

	for _, ctx := range contexts {
		t.Run(ctx.name, func(t *testing.T) {
			broke, okN := 0, 0
			for _, p := range payloads {
				esc := ctx.escape(p)
				doc := ctx.wrap(esc)
				attrs, err := html5StartTagAttrs(doc)
				if err != nil {
					t.Fatalf("tokenize %s payload=%q: %v html=%q", ctx.name, fmtPayload(p), err, doc)
				}
				if len(attrs) != len(ctx.wantKeys) {
					broke++
					t.Errorf("%s: attr count=%d want %d payload=%q html=%q attrs=%v",
						ctx.name, len(attrs), len(ctx.wantKeys), fmtPayload(p), doc, attrs)
					continue
				}
				for _, k := range ctx.wantKeys {
					if _, ok := attrs[k]; !ok {
						broke++
						t.Errorf("%s: missing %q payload=%q html=%q attrs=%v", ctx.name, k, fmtPayload(p), doc, attrs)
						break
					}
				}
				for name := range attrs {
					found := false
					for _, k := range ctx.wantKeys {
						if name == k {
							found = true
							break
						}
					}
					if !found {
						broke++
						t.Errorf("%s: NEW attr %q payload=%q html=%q", ctx.name, name, fmtPayload(p), doc)
						break
					}
				}
				okN++
			}
			t.Logf("%s: ok=%d broke=%d total=%d", ctx.name, okN, broke, len(payloads))
			if broke > 0 {
				t.Fatalf("%s: %d payloads created/lost attributes", ctx.name, broke)
			}
		})
	}
}

func fmtPayload(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); {
		r, size := utf8.DecodeRuneInString(p[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "\\x%02X", p[i])
			i++
			continue
		}
		if r < 0x20 || r == 0x7f {
			fmt.Fprintf(&b, "\\u%04X", r)
		} else {
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

func html5ASCIIWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// html5StartTagAttrs is a minimal HTML5 start-tag attribute tokenizer (zero deps).
func html5StartTagAttrs(fragment string) (map[string]string, error) {
	i := strings.IndexByte(fragment, '<')
	if i < 0 {
		return nil, fmt.Errorf("no tag")
	}
	i++
	for i < len(fragment) && !html5ASCIIWS(fragment[i]) && fragment[i] != '>' && fragment[i] != '/' {
		i++ // tag name
	}
	attrs := map[string]string{}
	for i < len(fragment) {
		for i < len(fragment) && html5ASCIIWS(fragment[i]) {
			i++
		}
		if i >= len(fragment) || fragment[i] == '>' || fragment[i] == '/' {
			return attrs, nil
		}
		nameStart := i
		for i < len(fragment) && !html5ASCIIWS(fragment[i]) && fragment[i] != '=' && fragment[i] != '>' && fragment[i] != '/' {
			i++
		}
		name := strings.ToLower(fragment[nameStart:i])
		for i < len(fragment) && html5ASCIIWS(fragment[i]) {
			i++
		}
		val := ""
		if i < len(fragment) && fragment[i] == '=' {
			i++
			for i < len(fragment) && html5ASCIIWS(fragment[i]) {
				i++
			}
			if i >= len(fragment) {
				attrs[name] = val
				return attrs, nil
			}
			switch fragment[i] {
			case '"':
				i++
				start := i
				for i < len(fragment) && fragment[i] != '"' {
					i++
				}
				val = fragment[start:i]
				if i < len(fragment) {
					i++
				}
			case '\'':
				i++
				start := i
				for i < len(fragment) && fragment[i] != '\'' {
					i++
				}
				val = fragment[start:i]
				if i < len(fragment) {
					i++
				}
			default:
				start := i
				for i < len(fragment) && !html5ASCIIWS(fragment[i]) && fragment[i] != '>' {
					// '/' is allowed inside unquoted values; only '/>' ends the tag
					// and is handled when we see '>' after optional spaces.
					i++
				}
				val = fragment[start:i]
			}
		}
		attrs[name] = val
	}
	return attrs, nil
}
