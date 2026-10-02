# Canvas directives (golden catalog)

Package: `github.com/zatrano/canvas` · Brand: **Canvas** · Typical template root: `templates/` · Host API example: `http.Template("name", data)`.

This file is the DX golden summary. Behavior is locked by `*_test.go` suites.
Tokenizer: `lex/` · AST: `ast/` (nest blocks; foreach/forelse/if(+simple compare)/auth leaf lower; layout/switch/complex if_expr still regex in Engine).

## Layouts

| Directive | Meaning |
|-----------|---------|
| `@extends('layouts.app')` | Parent layout |
| `@section('content')` … `@endsection` | Section definition |
| `@yield('content')` | Section outlet in the parent |
| `@parent` | Parent section body inside a child section |
| `@include('partials.x')` | Include |
| `@includeIf` / `@includeWhen` / `@includeUnless` / `@includeFirst` | Conditional include |
| `@push('scripts')` … `@endpush` / `@stack('scripts')` | Stack |
| `@once` … `@endonce` | Render-once block |

## Control flow

| Directive | Notes |
|-----------|-------|
| `@if` / `@elseif` / `@else` / `@endif` | Go template if |
| `@unless` / `@endunless` | Negated if |
| `@isset` / `@empty` | Presence / emptiness |
| `@switch` / `@case` / `@default` / `@endswitch` | Switch |
| `@foreach` / `@forelse` / `@empty` / `@endforelse` | Loop |
| `@each` | Short foreach |

## Components

| Form | Notes |
|------|-------|
| `<x-alert type="info">…</x-alert>` | x-tag |
| `@component` / `@slot` / `@props` / `@aware` | Components |
| `@class` / `@style` | Attribute bag |

## Forms / auth helpers

| Directive | Output |
|-----------|--------|
| `@csrf` | `<input type="hidden" name="_token" …>` |
| `@csrfMeta` | `<meta name="csrf-token" …>` |
| `@method('PUT')` | Spoof method field |
| `@auth` / `@guest` / `@can` | Auth gate (when wired) |

## Misc

| Directive | Notes |
|-----------|-------|
| `@json($x)` / `@js($x)` | JSON encode; Strict forbids plain `{{ }}` in script/style — use `@js` |
| `@class` / `@style` | Attribute helpers — values HTML-escaped into `class`/`style` attrs |
| `@lang` / `@choice` | Localization (when wired) |
| `@verbatim` … `@endverbatim` | **Supported** — body preserved raw through compile; escape context scanner skips the body |
| `@@` → literal `@` | **Supported** — `@@if(...)` → `@if(...)` text (also in attributes) |

The engine has no code-execution directive.

Unrecognized directives (including single-line forms without a closer) emit as literal text;
`@end<name>` closers that are not a recognized directive pair are a **Strict** compile error
(`unknown closing directive @end…; see docs/directives.md`). Legacy mode leaves them as literal text.

JS strings / HTML comments that contain `@foreach` (etc.) are still tokenized as directives (directive tokenization rules). Escape with `@verbatim` when you need a literal `@`.

## HTTP

```go
return http.Template("web.welcome", map[string]any{"name": "Ada"})
// → templates/web/welcome.html
```

Error messages include the file path: `Canvas template [name] (path) compile/parse error: …`.
