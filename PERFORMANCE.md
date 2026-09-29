# Canvas performance — world’s-fastest path

See **[docs/performance.md](docs/performance.md)** for the full tables, methodology, and CI gates.

Quick verdict (v0.1.0, same list-page HTML):

| Engine | ns/op | vs |
|--------|------:|----|
| **Canvas typed** | **~502** | **~3.6×** faster than quicktemplate |
| **Canvas dynamic `RenderTo`** | **~1147** | beats QT; **≥50×** vs legacy dataGet |
| quicktemplate | ~1818 | — |
| `html/template`+dataGet | ~160k+ | — |
