# Security

## Supported versions

| Version | Supported |
|---------|-----------|
| 0.1.x | yes (best-effort) |

Report issues via GitHub. Do not open public issues for unfixed zero-days without coordinated disclosure.

## Guarantees (honest)

Canvas **v0.1** is an experimental high-performance HTML template engine. Auto-escape on `{{ }}` / typed streams is the default XSS mitigation for string output.

It is **not** claimed to be:

- A full HTML sanitizer for untrusted markup
- Immune to every injection when using raw echo / custom funcs
- A security-audited drop-in for every Blade/Jinja workload

## Mitigations present

1. HTML escape on standard echo paths (`{{ $x }}`, `rt.WriteEscaped`, typed streams)
2. Raw echo (`{!! !!}` / `@json` misuse) is explicit — operator responsibility
3. No arbitrary code execution in templates (`@php` stripped / unsupported)
4. Pooled Writer does not retain data across `ReleaseWriter` after `Reset`
5. CI: gosec, govulncheck, Semgrep, Trivy on every push/PR

## Operator guidance

- Prefer escaped echo; avoid raw HTML unless the value is trusted
- Do not pass untrusted maps into custom `AddFunc` helpers without validation
- Keep Go and Canvas versions current; run `govulncheck ./...`

## Testing

```bash
go test ./... -count=1
go test -run '^TestGate' -v
gofmt -l .
go vet ./...
staticcheck ./...
govulncheck ./...
```
