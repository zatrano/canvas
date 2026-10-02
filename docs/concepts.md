# Concepts

## Engine

`canvas.New(directory)` loads `.html` files from a root. Cache is **on** by default.

| Method | Role |
|--------|------|
| `Render(name, data)` | string result (1 alloc for the string) |
| `RenderTo(w, name, data)` | write into pooled `*rt.Writer` (0 alloc on AST-lowered list pages) |
| `Share` / `AddFunc` / `Directive` | cross-view data and helpers — developer trust (see SECURITY.md) |
| layouts / components | `@extends`, `@section`, `<x-*>` |

Data model for the dynamic path is `map[string]any`. Typed path uses structs / string slices via `rt.Stream*`.

## Pipeline

```text
source
  → lex (tokenizer)
  → ast (Nest blocks + leaf nodes)
  → rt.CompileFunc → RenderFunc (specialized Go closures)
  → Writer pool
```

Templates that `CanASTLower()` skip `html/template` entirely on the hot path.
Layout / include / component paths still fall back to the regex → `html/template` path (~318 allocs on a realistic layout+component bench).

## Writer

`rt.Writer` is a pooled `[]byte` buffer with an optional range frame (no per-render `Ctx` alloc for top-level foreach).

```go
w := rt.AcquireWriter()
_ = eng.RenderTo(w, name, data)
out := append([]byte(nil), w.Bytes()...) // copy if retaining past release
rt.ReleaseWriter(w)
```

`Bytes()` aliases the pool buffer and is **invalid after `ReleaseWriter`**.

## Packages

| Path | Role |
|------|------|
| `canvas` | Engine API |
| `canvas/lex` | Tokenizer |
| `canvas/ast` | Parser |
| `canvas/rt` | AOT + Writer + typed streams |
| `canvas/gen` | Emit typed Go from AST |
