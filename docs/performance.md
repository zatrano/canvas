# Performance

Same HTML shape for every engine: `<h1>{{title}}</h1>` + 50× `<li>{{name}}</li>`.

Measured **2026-09-29**, **v0.1.0**, Go **1.25.0**, Windows/amd64, GOMAXPROCS=8, CPU **i5-1135G7 @ 2.40GHz**, `count=5` median.

```bash
go test . -bench='Benchmark(CanvasRTTo|CanvasTyped|QuickTemplate|CeilingQTStyle|HTMLTemplateDataGet)$' -benchmem -count=5
go test -run 'Gate' -v
```

## Head-to-head (ns/op, 0 allocs unless noted)

| Rank | Engine | ns/op | allocs | vs Canvas typed |
|-----:|--------|------:|-------:|----------------:|
| 1 | Hand ceiling (ideal typed loop) | ~484 | 0 | ~0.96× |
| 2 | **Canvas typed (`rt.StreamListPage`)** | **~502** | **0** | **1×** |
| 3 | **Canvas dynamic AOT (`RenderTo`)** | **~1147** | **0** | — |
| 4 | quicktemplate (real library) | ~1818 | 0 | Canvas typed **~3.6×** faster |
| 5 | `html/template` + `dataGet` | ~160k+ | 572 | Canvas dynamic **≥50×** faster |

## Why Canvas wins vs quicktemplate

- Direct `[]byte` Writer — no `io.Writer` hop per chunk
- Single escape scan (no dual `QWriter` E/N wrapper)
- Fused list-page AOT for dynamic maps; typed stream for zero-map builds
- 0 allocs on `RenderTo` with pooled Writer

## CI gates (v0.1.0)

| Gate | Floor |
|------|-------|
| Typed Canvas vs quicktemplate | ≥ **2.0×** |
| Dynamic `RenderTo` vs quicktemplate | ≥ **1.05×** |
| Dynamic vs `html/template`+dataGet | ≥ **20×** |
| `RenderTo` | **~0 allocs/op** |

```bash
go test -run 'Gate' -v
```

## Ship max speed

```go
eng := canvas.New("templates") // cache ON

w := rt.AcquireWriter()
defer rt.ReleaseWriter(w)
_ = eng.RenderTo(w, "users.index", data)
resp.Write(w.Bytes())
```

Typed absolute floor:

```go
rt.StreamListPage(w, title, items)
```
