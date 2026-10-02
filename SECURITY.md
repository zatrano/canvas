# Security

## Supported versions

| Version | Supported |
|---------|-----------|
| 0.1.x | yes (best-effort) |

Report issues via GitHub. Do not open public issues for unfixed zero-days without coordinated disclosure.

## Escape modes

**Strict (default).** Contextual HTML escaping is on by default for every new `Engine`.

```go
eng := canvas.New("templates") // EscapeStrict
// Migration off-ramp for legacy templates:
eng.SetEscapeMode(rt.EscapeLegacy)
```

| Context | Strict behavior |
|---------|-----------------|
| Text / RCDATA (`<textarea>`, `<title>`) / HTML comments | HTML-escape `& < > " '` |
| Quoted attribute | HTML-escape |
| Unquoted attribute | `EscapeUnquotedAttr` (spaces/`=`/`<>"'` → entities) |
| URL attrs (`href`, `src`, `action`, `formaction`, `poster`, `ping`, `background`, `srcset`, `xlink:href`, …) at **value start** (incl. interpolation-only prefixes) | Scheme filter → dangerous schemes become `#unsafe` |
| Mid-URL without a safe static lock (`/`, `#`, `https://`, …) | Always `#unsafe` (`EscURLBlock`) |
| `on*` event handlers, `srcdoc` | **Compile error** |
| `style="{{ $x }}"` (whole value / property name) | **Compile error** |
| `style="color: {{ $c }}"` (declaration **value** after `:`) | CSS value filter → `unsafe` if charset fails or contains `/*`, `url(`, `expression(`, `\`, `;`, `}` |
| Tag name / attribute name / between-attributes interpolation | **Compile error** — use `@attrs($map)` for bags |
| `@attrs($map)` in attribute position | Safe bag: name regex, reject `on*`/`srcdoc`/`style`/`srcset`/`xmlns*`, URL scheme filter, HTML-escape values; wrong position → compile error |
| `<script>` / `<style>` body (`{{ }}`) | **Compile error** — use `@js($x)` or put data in `data-*` |
| `@include` / `@extends` / `@component` / `@yield` / `@stack` / `{{ $slot }}` inside tag / attr / script / style / HTML comment | **Compile error** (call-site context); text/`<title>` OK |
| `@json` / `@js` | `encoding/json` + attr-aware HTML; **forbidden** in unquoted attrs and `style` |
| Unrecognized `@end<name>` (letter-only, at directive boundary) | **Compile error** (Strict); Legacy → literal text |

**Style attribute:** whole-value / property-name interpolation remains a compile error. Declaration values after a static `:` use `EscapeCSSValue` (safe token charset). Migration: `style="width: {{ $w }}px"` with numeric/color tokens, or `@style($map)` / classes.

### Patterns that fail closed (fix)

```html
<iframe srcdoc="{{ $x }}">          <!-- error: srcdoc -->
<div style="{{ $x }}">              <!-- error: whole style value -->
<div style="color: {{ $c }}">       <!-- CSS filter (unsafe if bad token) -->
<a onclick="{{ $x }}">              <!-- error: on* -->
<script>{{ $x }}</script>           <!-- error: script body -->
<{{ $t }}>                          <!-- error: tag name -->
<div {{ $attrs }}>                  <!-- error: attr name -->
<a href="x" {{ $y }}>               <!-- error: between attrs -->
<script>@include('p')</script>      <!-- error: include in script -->
<a href="@yield('u')">               <!-- error: yield in attribute -->
<div title="{{ $slot }}">            <!-- error: $slot in attribute -->
<script>@stack('s')</script>         <!-- error: stack in script -->
```

**Fixes:** `@js($x)` for JS string contexts, `data-*` + client read, or `rt.SafeHTML` / `template.HTML` only for **trusted** markup.

### Guarantees (honest)

Canvas **v0.1** is an experimental high-performance HTML template engine.

**What Strict covers:** the table above on AOT, regex/`html/template`, and layout/component expand paths.

**What Strict still does not claim:** full `html/template` parity for every CSS/URI corner case; raw `{!! !!}`; developer `AddFunc`/`Directive` trust.

### Path loading

Include/extends/component names are resolved with `io/fs` (`os.DirFS`) after `fs.ValidPath` + `filepath.IsLocal`. Absolute paths, `..`, NUL, and empty names are rejected.

**Symlink caveat (Go &lt; 1.24):** this module targets Go 1.22, so `os.Root` / `OpenRoot` is unavailable. A symlink **inside** the templates root that points **outside** may still be followed by `DirFS`. Do not place untrusted symlinks in the template tree. On Go 1.24+ a future release may adopt `os.Root` to close that gap.

It is **not** claimed to be:

- A full HTML sanitizer for untrusted markup
- A complete contextual escaper on par with every `html/template` CSS/URI edge case
- Immune to injection when using raw echo (`{!! !!}`), custom `AddFunc`/`Directive`, or `EscapeLegacy`

## Verifiable controls

These are checks the repository runs (or generates artifacts for), not third-party audit claims:

1. **Contextual Strict escaping** — default `EscapeStrict` on AOT, regex/`html/template`, and layout expand paths (see table above)
2. **XSS oracle + exhaustive context scan** — `oracle` combinatorial catalog × render paths × attack payloads (`TestOracle_CombinatorialStrict`)
3. **Structure-aware fuzz** — `FuzzStrictStructured` / nightly coverage-guided fuzz over precompiled templates
4. **Mutation-tested rules** — `oracle/mutation_results.json` regenerated by `TestMutationGenerateResults`; each rule records a breaking test
5. **Race and poison CI jobs** — `-race` where CGO is available; `canvas_poison` build-tag jobs for Writer use-after-release

## Trust model: `AddFunc` and `Directive`

`AddFunc` and `Directive` register **developer-supplied** code into the engine.

- Anyone who can edit templates can call every registered `AddFunc` by name (e.g. `{{ envleak }}` if you registered it).
- `Directive` replacers return a **raw string** spliced into the template source before / during compile — **not** HTML-escaped by Canvas.
- If tenants or end users can author templates, treat registered funcs/directives as part of the **attack surface** (SSTI-class). Only register pure, sandboxed helpers; never expose filesystem, network, or env access unless you accept that risk.

## Mitigations present

1. Contextual Strict escape on standard echo paths (`{{ $x }}`, `rt.WriteEscaped`, typed streams) — see table above
2. Raw echo (`{!! !!}`) is explicit — operator responsibility
3. No arbitrary code-execution directives in templates
4. Writer pool: `ReleaseWriter` **resets and may reuse** the underlying buffer. A `[]byte` from `Bytes()` (or a `string` derived without copying) **aliases** that buffer and becomes **invalid after release** — it can be overwritten by the next acquire. Always finish using the bytes **before** `ReleaseWriter`, or copy them.
5. CI: gosec, govulncheck, Semgrep, Trivy on every push/PR

## Escapes and verbatim (syntax)

| Construct | Status |
|-----------|--------|
| `@verbatim` … `@endverbatim` | **Supported** — body is preserved raw through compile (`layouts.go` / `engine.go`); context tracker does not see the body (placeholder) |
| `@@` → literal `@` | **Supported** — `@@if(...)` renders as `@if(...)` text; works in attributes |

## Operator guidance

- Prefer escaped echo; avoid raw HTML unless the value is trusted (`rt.SafeHTML` / `template.HTML`)
- Do not interpolate untrusted data into URLs, event handlers, or `<script>`/`<style>` — Strict rejects the unsafe forms
- Do not pass untrusted maps into custom `AddFunc` helpers without validation
- Keep Go and Canvas versions current; run `govulncheck ./...`

## Testing

```bash
go test ./... -count=1
go test -run 'TestStrictExtra_|TestEscape' -v
gofmt -l .
go vet ./...
staticcheck ./...
govulncheck ./...
```
