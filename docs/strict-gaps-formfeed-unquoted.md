# Strict gaps: FORM FEED and unquoted style/URL

Red proof: AOT path before the FORM FEED / unquoted-style harden (`html/template` already entity-encodes U+000C on the html path).

| Context | Attack payload | Pre-fix AOT output | Why exploitable |
|---------|----------------|---------------------|-----------------|
| Unquoted attr after static `/ok/` | `99000\f0` (U+000C FORM FEED) in `{{$x}}` | `<div class=/ok/99000\f0>` (raw `0x0c`) | HTML5 treats U+000C as attribute whitespace; `EscapeUnquotedAttr` only entity-escaped U+0020/`\t`/`\n`/`\r`. Tokenizer can start a new attribute after FF. |
| Unquoted URL at `attr=` (before-value) | `9*000\f` + second interp in `poster={{$a}}{{$b}}` | `<video poster=9*000\fx>` (raw FF) | `ctxBeforeValue` + URL attr returned `EscURLAttr` (quoted-URL / percent-encode path) without unquoted space escape → FF survives and splits attributes. |
| Unquoted `style=width:{{$x}}` | `10 px` (space) | `<div style=width:10 px>` | `EscCSS` / style-value path allowed spaces that HTML tokenizers treat as ending the unquoted attribute; now **compile forbid**. |

### After harden (HEAD)

| Context | Result |
|---------|--------|
| Unquoted FF | `&#12;` via `EscapeUnquotedAttr` (+ VT `&#11;`) |
| `poster=` before-value | `EscURLUnquoted` (scheme filter + unquoted escape); FF does not survive raw |
| Unquoted style interp | Strict compile error (`attribute:style`) |

### Full-byte unquoted oracle

`TestEscapeUnquotedAttr_PerChar` renders every byte `0x00–0xFF` plus Unicode whitespace (U+0085, U+00A0, U+1680, U+2000–U+200A, U+2028, U+2029, U+3000) into unquoted attr / unquoted URL / unquoted style value positions, tokenizes with a minimal HTML5 start-tag attr scanner, and asserts **no new attribute** is created.

| Context | Result |
|---------|--------|
| unquoted-attr | **PASS** 273/273 |
| unquoted-url | **PASS** 273/273 |
| unquoted-style | **PASS** 273/273 |
