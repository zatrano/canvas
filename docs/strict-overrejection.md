# Combinatorial compile-error groups

Counts are over unique templates × 3 render paths × `AttackPayloads` (same cells as `TestOracle_CombinatorialStrict`). Compile failure is payload-independent; each (tmpl, path) is counted × payload count.

| context | position | expr | compile_errors | compiles_ok |
|---------|----------|------|----------------:|------------:|
| attr-name | value-start | {{}} | 90 | 0 |
| attrs-bag | value-start | @attrs | 0 | 90 |
| between-attrs | value-start | {{}} | 180 | 0 |
| template-comment-script | value-start | {{}} | 0 | 90 |
| comment | value-start | @js | 0 | 90 |
| comment | value-start | @json | 0 | 90 |
| comment | value-start | {{}} | 0 | 90 |
| double-quoted | after-static | @js | 270 | 990 |
| double-quoted | after-static | @json | 270 | 990 |
| double-quoted | after-static | {{}} | 180 | 1080 |
| double-quoted | mid-value | @js | 270 | 990 |
| double-quoted | mid-value | @json | 270 | 990 |
| double-quoted | mid-value | {{}} | 180 | 1080 |
| double-quoted | multi-interp | @js | 270 | 990 |
| double-quoted | multi-interp | @json | 270 | 990 |
| double-quoted | multi-interp | {{}} | 270 | 990 |
| double-quoted | value-start | @js | 270 | 990 |
| double-quoted | value-start | @json | 270 | 990 |
| double-quoted | value-start | {{}} | 450 | 990 |
| single-quoted | after-static | @js | 270 | 990 |
| single-quoted | after-static | @json | 270 | 990 |
| single-quoted | after-static | {{}} | 180 | 1080 |
| single-quoted | mid-value | @js | 270 | 990 |
| single-quoted | mid-value | @json | 270 | 990 |
| single-quoted | mid-value | {{}} | 180 | 1080 |
| single-quoted | multi-interp | @js | 270 | 990 |
| single-quoted | multi-interp | @json | 270 | 990 |
| single-quoted | multi-interp | {{}} | 270 | 990 |
| single-quoted | value-start | @js | 270 | 990 |
| single-quoted | value-start | @json | 270 | 990 |
| single-quoted | value-start | {{}} | 270 | 990 |
| tag-name | value-start | {{}} | 90 | 0 |
| text | value-start | @js | 0 | 90 |
| text | value-start | @json | 0 | 90 |
| text | value-start | {{}} | 270 | 360 |
| textarea | value-start | @js | 0 | 90 |
| textarea | value-start | @json | 0 | 90 |
| textarea | value-start | {{}} | 0 | 90 |
| title | value-start | @js | 0 | 90 |
| title | value-start | @json | 0 | 90 |
| title | value-start | {{}} | 0 | 180 |
| unquoted | after-static | @js | 1260 | 0 |
| unquoted | after-static | @json | 1260 | 0 |
| unquoted | after-static | {{}} | 270 | 990 |
| unquoted | mid-value | @js | 1260 | 0 |
| unquoted | mid-value | @json | 1260 | 0 |
| unquoted | mid-value | {{}} | 270 | 990 |
| unquoted | multi-interp | @js | 1260 | 0 |
| unquoted | multi-interp | @json | 1260 | 0 |
| unquoted | multi-interp | {{}} | 270 | 990 |
| unquoted | value-start | @js | 1260 | 0 |
| unquoted | value-start | @json | 1260 | 0 |
| unquoted | value-start | {{}} | 360 | 990 |
| verbatim-open-script | value-start | {{}} | 90 | 0 |
| verbatim-open-style | value-start | {{}} | 90 | 0 |

Total compile_errors: 18360
Total compiles_ok: 29700
Unique templates always-forbid: 204, always-ok: 330

## Expected forbid rules (must match groups above)

- tag-name / attr-name / between-attrs: `{{ }}` only → forbid
- unquoted `@json` / `@js`: always forbid
- on* / srcdoc (any quote style, any expr): forbid
- style attr whole-value / multi-interp: forbid; after `prop:` → CSS filter (compiles)
- style attr `@json`/`@js`: forbid
- text / comment / title / textarea: compile OK

## Migration: common patterns that fail closed

| Pattern | Why | Supported safe alternative | Fix |
|---------|-----|----------------------------|-----|
| `<div {{ $attrs }}>` (attribute bag) | attribute-name interpolation forbidden | **VAR** (`@attrs($attrs)`) | Use `@attrs($map)` (name regex, reject on*/srcdoc/style/srcset, URL scheme filter). |
| `<option {{ $sel }}>` | same | **VAR** (partial: `@selected`) | Prefer `@selected($cond)`. |
| `<input {{ $attrs }}>` | same | **VAR** (`@attrs($attrs)`) | `@attrs($map)` and/or boolean directives. |
| `<a href="{{ $u }}">` untrusted scheme | URL filter → `#unsafe` | **VAR** | Validate scheme or static `https://` prefix. |
| `<div style="{{ $x }}">` | whole style value forbidden | **VAR** | `style="color: {{ $c }}"` or classes. |
| Unquoted `style=width:{{ $x }}` / `style=color:{{ $x }};` | spaces split unquoted attrs; now forbid | **VAR** | Quote: `style="width: {{ $x }}"` (EscCSS after `:`) or use classes. |
| `<a onclick="…">` / `@js` / `@json` | on* forbidden | **VAR** | `data-*` + JS, or `@js` in `<script>`. |
| Unquoted `@json`/`@js` | spaces break attrs | **VAR** | Quote: `data-x="@json($x)"`. |
| `<iframe srcdoc="…">` | srcdoc forbidden | **YOK** | Outside templates or intentional SafeHTML. |
| `<script>@yield('js')</script>` / `<a href="@yield('u')">` | yield call site in script/attr | **YOK** (fail-closed) | Put `@yield` in text/`<title>` only; pass data via `@js` / attrs. |
| `<div title="{{ $slot }}">` | `$slot` in attribute | **YOK** | Keep `{{ $slot }}` in element body text. |
| `@lang('k')` with HTML in catalog | trusted in text by default | **VAR** | Tenant-editable catalogs: `SetLangEscapeCatalog(true)`. Attr always escaped; params always escaped. |

### Still without a full bag helper

- `<option {{ $sel }}>` beyond `@selected`
- `srcdoc` interpolation (intentional fail-closed)

## Over-rejection delta (unquoted-style forbid: 18180 → 18360 compile errors)

+180 cells (= 2 templates × 3 paths × 30 payloads). Both are **new unquoted-style forbids**:

| template | context | position | expr |
|----------|---------|----------|------|
| `<div style=width:{{ $x }}>` | unquoted | after-static | `{{}}` |
| `<div style=color:{{ $x }};>` | unquoted | mid-value | `{{}}` |

Previously these used `EscCSS` and compiled; spaces in CSS tokens could split attributes. **Not** a safe-and-common pattern — fix by quoting the attribute (table above).
