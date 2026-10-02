# Oracle (XSS injection checker)

Separate module (`go.mod`) so `golang.org/x/net/html` stays out of the root module.

## Tests

```bash
go test -C oracle -count=1 ./...
```

Combinatorial Strict scan (slow):

```bash
go test -C oracle -count=1 -run TestOracle_CombinatorialStrict -timeout 10m
```

## Mutation coverage table

`mutation_results.json` is **generated**, not hand-edited. Regenerate after changing escape rules:

```bash
go test -C oracle -tags=mutation -run TestMutationGenerateResults -count=1 -timeout 20m
```

Each rule mutates `rt/escape_ctx.go` in a temp copy, runs candidate tests, and records the first failing test name. A mutation that fails no test is a coverage gap (the generator fails the rule).

Legacy PowerShell proof (oracle-caught focus): `powershell -File oracle/mutation_proof.ps1`
