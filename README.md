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
[![Version](https://img.shields.io/github/v/tag/zatrano/canvas?filter=v*&sort=semver&label=version&color=blue)](https://github.com/zatrano/canvas/releases/tag/v1.0.1)
[![Latest Release](https://img.shields.io/github/v/release/zatrano/canvas?display_name=tag&label=latest&color=brightgreen)](https://github.com/zatrano/canvas/releases/latest)
[![Security Policy](https://img.shields.io/badge/Security-Policy-red?logo=github)](SECURITY.md)

[![Deps](https://img.shields.io/badge/Dependencies-0-lightgrey?style=flat-square)](go.mod)

</div>

---

**Status: v1.0.1 (stable).** The public API follows semantic versioning: no breaking changes within 1.x. Directive-based template syntax, native AOT hot path. The list-page bench against [quicktemplate](https://github.com/valyala/quicktemplate) is in [Benchmarks](#benchmarks). Typed list is ahead. Dynamic `RenderTo` is not. Read [SECURITY.md](SECURITY.md) before production exposure.

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
go get github.com/zatrano/canvas@v1.0.1
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

**Not included in v1.0.1:** CSRF, session/auth, or HTTP response helpers — those live in the consuming application. Import Canvas and call `canvas.New` / `Render` directly.

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

Current release is **v1.0.1**. The numbers below were measured on **2026-10-12** while the tree was **v0.2.0**, Go **1.26.9**, Windows/amd64, Intel Core i5-6200U @ 2.30GHz (2 cores / 4 threads), `GOMAXPROCS=4`, Balanced power plan. They were not re-measured for the v1.0.x tags; v1.0.0 is the same code as v0.2.1, and v1.0.1 changes only nested and reflective `@foreach` scoping, not the list-page paths below. Absolute ns are from this host.

Same list-page HTML: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`. A gate ratio is one `go test -count=1` run; each side is an internal median of 7. A microbench ns is the median of `-count=7`, `-benchtime=1s`. Those two runs are not the same number. The typed microbench ratio depends on process order.

```bash
go test -C bench -run 'Gate' -v
go test -C bench -bench='Benchmark(CanvasTyped|QuickTemplate)' -benchmem -count=7
```

| Workload | Canvas | quicktemplate | html/template | Ratio |
|---|---:|---:|---:|---:|
| Typed list, gate | 1061 ns | 3913 ns | — | 3.69× |
| Typed list, microbench | 926 ns, 0 alloc | 4467 ns | — | 4.82× |
| Dynamic `RenderTo`, gate | 4865 ns | 3870 ns | — | 0.80× |
| Dynamic `RenderTo`, gate vs `html/template` | 2714 ns | — | 154286 ns | 56.8× |
| Dynamic `RenderTo`, microbench | 2811 ns, 0 alloc | — | 112368 ns, 572 alloc | 40× |

The dynamic `RenderTo` gate is under quicktemplate on this host (0.80×). The CI floor for that comparison is still 1.05×. The typed-list gate is above its 2.0× floor.

Zero allocations is the toy list page only.

| Workload | allocs/op | Time |
|---|---:|---:|
| AST-lowered list-page `RenderTo` (map items) | 0 | 2811 ns |
| Nested `@foreach` (10×10, AOT) | 0 | 4721 ns |
| Struct-field data (dynamic path) | 51 | 15036 ns |
| Layout + include + component + foreach | 318 | 61741 ns |

The last row is still on the regex / `html/template` fallback. CI floors: [docs/performance.md](docs/performance.md).

| Gate | Floor |
|---|---:|
| Typed vs quicktemplate | ≥ 2.0× |
| Dynamic `RenderTo` vs quicktemplate | ≥ 1.05× |
| Dynamic vs `html/template` | ≥ 20× |

Canvas is not a quicktemplate fork. quicktemplate has no dynamic `map` path. Canvas does.

## Quality gates (CI)

| Job | What |
|-----|------|
| [Tests](.github/workflows/tests.yml) | unit + race + coverage + build |
| [Performance](.github/workflows/performance.yml) | Gate vs QT + microbench |
| [Security](.github/workflows/security.yml) | gosec · govulncheck · Semgrep · Trivy |
| [Static Analysis](.github/workflows/static-analysis.yml) | staticcheck |
| [Coding Style](.github/workflows/coding-style.yml) | gofmt · vet · golangci-lint |

## Used by

Canvas is a standalone Go library. Any application can import `github.com/zatrano/canvas` and render templates without a framework.

## License

See [LICENSE](LICENSE).
