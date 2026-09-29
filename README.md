# Canvas

Independent HTML template engine for Go. Zero framework dependencies.

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
[![Version](https://img.shields.io/github/v/tag/zatrano/canvas?filter=v*&sort=semver&label=version&color=blue)](https://github.com/zatrano/canvas/releases/tag/v0.1.0)
[![Latest Release](https://img.shields.io/github/v/release/zatrano/canvas?display_name=tag&label=latest&color=brightgreen)](https://github.com/zatrano/canvas/releases/latest)
[![Security Policy](https://img.shields.io/badge/Security-Policy-red?logo=github)](SECURITY.md)

[![Typed](https://img.shields.io/badge/Typed%20list-502%20ns-2ea44f?style=flat-square)](#benchmarks)
[![vs QT](https://img.shields.io/badge/vs%20quicktemplate-~3.6×-2ea44f?style=flat-square)](#benchmarks)
[![Dynamic](https://img.shields.io/badge/Dynamic%20RenderTo-1147%20ns-0366d6?style=flat-square)](#benchmarks)
[![Allocs](https://img.shields.io/badge/RenderTo-0%20alloc-2ea44f?style=flat-square)](#benchmarks)
[![Deps](https://img.shields.io/badge/Runtime%20deps-0-lightgrey?style=flat-square)](go.mod)

---

**Status: v0.1.0 (experimental).** Blade-class syntax, native AOT hot path. On the list-page bench, Canvas typed beats [quicktemplate](https://github.com/valyala/quicktemplate) by ~3× — see [docs/performance.md](docs/performance.md). Read [SECURITY.md](SECURITY.md) before production exposure.

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
go get github.com/zatrano/canvas@v0.1.0
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
	defer rt.ReleaseWriter(w)
	_ = eng.RenderTo(w, "hello", map[string]any{
		"title": "Hello",
		"items": []map[string]any{{"name": "a"}},
	})
	_ = w.Bytes()
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

- Blade-class directives (`@foreach`, `@if`, layouts, components, …)
- Native AOT: AST → specialized Go closures (no opcode VM on the hot path)
- `RenderTo` + Writer pool — **0 allocs/op** on the list-page bench
- Typed streams (`rt.StreamListPage`) for absolute floor / `canvas gen`
- Fast HTML escape (scan-then-copy)
- Zero external runtime deps (stdlib only)

**Not a framework.** CSRF / auth / `http.Template` wiring lives in the consumer (ZATRANO’s `packages/template` addon).

## Documentation

| Doc | Topic |
|-----|--------|
| [Getting started](docs/getting-started.md) | Install → first render |
| [Concepts](docs/concepts.md) | Engine / AST / AOT / Writer |
| [Directives](docs/directives.md) | Blade-class catalog |
| [Performance](docs/performance.md) | Benchmarks + CI gates |
| [Security](SECURITY.md) | Policy + operator guidance |

## Benchmarks

Measured on **2026-09-29**, **v0.1.0**, Go **1.25.0**, Windows/amd64, GOMAXPROCS=8, CPU **i5-1135G7 @ 2.40GHz**. Same HTML: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`. Absolute ns varies by host; **Canvas must beat quicktemplate** (CI gates).

```bash
go test . -bench='Benchmark(CanvasRTTo|CanvasTyped|QuickTemplate|HTMLTemplateDataGet)$' -benchmem -count=5
go test -run 'Gate' -v
```

### List page — median ns/op

| Rank | Engine | ns/op | allocs |
|-----:|--------|------:|-------:|
| 1 | **Canvas typed** | **~502** | **0** |
| 2 | **Canvas dynamic `RenderTo`** | **~1147** | **0** |
| 3 | quicktemplate | ~1818 | 0 |
| 4 | `html/template` + `dataGet` | ~160k+ | 572 |

Ratios (this host): typed **~3.6×** vs quicktemplate · dynamic **~1.6×** vs quicktemplate · dynamic **≥50×** vs legacy dataGet.

### CI performance contract (v0.1.0)

| Gate | Floor |
|------|-------|
| Typed vs quicktemplate | ≥ 2.0× |
| Dynamic `RenderTo` vs quicktemplate | ≥ 1.05× |
| Dynamic vs `html/template`+dataGet | ≥ 20× |
| `RenderTo` | ~0 allocs/op |

```bash
go test -run 'Gate' -v
```

## vs quicktemplate / html/template

| | Canvas | quicktemplate | html/template |
|--|--------|---------------|---------------|
| Role | Blade-class + typed streams | typed codegen | stdlib |
| Runtime deps | none | bytebufferpool | stdlib |
| Dynamic `map` data | yes (AOT) | no (gen only) | yes |
| This-host typed list | **~502 ns #1** | ~1818 ns | — |
| This-host dynamic list | **~1147 ns**, 0 alloc | — | ~160k+, 572 alloc |

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
