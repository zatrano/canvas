# Canvas directives (golden catalog)

Paket: `github.com/zatrano/canvas` · Marka: **Canvas** · Zatrano dizini: `templates/` · Framework API: `http.Template("name", data)`.

Bu dosya V3 DX golden özetidir. Davranış testleri `*_test.go` altındadır.
Tokenizer: `lex/` · AST: `ast/` (Nest blocks; foreach/if/auth leaf lower; layout/forelse/compare still regex in Engine).

## Layouts

| Directive | Anlam |
|-----------|--------|
| `@extends('layouts.app')` | Parent layout |
| `@section('content')` … `@endsection` | Bölüm tanımı |
| `@yield('content')` | Parent’ta bölüm yeri |
| `@parent` | Section içinde parent içeriği |
| `@include('partials.x')` | Include |
| `@includeIf` / `@includeWhen` / `@includeUnless` / `@includeFirst` | Koşullu include |
| `@push('scripts')` … `@endpush` / `@stack('scripts')` | Stack |
| `@once` … `@endonce` | Tek seferlik blok |

## Control flow

| Directive | Not |
|-----------|-----|
| `@if` / `@elseif` / `@else` / `@endif` | Go template if |
| `@unless` / `@endunless` | Ters if |
| `@isset` / `@empty` | Varlık |
| `@switch` / `@case` / `@default` / `@endswitch` | Switch |
| `@foreach` / `@forelse` / `@empty` / `@endforelse` | Döngü |
| `@each` | Kısa foreach |

## Components

| Biçim | Not |
|-------|-----|
| `<x-alert type="info">…</x-alert>` | x-tag |
| `@component` / `@slot` / `@props` / `@aware` | Blade-style |
| `@class` / `@style` | Attribute bag |

## Forms / auth helpers

| Directive | Çıktı |
|-----------|--------|
| `@csrf` | `<input type="hidden" name="_token" …>` |
| `@csrfMeta` | `<meta name="csrf-token" …>` |
| `@method('PUT')` | Spoof method field |
| `@auth` / `@guest` / `@can` | Auth gate (bağlıysa) |

## Misc

| Directive | Not |
|-----------|-----|
| `@json($x)` | JSON encode |
| `@lang` / `@choice` | Localization (bağlıysa) |
| `@verbatim` … `@endverbatim` | Ham blok |
| `@php` | **Strip** (desteklenmez; Go’ya taşı) |

## HTTP

```go
return http.Template("web.welcome", map[string]any{"name": "Ada"})
// → templates/web/welcome.html
```

Hata mesajları dosya yolunu içerir: `Canvas template [name] (path) compile/parse error: …`.
