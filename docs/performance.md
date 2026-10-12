# Performance

Same HTML shape for every engine: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`.

Numbers are **host-specific**. Measured **2026-10-12**, **v0.2.0**, Go **1.26.9**, Windows/amd64, CPU **i5-6200U @ 2.30GHz** (2 cores / 4 threads), `GOMAXPROCS=4`, power plan Dengeli. Microbench figures are the median of `count=7`, `-benchtime=1s`. Gate ratios are one `count=1` run.

```bash
go test -C bench -run 'Gate' -count=1 -v
go test -C bench -bench='Benchmark(CanvasTyped|QuickTemplate)' -benchmem -count=7
go test -bench='Benchmark(CanvasRTTo|HTMLTemplateDataGet)' -benchmem -count=7
```

## Head-to-head (ns/op, 2026-10-12)

Microbench medians. Typed and quicktemplate shared one process (quicktemplate ran first). `RenderTo` and `html/template` shared another.

| Rank | Engine | ns/op | allocs | Notes |
|-----:|--------|------:|-------:|-------|
| 1 | **Canvas typed (`rt.StreamListPage`)** | **926** | **0** | |
| 2 | **Canvas dynamic AOT (`RenderTo`)** | **2811** | **0** | AST-lowerable list page |
| 3 | quicktemplate (real library) | 4467 | 0 | separate from the gate below |
| 4 | `html/template` + legacy `dataGet` | 112368 | 572 | `RenderTo` is **40×** this median |

## Gate baselines (this host, `TestGate*`)

| Metric | Value | Floor |
|--------|------:|------:|
| CanvasTyped / quicktemplate | **3.69×** (1061 ns vs 3913 ns) | ≥ 2.0× |
| Canvas `RenderTo` / quicktemplate | **0.80×** (4865 ns vs 3870 ns) | ≥ 1.05× |
| Canvas `RenderTo` / legacy `html/template`+`dataGet` | **56.8×** (2714 ns vs 154286 ns) | ≥ 20× |
| List-page `RenderTo` allocs/op | **0** | ~0 |

The dynamic `RenderTo` / quicktemplate ratio on this host is under the CI floor. The floor is the Linux regression contract and is unchanged.

## Realistic workloads (measurement-host medians)

| Scenario | ns/op | allocs |
|----------|------:|-------:|
| Nested foreach 10×10 (AOT) | 4721 | **0** |
| Struct slice fields (dynamic) | 15036 | **51** |
| Layout + component + partial + foreach | 61741 | **318** |

`@component` and `<x-*>` with a slot, `$attributes`, `@props` defaults, `@aware` inheritance, and literal props (including `<` and quotes) are inlined into Canvas syntax. Literals are escaped for the echo's HTML context and the page is rendered by AOT. The row above is the earlier html/template measurement.

## What this host showed

Typed `StreamListPage` beat quicktemplate (**3.69×** in the gate, **926 ns** vs **4467 ns** in the microbench). Dynamic `RenderTo` stayed at **0 allocs** and beat `html/template`+`dataGet` (**56.8×** in the gate). The same dynamic path did **not** beat quicktemplate in the gate (**0.80×**).

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
