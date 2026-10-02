# Performance

Same HTML shape for every engine: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`.

Numbers are **host-specific**. Measured **2026-10-02**, **v0.2.0**, Go **1.22+**, Windows/amd64, CPU **i5-1135G7 @ 2.40GHz**, `count=7` median (unless noted).

```bash
go test -C bench -run 'Gate' -count=1 -v
go test -C bench -bench='Benchmark(CanvasTyped|QuickTemplate)' -benchmem -count=7
go test -bench='Benchmark(CanvasRTTo|HTMLTemplateDataGet)' -benchmem -count=7
```

## Head-to-head (ns/op, 2026-10-02)

| Rank | Engine | ns/op | allocs | Notes |
|-----:|--------|------:|-------:|-------|
| 1 | **Canvas typed (`rt.StreamListPage`)** | **735** | **0** | |
| 2 | **Canvas dynamic AOT (`RenderTo`)** | **1646** | **0** | AST-lowerable list page |
| 3 | quicktemplate (real library) | 2042 | 0 | Canvas typed **~2.8×** faster |
| 4 | `html/template` + legacy `dataGet` | 81113 | 572 | Canvas dynamic **~49×** faster (microbench median) |

## Gate baselines (this host, `TestGate*`)

| Metric | Value | Floor |
|--------|------:|------:|
| CanvasTyped / quicktemplate | **2.71×** (738 ns vs 1998 ns) | ≥ 2.0× |
| Canvas `RenderTo` / quicktemplate | **1.15×** (1582 ns vs 1812 ns) | ≥ 1.05× |
| Canvas `RenderTo` / legacy `html/template`+`dataGet` | **67.8×** (1559 ns vs 105667 ns) | ≥ 20× |
| List-page `RenderTo` allocs/op | **0** | ~0 |

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

## CI gates (v0.2.0)

| Gate | Floor |
|------|-------|
| Typed Canvas vs quicktemplate | ≥ **2.0×** |
| Dynamic `RenderTo` vs quicktemplate | ≥ **1.05×** |
| Dynamic vs `html/template`+dataGet | ≥ **20×** |
| List-page `RenderTo` | **~0 allocs/op** |

```bash
go test -C bench -run 'Gate' -v
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
