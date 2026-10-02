# Canvas performance

Full tables, methodology, and CI gates: **[docs/performance.md](docs/performance.md)**.

**0 alloc** applies to AST-lowered list-page / nested-foreach `RenderTo`. Layout+component and struct-data paths allocate — see the docs.
