<div align="center">

# Canvas

Independent HTML template engine for Go. Zero external dependencies.

[![Tests](https://github.com/zatrano/canvas/actions/workflows/tests.yml/badge.svg)](https://github.com/zatrano/canvas/actions/workflows/tests.yml)
[![Static Analysis](https://github.com/zatrano/canvas/actions/workflows/static-analysis.yml/badge.svg)](https://github.com/zatrano/canvas/actions/workflows/static-analysis.yml)
[![Coding Style](https://github.com/zatrano/canvas/actions/workflows/coding-style.yml/badge.svg)](https://github.com/zatrano/canvas/actions/workflows/coding-style.yml)
[![Security](https://github.com/zatrano/canvas/actions/workflows/security.yml/badge.svg)](https://github.com/zatrano/canvas/actions/workflows/security.yml)
[![Performance](https://github.com/zatrano/canvas/actions/workflows/performance.yml/badge.svg)](https://github.com/zatrano/canvas/actions/workflows/performance.yml)

[![gosec](https://img.shields.io/badge/gosec-enabled-E34C26?logo=go&logoColor=white)](https://github.com/zatrano/canvas/actions/workflows/security.yml)
[![govulncheck](https://img.shields.io/badge/govulncheck-enabled-00ADD8?logo=go&logoColor=white)](https://github.com/zatrano/canvas/actions/workflows/security.yml)
[![Semgrep](https://img.shields.io/badge/Semgrep-enabled-1B2A4E?logo=semgrep&logoColor=white)](https://github.com/zatrano/canvas/actions/workflows/security.yml)
[![Trivy](https://img.shields.io/badge/Trivy-enabled-1904DA?logo=aquasecurity&logoColor=white)](https://github.com/zatrano/canvas/actions/workflows/security.yml)

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Version](https://img.shields.io/github/v/tag/zatrano/canvas?filter=v*&sort=semver&label=version&color=blue)](https://github.com/zatrano/canvas/releases/tag/v0.2.0)
[![Latest Release](https://img.shields.io/github/v/release/zatrano/canvas?display_name=tag&label=latest&color=brightgreen)](https://github.com/zatrano/canvas/releases/latest)
[![Security Policy](https://img.shields.io/badge/Security-Policy-red?logo=github)](SECURITY.md)

[![Typed](https://img.shields.io/badge/Typed%20list-513%20ns-2ea44f?style=flat-square)](#benchmarks)
[![vs QT](https://img.shields.io/badge/vs%20quicktemplate-~3.5×-2ea44f?style=flat-square)](#benchmarks)
[![Dynamic](https://img.shields.io/badge/Dynamic%20RenderTo-1315%20ns-0366d6?style=flat-square)](#benchmarks)
[![Allocs](https://img.shields.io/badge/list--page%20RenderTo-0%20alloc-2ea44f?style=flat-square)](#benchmarks)
[![Deps](https://img.shields.io/badge/Dependencies-0-lightgrey?style=flat-square)](go.mod)

</div>

---

**Status: v0.2.0 (experimental).** Directive-based template syntax, native AOT hot path. On the list-page bench, Canvas typed beats [quicktemplate](https://github.com/valyala/quicktemplate) by ~3× — see [docs/performance.md](docs/performance.md). Read [SECURITY.md](SECURITY.md) before production exposure.

```text
Your app / framework
        ↓
      Canvas
        ↓
     []byte HTML
```

Canvas owns lex → AST → CompileFunc → pooled Writer. Routing, auth, CSRF, and HTTP response helpers stay in your code (or your framework).

## Install

```bash
go get github.com/zatrano/canvas@v0.2.0
```

## Quick start

```go
package main

import (
	"fmt"
	"log"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

func main() {
	eng := canvas.New("templates")
	html, err := eng.Render("hello", map[string]any{
		"title": "Hello",
		"items": []map[string]any{{"name": "a"}, {"name": "b"}},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(html)

	w := rt.AcquireWriter()
	if err := eng.RenderTo(w, "hello", map[string]any{
		"title": "Hello",
		"items": []map[string]any{{"name": "a"}},
	}); err != nil {
		rt.ReleaseWriter(w)
		log.Fatal(err)
	}
	// Consume Bytes() before ReleaseWriter — the slice aliases the pooled buffer.
	// Prefer w.CloneBytes() / w.String() if you need the bytes after release.
	out := append([]byte(nil), w.Bytes()...)
	rt.ReleaseWriter(w)
	_ = out
}
```

`templates/hello.html`:

```html
<h1>{{ $title }}</h1>
@foreach($items as $item)
<li>{{ $item.name }}</li>
@endforeach
```

Full guides: **[Documentation](docs/getting-started.md)**.

## Features

- Directive-based control flow (`@foreach`, `@if`, layouts, components, …)
- Native AOT: AST → specialized Go closures (no opcode VM on the hot path)
- `RenderTo` + Writer pool — **0 allocs/op** on AST-lowered list-page benches (see below)
- Typed streams (`rt.StreamListPage`) for absolute floor / `canvas gen`
- Fast HTML escape (scan-then-copy; **Strict contextual** by default — see [SECURITY.md](SECURITY.md))
- Zero external runtime deps (stdlib only)

**Not included in v0.2.0:** CSRF, session/auth, HTTP handlers, or `html/template` adapters — those live in the consumer (ZATRANO’s `packages/template` addon). Canvas is the template engine only.

### Strict escape (default)

Unsafe patterns such as `onclick="{{ $x }}"`, `<script>{{ $x }}</script>`, `srcdoc="{{ $x }}"`, whole-value `style="{{ $x }}"`, or tag/attribute-name interpolation **fail at compile time**. URL attributes filter dangerous schemes to `#unsafe`. Style declaration values (`style="color: {{ $c }}"`) use a CSS token filter. To keep pre-Strict behavior while migrating:

```go
eng.SetEscapeMode(rt.EscapeLegacy)
```

Full matrix and fix hints: [SECURITY.md](SECURITY.md).

## Documentation

| Doc | Topic |
|-----|--------|
| [Getting started](docs/getting-started.md) | Install → first render |
| [Concepts](docs/concepts.md) | Engine / AST / AOT / Writer |
| [Directives](docs/directives.md) | Directive catalog |
| [Performance](docs/performance.md) | Benchmarks + CI gates |
| [Security](SECURITY.md) | Policy + operator guidance |

## Benchmarks

Measured on **2026-10-01**, **v0.2.0**, Go **1.22+**, Windows/amd64. Absolute ns are **host-specific** and vary with load; treat CI gate **ratios** as authoritative (see [docs/performance.md](docs/performance.md)).

Same list-page HTML: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`. Numbers below are a **host-specific snapshot** (gate / bench medians on this machine).

```bash
go test -C bench -run 'Gate' -v
go test . -bench='Benchmark(CanvasRTTo|HTMLTemplateDataGet)' -benchmem -count=5
go test -C bench -bench='Benchmark(CanvasTyped|QuickTemplate)' -benchmem -count=5
```

### Gate baselines (this host, `TestGate*`)

| Metric | Value |
|--------|------:|
| CanvasTyped / quicktemplate | **~3.5×** (513 ns vs 1760 ns) |
| Canvas `RenderTo` / quicktemplate | **~1.4×** (1315 ns vs 1920 ns) |
| Canvas `RenderTo` / legacy `html/template`+`dataGet` | **≥20×** floor (observed ~25×–99×) |
| `RenderTo` allocs/op | **0** |

### Allocations — scope of “0 alloc”

| Workload | allocs/op | Notes |
|----------|----------:|-------|
| AST-lowered list-page `RenderTo` (map items) | **0** | Toy list; badge claim |
| Nested `@foreach` (10×10, AOT) | **0** | Still on CompileFunc path |
| Struct-field data (dynamic path) | **~51** | Reflect `FieldByName` |
| Layout + include + component + foreach | **~318** | Still on regex / `html/template` fallback (~56 µs host) — **AOT lowering of layout/include/component is out of scope for current work** |

### CI performance contract (v0.2.0)

| Gate | Floor |
|------|-------|
| Typed vs quicktemplate | ≥ 2.0× |
| Dynamic `RenderTo` vs quicktemplate | ≥ 1.05× |
| Dynamic vs `html/template`+dataGet | ≥ 20× |
| List-page `RenderTo` | ~0 allocs/op |

```bash
go test -C bench -run 'Gate' -v
```

## vs quicktemplate / html/template

| | Canvas | quicktemplate | html/template |
|--|--------|---------------|---------------|
| Role | directives + typed streams | typed codegen | stdlib |
| Runtime deps | none | bytebufferpool | stdlib |
| Dynamic `map` data | yes (AOT) | no (gen only) | yes |
| This-host typed list | **~513 ns #1 (~3.5× QT)** | ~1760 ns | — |
| This-host dynamic list | **~1315 ns**, 0 alloc (~1.4× QT) | ~1920 ns | map / dataGet ~25×–99× slower |

Canvas is **not** a quicktemplate fork. Full tables: [docs/performance.md](docs/performance.md).

## Quality gates (CI)

| Job | What |
|-----|------|
| [Tests](.github/workflows/tests.yml) | unit + race + coverage + build |
| [Performance](.github/workflows/performance.yml) | Gate vs QT + microbench |
| [Security](.github/workflows/security.yml) | gosec · govulncheck · Semgrep · Trivy |
| [Static Analysis](.github/workflows/static-analysis.yml) | staticcheck |
| [Coding Style](.github/workflows/coding-style.yml) | gofmt · vet · golangci-lint |

## Used by

[ZATRANO V3](https://github.com/zatrano) uses Canvas as its SSR foundation (`packages/template` addon).

## License

See [LICENSE](LICENSE).
