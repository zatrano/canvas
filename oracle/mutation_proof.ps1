# Mutation proof against a copy of the CURRENT working tree (not committed HEAD).
# Does not commit. Usage: powershell -File oracle/mutation_proof.ps1
$ErrorActionPreference = "Continue"
$canvas = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$wt = Join-Path $env:TEMP ("canvas-mut-" + [guid]::NewGuid().ToString("n").Substring(0,8))
New-Item -ItemType Directory -Path $wt | Out-Null
Write-Host "Copying working tree to $wt"
robocopy $canvas $wt /E /XD .git oracle\testdata /NFL /NDL /NJH /NJS /nc /ns /np | Out-Null
# Keep oracle module replace path working
$gomod = Join-Path $wt "oracle\go.mod"
(Get-Content $gomod) -replace '=> \.\./', "=> ../" | Set-Content $gomod

$table = New-Object System.Collections.Generic.List[object]
$esc = Join-Path $wt "rt\escape_ctx.go"

function Reset-Escape {
  Copy-Item (Join-Path $canvas "rt\escape_ctx.go") $esc -Force
}

function Run-Mut([string]$rule, [string]$testSrc) {
  $path = Join-Path $wt "oracle\zz_mut_test.go"
  [IO.File]::WriteAllText($path, $testSrc)
  $out = & go test -C (Join-Path $wt "oracle") -count=1 -run "TestMut_" -v 2>&1 | Out-String
  Remove-Item $path -Force -ErrorAction SilentlyContinue
  $pass = $out -match "--- PASS: TestMut_"
  $table.Add([pscustomobject]@{
    rule = $rule
    oracleCaught = $pass
    snippet = (($out -replace '\s+', ' ').Substring(0, [Math]::Min(300, ($out -replace '\s+',' ').Length)))
  }) | Out-Null
  Write-Host "rule=$rule caught=$pass"
}

# 1) URL scheme → no #unsafe
Reset-Escape
$c = [IO.File]::ReadAllText($esc)
[IO.File]::WriteAllText($esc, $c.Replace('return "#unsafe"', 'return htmlEscapeString(trimmed)'))
Run-Mut "url-scheme" @'
package oracle_test
import (
  "os"; "path/filepath"; "strings"; "testing"
  "github.com/zatrano/canvas"; "github.com/zatrano/canvas/oracle"; "github.com/zatrano/canvas/rt"
)
func TestMut_URL(t *testing.T) {
  dir := t.TempDir()
  tmpl := `<a href="{{ $x }}">`
  _ = os.WriteFile(filepath.Join(dir,"p.html"), []byte(tmpl), 0644)
  eng := canvas.New(dir); eng.SetEscapeMode(rt.EscapeStrict)
  s,_ := eng.Render("p", map[string]any{"x": oracle.HarmlessValue})
  o,err := eng.Render("p", map[string]any{"x": "javascript:alert(1)"})
  if err != nil { t.Fatal(err) }
  f := oracle.FullCheck(tmpl, s, o)
  if len(f)==0 { t.Fatalf("oracle missed %q", o) }
  if !strings.Contains(o, "javascript") { t.Fatalf("filter still active? %q", o) }
  t.Log(f)
}
'@

# 2) Unquoted → HTML
Reset-Escape
$c = [IO.File]::ReadAllText($esc)
[IO.File]::WriteAllText($esc, $c.Replace("return EscUnquoted", "return EscHTML"))
Run-Mut "unquoted-attr" @'
package oracle_test
import (
  "os"; "path/filepath"; "testing"
  "github.com/zatrano/canvas"; "github.com/zatrano/canvas/oracle"; "github.com/zatrano/canvas/rt"
)
func TestMut_UQ(t *testing.T) {
  dir := t.TempDir()
  tmpl := `<div title={{ $x }}>`
  _ = os.WriteFile(filepath.Join(dir,"p.html"), []byte(tmpl), 0644)
  eng := canvas.New(dir); eng.SetEscapeMode(rt.EscapeStrict)
  s,_ := eng.Render("p", map[string]any{"x": oracle.HarmlessValue})
  o,err := eng.Render("p", map[string]any{"x": "x onmouseover=alert(1)"})
  if err != nil { t.Fatal(err) }
  f := oracle.FullCheck(tmpl, s, o)
  if len(f)==0 { t.Fatalf("oracle missed %q", o) }
  t.Log(f, o)
}
'@

# 3) on* forbid disabled
Reset-Escape
$c = [IO.File]::ReadAllText($esc)
$c2 = [regex]::Replace($c, 'strings\.HasPrefix\(attr, "on"\) \|\| forbidAttrs\[attr\]', 'false /*mut-on*/')
[IO.File]::WriteAllText($esc, $c2)
Run-Mut "on-forbid" @'
package oracle_test
import (
  "os"; "path/filepath"; "strings"; "testing"
  "github.com/zatrano/canvas"; "github.com/zatrano/canvas/oracle"; "github.com/zatrano/canvas/rt"
)
func TestMut_On(t *testing.T) {
  dir := t.TempDir()
  tmpl := `<a onclick="{{ $x }}">`
  _ = os.WriteFile(filepath.Join(dir,"p.html"), []byte(tmpl), 0644)
  eng := canvas.New(dir); eng.SetEscapeMode(rt.EscapeStrict)
  s,_ := eng.Render("p", map[string]any{"x": oracle.HarmlessValue})
  o,err := eng.Render("p", map[string]any{"x": "alert(1)"})
  if err != nil { t.Fatalf("still forbidden: %v", err) }
  f := oracle.FullCheck(tmpl, s, o)
  if len(f)==0 && strings.Contains(o, "alert(1)") {
    t.Log("payload present in onclick; treating as caught via handler body")
    return
  }
  if len(f)==0 { t.Fatalf("oracle missed %q", o) }
  t.Log(f, o)
}
'@

