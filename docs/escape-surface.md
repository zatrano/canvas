# Escape surface inventory

Generated from `docs/directives.md`, `lex.KnownDirectives`, and `engine.go` / `layouts.go` / `rt` (not assumed).

Legend: **out** = produces HTML output · **escape** = passes through escape pipeline · **oracle** = combinatorial/fuzz token · **mut** = mutation rule.

| Directive / construct | out? | Escape path | oracle | mut |
|----------------------|------|-------------|--------|-----|
| `{{ $x }}` | yes | contextual: html / unquoted-attr / url / css / **forbid** (script, style, on*, srcdoc, tag, attr-name) | yes | yes (escape_ctx) |
| `{!! $x !!}` | yes | **raw** (legacy/trusted only; Prefer SafeHTML) | partial | — |
| `@json` / `@js` | yes | json (+ HTML escape in attrs); forbid in unquoted/on*/srcdoc/style-el | yes | yes |
| `@attrs($map)` | yes | attr bag: name regex, reject on*/srcdoc/style/srcset; values html-escape; URL → scheme filter; SafeHTML **not** raw | yes | yes (7 rules → subtests) |
| `@class` / `@style` | yes | attr helpers (values escaped into class/style) | — | — |
| `@checked`/`@selected`/`@disabled`/… | yes | boolean attr name only (no value) | — | — |
| `@csrf` / `@csrfMeta` / `@method` | yes | token/method via `{{ }}` → html escape | yes (extra) | — |
| `@error` … `@enderror` | conditional | body echoes still contextual | — | — |
| `@lang` / `@choice` | yes | catalog **trusted in text**; **attr-escape** in attributes; params **always** HTML-escaped; `SetLangEscapeCatalog(true)` opt-in catalog escape; Strict forbid in script/style/tag | yes (extra) | yes (`lang-param-escape`, `lang-catalog-trusted`, `lang-catalog-escape-opt`, `lang-attr-context`) |
| `@section('t', $x)` + `@yield('t')` | yes | short-var → `{{ $x }}` then contextual escape at yield site | — | — |
| `@yield('x', 'lit')` | yes | literal default (static text) | — | — |
| `@yield('x', $default)` | yes | → `{{ $default }}` → contextual escape at yield site | yes (extra) | yes (`yield-default-var-escape`) |
| `@yield` / `@stack` / `{{ $slot }}` / `@include*` call sites | — | **forbid** in script/style/tag/attr/comment (placement) | yes | yes (directive×context + root) |
| `@each` | yes | partial body compiled with escape | — | placement via include-like expand |
| `@includeIf`/`When`/`Unless`/`First` | yes | same as include; call-site placement forbid | yes (include) | yes |
| `@verbatim` | yes | raw body; following echoes scan **expanded** HTML | yes | — |
| `@@` | text | literal `@` | — | — |
| `@dump` / `@dd` | **no** | not implemented (not in KnownDirectives); production has no dump path | — | — |
| `@asset` / `@vite` / `@route` / `@url` | **no** | not implemented — use `{{ $u }}` in href/src/action (scheme filter) or `@attrs` | — | — |
| `rt.SafeHTML` / `template.HTML` | yes | **raw** in text; **escaped** inside `@attrs` | — | yes |

## Notes

- **Translation HTML:** catalog strings are trusted in **text** by default. Parameters are always HTML-escaped. Attributes always attr-escape the result. Opt-in: `Engine.SetLangEscapeCatalog(true)` for tenant-editable catalogs. See [strict-gaps-lang-yield.md](strict-gaps-lang-yield.md).
- **`@yield('x', $default)`:** missing section inserts `{{ $default }}` so Strict context rules apply at the yield site.
- **URL helpers:** Canvas has no `@asset`/`@route`. URLs belong in `{{ $u }}` or `@attrs` (scheme filter at value start).
- **Surfaces that skip escape:** intentional raw channels — `{!! !!}`, `rt.SafeHTML`/`template.HTML` in text, `@verbatim` body text, and **trusted `@lang` catalogs in text** (params still escaped).

## Tests locking this inventory

- `TestEscapeSurface_*` (section short, yield defaults, lang, each/includeWhen, dump/dd absence, csrf/class)
- `TestLayoutPlacement_StrictForbiddenCallSites` (directive×context matrix)
- `TestAttrs_Strict` (+ mutation subtests)
- `TestStrictExtra_DirectiveAttrPayload`
