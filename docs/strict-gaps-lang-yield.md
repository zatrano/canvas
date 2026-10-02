# Strict gaps: @lang catalog vs parameters

## 1a) Catalog vs parameter matrix

Payloads: `<script>alert(1)</script>` · `"><img onerror=x>`  
Paths measured: published **v0.1.0**, pre-split catalog trust, and **HEAD** (post-split).  
AOT for bare `@lang`; html/layout for replacements.

| Placement | Payload site | v0.1.0 | Pre-split | HEAD (default) | Exploitable on v0.1.0? |
|-----------|--------------|--------|-----------|----------------|------------------------|
| Text `<p>@lang('k')</p>` | catalog = payload | raw `<script>…` / raw img | raw | catalog **trusted** (raw HTML) | **Yes** (catalog) |
| Attr `title="@lang('k')"` | catalog = payload | raw breakout | raw breakout | **attr-escaped** | **Yes** (catalog) |
| Text `@lang('k', ['name'=>$x])` | param = payload, catalog `Hi :name` | no interpolation (out≈`k` / unused) | same | `Hi` + **escaped** param; catalog HTML kept if present | **No** (params not applied on default path) |
| Attr `@lang(..., $x)` | param = payload | same | same | attr-escaped whole string | **No** |

**Verdict:** v0.1.0 XSS was **catalog-only**. Parameter values were not emitted by the default `canvasTrans` / AOT path.

## Policy (maintainer decision)

| Layer | Policy |
|-------|--------|
| Catalog string | **Trusted** in **text** (HTML translations work). |
| Catalog in **attribute** | Always **attr-escape** (even when catalog trusted). |
| Replacement params (`:name` / `{name}`) | **Always HTML-escape** (Strict **and** Legacy). |
| Opt-in | `Engine.SetLangEscapeCatalog(true)` — also escape catalog (tenant-editable tables). Default **false**. |
| script / style / tag | Strict **compile error**. |

## Mutation locks

| Rule | Breaking subtest |
|------|------------------|
| `lang-param-escape` | `TestEscapeSurface_LangParamEscape/html/strict/text` |
| `lang-catalog-trusted` | `TestEscapeSurface_LangCatalogTrusted/aot/text_raw` |
| `lang-catalog-escape-opt` | `TestEscapeSurface_LangEscapeCatalogOptIn/aot` |
| `lang-attr-context` | `TestEscapeSurface_LangCatalogTrusted/aot/attr_escaped` |
| `yield-default-var-escape` | `TestEscapeSurface_YieldDefaultVar/aot` |

## `@yield('name', $default)`

Unchanged: missing section expands to `{{ $default }}` so Strict contextual escape applies. Not XSS on v0.1.0 (literal left unexpanded).