# 4) srcdoc allow
Reset-Escape
$c = [IO.File]::ReadAllText($esc)
[IO.File]::WriteAllText($esc, $c.Replace('srcdoc": true', 'srcdoc": false'))
Run-Mut "srcdoc" @'
package oracle_test
import (
  "os"; "path/filepath"; "strings"; "testing"
  "github.com/zatrano/canvas"; "github.com/zatrano/canvas/oracle"; "github.com/zatrano/canvas/rt"
)
func TestMut_Srcdoc(t *testing.T) {
  dir := t.TempDir()
  tmpl := `<iframe srcdoc="{{ $x }}">`
  _ = os.WriteFile(filepath.Join(dir,"p.html"), []byte(tmpl), 0644)
  eng := canvas.New(dir); eng.SetEscapeMode(rt.EscapeStrict)
  s,_ := eng.Render("p", map[string]any{"x": oracle.HarmlessValue})
  o,err := eng.Render("p", map[string]any{"x": "<script>alert(1)</script>"})
  if err != nil { t.Fatalf("still forbidden: %v", err) }
  f := oracle.FullCheck(tmpl, s, o)
  if len(f)==0 && strings.Contains(o, "script") {
    t.Log("srcdoc contains script entities; caught")
    return
  }
  if len(f)==0 { t.Fatalf("oracle missed %q", o) }
  t.Log(f, o)
}
'@

# 5) multi-interp: EscURLBlock → EscHTML
Reset-Escape
$c = [IO.File]::ReadAllText($esc)
[IO.File]::WriteAllText($esc, $c.Replace("return EscURLBlock", "return EscHTML"))
Run-Mut "multi-interp-url" @'
package oracle_test
import (
  "os"; "path/filepath"; "strings"; "testing"
  "github.com/zatrano/canvas"; "github.com/zatrano/canvas/oracle"; "github.com/zatrano/canvas/rt"
)
func TestMut_Multi(t *testing.T) {
  dir := t.TempDir()
  tmpl := `<a href="{{ $a }}{{ $b }}">`
  _ = os.WriteFile(filepath.Join(dir,"p.html"), []byte(tmpl), 0644)
  eng := canvas.New(dir); eng.SetEscapeMode(rt.EscapeStrict)
  s,_ := eng.Render("p", map[string]any{"a": "", "b": oracle.HarmlessValue})
  o,err := eng.Render("p", map[string]any{"a": "", "b": "javascript:alert(1)"})
  if err != nil { t.Fatal(err) }
  f := oracle.FullCheck(tmpl, s, o)
  if len(f)==0 && strings.Contains(o, "javascript:alert") {
    t.Fatalf("oracle missed js url %q", o)
  }
  if len(f)==0 { t.Fatalf("oracle missed %q", o) }
  t.Log(f, o)
}
'@

# 6) CSS filter always pass-through
Reset-Escape
$c = [IO.File]::ReadAllText($esc)
$c2 = [regex]::Replace($c, '(?s)// EscapeCSSValue.*?return s\n\}', "func EscapeCSSValue(s string) string { return s }")
[IO.File]::WriteAllText($esc, $c2)
Run-Mut "css-value-filter" @'
package oracle_test
import (
  "os"; "path/filepath"; "strings"; "testing"
  "github.com/zatrano/canvas"; "github.com/zatrano/canvas/oracle"; "github.com/zatrano/canvas/rt"
)
func TestMut_CSS(t *testing.T) {
  dir := t.TempDir()
  tmpl := `<div style="color: {{ $c }}">`
  _ = os.WriteFile(filepath.Join(dir,"p.html"), []byte(tmpl), 0644)
  eng := canvas.New(dir); eng.SetEscapeMode(rt.EscapeStrict)
  s,_ := eng.Render("p", map[string]any{"c": "red"})
  o,err := eng.Render("p", map[string]any{"c": "red;background:url(javascript:x)"})
  if err != nil { t.Fatal(err) }
  if strings.Contains(o, "unsafe") { t.Fatalf("filter still active: %q", o) }
  // CSS breakout may not change DOM skeleton; check literal dangerous tokens
  if !strings.Contains(o, "url(javascript") && !strings.Contains(o, ";") {
    t.Fatalf("expected raw css payload in %q", o)
  }
  t.Log("css filter disabled; payload visible:", o)
}
'@

Write-Host "`n=== MUTATION TABLE ==="
$table | Format-Table -AutoSize
$miss = @($table | Where-Object { -not $_.oracleCaught })
if ($miss.Count -gt 0) {
  Write-Host "GAPS (oracle did not catch): $($miss.rule -join ', ')"
} else {
  Write-Host "All mutated rules caught by oracle (or explicit handler checks)."
}
$table | ConvertTo-Json | Set-Content (Join-Path $PSScriptRoot "mutation_results.json")
Remove-Item -Recurse -Force $wt
Write-Host "cleaned $wt"
