# Performance

Same HTML shape for every engine: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`.

Numbers are **host-specific**. Measured **2026-09-29**, **v0.1.0**, Go **1.25.0**, Windows/amd64, GOMAXPROCS=8, CPU **i5-1135G7 @ 2.40GHz**, `count=5` median (unless noted).

```bash
go test . -bench='Benchmark(CanvasRTTo|CanvasTyped|QuickTemplate|CeilingQTStyle|HTMLTemplateDataGet|Baseline_HTMLTemplate)' -benchmem -count=5
go test -run 'Gate' -v
```

## Head-to-head (ns/op)

| Rank | Engine | ns/op | allocs | Notes |
|-----:|--------|------:|-------:|-------|
| 1 | Hand ceiling (ideal typed loop) | ~484 | 0 | |
| 2 | **Canvas typed (`rt.StreamListPage`)** | **~502** | **0** | |
| 3 | **Canvas dynamic AOT (`RenderTo`)** | **~1147** | **0** | AST-lowerable list page |
| 4 | quicktemplate (real library) | ~1818 | 0 | Canvas typed **~3.6×** faster |
| 5 | `html/template` + `map[string]any` | ~78k | 362 | Idiomatic baseline (~**25×**) |
| 6 | `html/template` + struct | ~132k | 309 | Idiomatic (~**41×**) |
| 7 | `html/template` + legacy `dataGet` | ~160k–284k | 572 | **≥50×** claim applies **only** to this path |

## Realistic workloads (measurement-host medians)

| Scenario | ns/op | allocs |
|----------|------:|-------:|
| Nested foreach 10×10 (AOT) | ~3.9k | **0** |
| Struct slice fields (dynamic) | ~14k | **~51** |
| Layout + component + partial + foreach | ~56k | **~318** |

Layout/include/component still go through the regex → `html/template` path. Lowering those to AOT is a **separate** task; this table is the measurement only.

## Why Canvas wins vs quicktemplate

- Direct `[]byte` Writer — no `io.Writer` hop per chunk
- Single escape scan (no dual `QWriter` E/N wrapper)
- Fused list-page AOT for dynamic maps; typed stream for zero-map builds
- 0 allocs on AST-lowered list-page `RenderTo` with pooled Writer

## Writer buffer lifetime

`w.Bytes()` returns a slice that **aliases** the pooled buffer. After `rt.ReleaseWriter(w)`, that slice (and any `string` built without copying) is **invalid** — the next acquire may overwrite it.

```go
w := rt.AcquireWriter()
_ = eng.RenderTo(w, "users.index", data)
out := append([]byte(nil), w.Bytes()...) // copy if you need to keep it
rt.ReleaseWriter(w)
resp.Write(out)
```

`defer rt.ReleaseWriter(w)` is fine if you finish using `Bytes()` **before the function returns** (defer runs on return). Do not store the slice for later use after release.

## CI gates (v0.1.0)

| Gate | Floor |
|------|-------|
| Typed Canvas vs quicktemplate | ≥ **2.0×** |
| Dynamic `RenderTo` vs quicktemplate | ≥ **1.05×** |
| Dynamic vs `html/template`+dataGet | ≥ **20×** |
| List-page `RenderTo` | **~0 allocs/op** |

```bash
go test -run 'Gate' -v
```

## Ship max speed

```go
eng := canvas.New("templates") // cache ON

w := rt.AcquireWriter()
_ = eng.RenderTo(w, "users.index", data)
out := append([]byte(nil), w.Bytes()...)
rt.ReleaseWriter(w)
resp.Write(out)
```

Typed absolute floor:

```go
rt.StreamListPage(w, title, items)
```
