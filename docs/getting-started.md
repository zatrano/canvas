# Getting started

## Install

```bash
go get github.com/zatrano/canvas@v0.1.0
```

Requires Go 1.22+.

## First template

`templates/hello.html`:

```html
<h1>{{ $title }}</h1>
@foreach($items as $item)
<li>{{ $item.name }}</li>
@endforeach
```

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
		"title": "Canvas",
		"items": []map[string]any{
			{"name": "fast"},
			{"name": "typed"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(html)

	// Zero-alloc hot path (reuse pooled writer):
	w := rt.AcquireWriter()
	defer rt.ReleaseWriter(w)
	if err := eng.RenderTo(w, "hello", map[string]any{
		"title": "Canvas",
		"items": []map[string]any{{"name": "a"}},
	}); err != nil {
		log.Fatal(err)
	}
	_ = w.Bytes()
}
```

## Typed stream (absolute floor)

When you know the shape at compile time:

```go
w := rt.AcquireWriter()
defer rt.ReleaseWriter(w)
rt.StreamListPage(w, "Hello", []rt.ListItem{{Name: "a"}, {Name: "b"}})
```

Or emit Go with `canvas/gen` from an AST document.

## Next

- [Concepts](concepts.md) — Engine / AST / AOT / Writer
- [Directives](directives.md) — Blade-class catalog
- [Performance](performance.md) — Benchmarks + gates
