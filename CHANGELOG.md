# Changelog

## [0.2.0] - 2026-10-02

### Breaking

- **`EscapeStrict` is the default** — contextual HTML escaping (URL scheme filter, unquoted attr escaping, compile-time forbids for `on*` / `srcdoc` / whole `style` value / `<script>`/`<style>` body / tag & attribute-name interpolation). Opt out with `Engine.SetEscapeMode(rt.EscapeLegacy)`.
- **Layout call sites forbidden** in script, style, tag, attribute, and HTML-comment contexts: `@yield` / `@stack` / `{{ $slot }}` / `@include` / `@extends` / `@component` (context name in the error). Root layout files use the same placement forbid pass.
- **Unrecognized `@end<name>` closers** are a Strict compile error (Legacy leaves them as literal text).
- **Unquoted `style=…` interpolations** are forbidden.
- **`@lang` catalog vs params:** catalog trusted in text; params always HTML-escaped; attributes always attr-escape; `SetLangEscapeCatalog(true)` for tenant-editable catalogs. Differs from v0.1.0 catalog-XSS surface (params were not interpolated on the default path).
- **`@yield('name', $default)`** now expands missing sections to `{{ $default }}` (contextual escape). On v0.1.0 the form was left as literal text.
- **`rt.SafeHTML` inside `@attrs` values** remains HTML-escaped (never raw in attributes).

### Added

#### Strict escaping (default)

- Contextual HTML escaping is **on by default** (`EscapeStrict`): URL scheme filter, unquoted attr escaping, compile-time forbid for `on*` / `srcdoc` / whole `style` value / `<script>`/`<style>` body / tag & attribute-name interpolation.
- Migration off-ramp: `Engine.SetEscapeMode(rt.EscapeLegacy)`. See [SECURITY.md](SECURITY.md), [docs/strict-gaps.md](docs/strict-gaps.md), [docs/strict-overrejection.md](docs/strict-overrejection.md).

#### SafeHTML

- `rt.SafeHTML` and `html/template.HTML` in render data are trusted markup (raw). Plain `string` stays escaped. **`rt.SafeHTML` inside `@attrs` values is still HTML-escaped** (never raw in attributes).

#### `@attrs($map)`

- Safe attribute bags in tag attribute position: sorted `map[string]string` / `map[string]any`, or ordered `rt.Attrs`; name regex + reject `on*` / `srcdoc` / `style` / `srcset` / `xmlns*`; URL attrs use scheme filter (`#unsafe`).

#### `@js` / `@json`

- Script-safe JSON / JS string helpers; forbidden in unquoted attrs, `on*`, `srcdoc`, and `<style>` element bodies.

#### MaxNestingDepth

- `Engine.MaxNestingDepth` (default **200**) for nested `@if` / `@foreach` / …

#### Unknown `@end*` closers

- Strict mode rejects unrecognized `@end<name>` tokens (letter-only name) at directive boundaries with a compile error pointing at `docs/directives.md`. Legacy mode leaves them as literal text.

#### `canvas_poison` build tag

- Optional `ReleaseWriter` fill (`0xDE`) to catch use-after-release of `Bytes()` aliases. Default builds have zero cost (`poison_stub`). Prefer `CloneBytes()` / `String()` when retaining output.

### Changed

- Layout call sites: `@yield` / `@stack` / `{{ $slot }}` / `@include` / `@extends` / `@component` forbidden in script, style, tag, attribute, and HTML-comment contexts (context name in the error).
- Writer: `CloneBytes()` for pool-safe copies.
- Benchmarks / gates that compare against other engines live in the separate `bench/` module (main `go.mod` has **zero** third-party requires).
- **`@lang` catalog vs params:** v0.1.0 XSS was catalog-only (params were not interpolated on the default path). Catalog is trusted in text (HTML in translations works); replacement values are always HTML-escaped in all modes; attributes always attr-escape; `Engine.SetLangEscapeCatalog(true)` restores full catalog escaping for tenant-editable tables. See [docs/strict-gaps-lang-yield.md](docs/strict-gaps-lang-yield.md).
- `@yield('name', $default)` is now a supported form (previously left as literal text).

### Fixed

- `@json` / AOT `writeJSON` uses `encoding/json.Marshal` default HTML escaping.
- Template path resolution via `io/fs` + `ValidPath` / `IsLocal`.
- Verbatim bodies scanned as raw HTML for escape context (placeholder is lexer-only).
- `@@` literal-`@` escape.
- Root layout files now run the same placement forbid pass as other templates.
- **`@lang`:** contextual catalog/param split + `SetLangEscapeCatalog`.
- **`@yield('name', $default)`** expands missing sections to `{{ $default }}` (contextual escape).
- Unquoted attribute escape covers HTML5 whitespace including FORM FEED (`\f`) and vertical tab (`\v`).
- Unquoted URL attrs at `name=` (before-value) use `EscURLUnquoted` so whitespace/scheme rules apply (fuzz finding).
- Unquoted `style=…` interpolations are forbidden (CSS tokens with spaces are unsafe without quotes).
- Unrecognized `@end<name>` closers are a Strict compile error (Legacy leaves them as literal text). See [docs/directives.md](docs/directives.md).

### Security

- **`@yield('x', $default)`:** not expanded on v0.1.0 (literal left in output — not XSS). New expansion path escapes via `{{ }}`.
