# Concepts

## Engine

`canvas.New(directory)` loads `.html` files from a root. Cache is **on** by default.

| Method | Role |
|--------|------|
| `Render(name, data)` | string result (1 alloc for the string) |
| `RenderTo(w, name, data)` | write into pooled `*rt.Writer` (0 alloc execute) |
| `Share` / `AddFunc` / `Directive` | cross-view data and helpers |
| layouts / components | `@extends`, `@section`, `<x-*>` |

Data model for the dynamic path is `map[string]any` (Blade-class). Typed path uses structs / string slices via `rt.Stream*`.

## Pipeline

```text
source
  → lex (tokenizer)
  → ast (Nest blocks + leaf nodes)
  → rt.CompileFunc → RenderFunc (specialized Go closures)
  → Writer pool
```

Templates that `CanASTLower()` skip `html/template` entirely on the hot path.
Complex leftover directives still fall back to the regex → `html/template` path.

## Writer

`rt.Writer` is a pooled `[]byte` buffer with an optional range frame (no per-render `Ctx` alloc for top-level foreach).

```go
w := rt.AcquireWriter()
defer rt.ReleaseWriter(w)
```

## Packages

| Path | Role |
|------|------|
| `canvas` | Engine API |
| `canvas/lex` | Tokenizer |
| `canvas/ast` | Parser |
| `canvas/rt` | AOT + Writer + typed streams |
| `canvas/gen` | Emit typed Go from AST |
