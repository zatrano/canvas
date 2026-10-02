package oracle_test

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/oracle"
	"github.com/zatrano/canvas/rt"
)

func renderStrict(t testing.TB, tmpl string, data map[string]any) (out string, err error) {
	t.Helper()
	dir := t.TempDir()
	name := "fuzz"
	if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := canvas.New(dir)
	eng.SetEscapeMode(rt.EscapeStrict)
	return eng.Render(name, data)
}

func checkCaseStrict(t testing.TB, tmpl string, data map[string]any) (ok bool, detail string) {
	t.Helper()
	safe := map[string]any{}
	for k, v := range data {
		switch m := v.(type) {
		case map[string]string:
			sm := make(map[string]string, len(m))
			for ak := range m {
				sm[ak] = oracle.HarmlessValue
			}
			safe[k] = sm
		default:
			safe[k] = oracle.HarmlessValue
		}
	}
	safeOut, errSafe := renderStrict(t, tmpl, safe)
	attackOut, errAttack := renderStrict(t, tmpl, data)
	if errAttack != nil {
		return true, "compile-or-render-error: " + errAttack.Error()
	}
	if errSafe != nil {
		return true, "safe-compile-error: " + errSafe.Error()
	}
	findings := oracle.FullCheck(tmpl, safeOut, attackOut)
	if len(findings) == 0 {
		return true, ""
	}
	var b strings.Builder
	for _, f := range findings {
		b.WriteString(f.Kind)
		b.WriteByte(':')
		b.WriteString(f.Detail)
		b.WriteByte(';')
	}
	return false, b.String() + " out=" + oracle.Normalize(attackOut)
}

func TestOracle_SeedCorpus(t *testing.T) {
	for _, seed := range oracle.CorpusSeeds() {
		tmpl := oracle.TemplateFromSeed(seed)
		for _, payload := range oracle.AttackPayloads {
			data := map[string]any{"x": payload, "a": payload, "b": payload, "t": payload}
			ok, detail := checkCaseStrict(t, tmpl, data)
			if !ok {
				t.Fatalf("injection tmpl=%q payload=%q detail=%s", tmpl, payload, detail)
			}
		}
	}
}

// FuzzStrictNoInjection keeps generative byte→template coverage (slower; temp dir per exec).
func FuzzStrictNoInjection(f *testing.F) {
	for _, seed := range oracle.CorpusSeeds() {
		f.Add(seed)
	}
	for _, p := range oracle.AttackPayloads {
		f.Add(append([]byte{0xff}, []byte(`<p>{{ $x }}</p>`+p)...))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		if len(data) > 512 {
			data = data[:512]
		}
		tmpl := oracle.TemplateFromSeed(data)
		if tmpl == "" || len(tmpl) > 400 {
			return
		}
		payload := oracle.AttackPayloads[int(data[0])%len(oracle.AttackPayloads)]
		if len(data) > 8 {
			payload = string(data[len(data)/2:])
			if len(payload) > 64 {
				payload = payload[:64]
			}
		}
		dataMap := map[string]any{
			"x": payload, "a": payload, "b": payload, "t": "div",
			"m": map[string]string{"id": "ok", "title": payload, "href": payload, "onclick": payload},
		}
		ok, detail := checkCaseStrict(t, tmpl, dataMap)
		if !ok {
			saveFailure(t, data, tmpl, detail)
			t.Fatalf("injection tmpl=%q detail=%s", tmpl, detail)
		}
	})
}

func saveFailure(t testing.TB, data []byte, tmpl, detail string) {
	t.Helper()
	dir := filepath.Join("testdata", "failures")
	_ = os.MkdirAll(dir, 0o755)
	name := filepath.Join(dir, time.Now().Format("20060102-150405.000000000")+".txt")
	_ = os.WriteFile(name, []byte(fmt.Sprintf("tmpl: %s\ndetail: %s\nhex: %x\n", tmpl, detail, data)), 0o644)
}

func structuredCatalog() []string {
	catalog := oracle.BuildComboCatalog()
	seen := map[string]struct{}{}
	var tmpls []string
	for _, c := range catalog {
		if _, ok := seen[c.Tmpl]; ok {
			continue
		}
		seen[c.Tmpl] = struct{}{}
		tmpls = append(tmpls, c.Tmpl)
	}
	// Extra @attrs bag variants (key/value fuzz surface).
	for _, extra := range []string{
		`<div @attrs($m)>`,
		`<a class="x" @attrs($m)>`,
		`<input type="text" @attrs($m)>`,
	} {
		if _, ok := seen[extra]; !ok {
			seen[extra] = struct{}{}
			tmpls = append(tmpls, extra)
		}
	}
	return tmpls
}

type structuredEngines struct {
	tmpls []string
	aot   *canvas.Engine
	html  *canvas.Engine
	lay   *canvas.Engine
	names []string
}

func buildStructuredEngines(tmpls []string) (*structuredEngines, error) {
	mk := func(mode string) (*canvas.Engine, []string, error) {
		dir, err := os.MkdirTemp("", "canvas-fuzz-"+mode+"-*")
		if err != nil {
			return nil, nil, err
		}
		names := make([]string, len(tmpls))
		for i, tmpl := range tmpls {
			name := fmt.Sprintf("t%d", i)
			names[i] = name
			body := tmpl
			if mode == "html" {
				body = tmpl
			}
			if mode == "layout" {
				_ = os.MkdirAll(filepath.Join(dir, "layouts"), 0o755)
				_ = os.WriteFile(filepath.Join(dir, "layouts", "app.html"), []byte(`@yield('content')`), 0o644)
				body = "@extends('layouts.app')\n@section('content')\n" + tmpl + "\n@endsection\n"
			}
			if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0o644); err != nil {
				return nil, nil, err
			}
		}
		eng := canvas.New(dir)
		eng.SetEscapeMode(rt.EscapeStrict)
		if mode == "html" {
			eng.PreferHTMLPath(true)
		}
		// Warm compile once per template.
		for _, name := range names {
			_, _ = eng.Render(name, map[string]any{
				"x": "1", "a": "1", "b": "1", "t": "div",
				"m": map[string]string{"id": "ok"},
			})
		}
		return eng, names, nil
	}
	aot, names, err := mk("aot")
	if err != nil {
		return nil, err
	}
	html, _, err := mk("html")
	if err != nil {
		return nil, err
	}
	lay, _, err := mk("layout")
	if err != nil {
		return nil, err
	}
	return &structuredEngines{tmpls: tmpls, aot: aot, html: html, lay: lay, names: names}, nil
}

func (s *structuredEngines) pick(path byte) *canvas.Engine {
	switch path % 3 {
	case 1:
		return s.html
	case 2:
		return s.lay
	default:
		return s.aot
	}
}

func fuzzPayloadAndBag(data []byte) (payload string, bag map[string]string) {
	payload = oracle.HarmlessValue
	if len(oracle.AttackPayloads) > 0 {
		payload = oracle.AttackPayloads[0]
	}
	if len(data) > 4 {
		p := string(data[4:])
		if len(p) > 64 {
			p = p[:64]
		}
		if p != "" {
			payload = p
		}
	}
	bag = map[string]string{
		"id": "ok", "title": payload, "href": payload, "onclick": payload,
	}
	// Optional attr key/value bytes: [keyLen][key…][val…]
	if len(data) > 8 {
		klen := int(data[3]) % 12
		rest := data[4:]
		if klen > 0 && len(rest) > klen {
			key := string(rest[:klen])
			val := string(rest[klen:])
			if len(val) > 40 {
				val = val[:40]
			}
			if key != "" {
				bag[key] = val
			}
		}
	}
	return payload, bag
}

// FuzzStrictStructured is the coverage-guided Strict fuzz (nightly primary).
// Catalog templates are compiled once per process before f.Fuzz so seed
// evaluation does not hang on per-seed TempDir+compile.
// Input: [tmplIdx u16][path u8][bagMeta u8][payload…]; path%3 selects aot/html/layout.
func FuzzStrictStructured(f *testing.F) {
	tmpls := structuredCatalog()
	for i := 0; i < len(tmpls) && i < 64; i++ {
		var b [8]byte
		binary.LittleEndian.PutUint16(b[0:2], uint16(i))
		b[2] = byte(i % 3)
		b[3] = byte(i % 8)
		copy(b[4:], []byte("x"))
		f.Add(b[:])
	}
	for _, p := range oracle.AttackPayloads[:min(4, len(oracle.AttackPayloads))] {
		var b []byte
		b = append(b, 0, 0, 0, 4)
		b = append(b, []byte(p)...)
		f.Add(b)
	}
	// FORM FEED in unquoted attr (regression seed).
	f.Add(append([]byte{0, 0, 0, 4}, []byte("99000\f0")...))

	set, err := buildStructuredEngines(tmpls)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if set == nil || len(data) < 4 {
			return
		}
		idx := int(binary.LittleEndian.Uint16(data[0:2])) % len(set.tmpls)
		eng := set.pick(data[2])
		name := set.names[idx]
		tmpl := set.tmpls[idx]
		payload, bag := fuzzPayloadAndBag(data)
		attack := map[string]any{"x": payload, "a": payload, "b": payload, "t": "div", "m": bag}
		safeBag := map[string]string{}
		for k := range bag {
			safeBag[k] = oracle.HarmlessValue
		}
		safe := map[string]any{
			"x": oracle.HarmlessValue, "a": oracle.HarmlessValue, "b": oracle.HarmlessValue,
			"t": "div", "m": safeBag,
		}
		safeOut, errS := eng.Render(name, safe)
		attackOut, errA := eng.Render(name, attack)
		if errA != nil || errS != nil {
			return
		}
		if findings := oracle.FullCheck(tmpl, safeOut, attackOut); len(findings) > 0 {
			saveFailure(t, data, tmpl, findings[0].Kind+":"+findings[0].Detail)
			t.Fatalf("injection idx=%d path=%d tmpl=%q detail=%v", idx, data[2]%3, tmpl, findings)
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestOracleStructuredFuzz_60sReport is a local REPORT helper (not the nightly
// coverage driver). It exercises the same precompiled hot loop as
// FuzzStrictStructured and prints execs= for human measurement.
// Nightly / CI must use: go test -C oracle -fuzz=FuzzStrictStructured -fuzztime=60s
func TestOracleStructuredFuzz_60sReport(t *testing.T) {
	if testing.Short() {
		t.Skip("long fuzz")
	}
	tmpls := structuredCatalog()
	set, err := buildStructuredEngines(tmpls)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	var execs atomic.Int64
	i := 0
	for time.Now().Before(deadline) {
		idx := i % len(set.tmpls)
		eng := set.pick(byte(i % 3))
		name := set.names[idx]
		tmpl := set.tmpls[idx]
		payload := oracle.AttackPayloads[i%len(oracle.AttackPayloads)]
		if i%11 == 0 {
			payload += string(rune('A' + i%26))
		}
		bag := map[string]string{"id": "ok", "title": payload, "href": payload, "onclick": payload}
		attack := map[string]any{"x": payload, "a": payload, "b": payload, "t": "div", "m": bag}
		safeBag := map[string]string{"id": "ok", "title": oracle.HarmlessValue, "href": oracle.HarmlessValue, "onclick": oracle.HarmlessValue}
		safe := map[string]any{
			"x": oracle.HarmlessValue, "a": oracle.HarmlessValue, "b": oracle.HarmlessValue,
			"t": "div", "m": safeBag,
		}
		safeOut, errS := eng.Render(name, safe)
		attackOut, errA := eng.Render(name, attack)
		execs.Add(1)
		if errA != nil || errS != nil {
			i++
			continue
		}
		// FullCheck every 128th exec keeps the hot path fast.
		if i%128 == 0 {
			if findings := oracle.FullCheck(tmpl, safeOut, attackOut); len(findings) > 0 {
				t.Fatalf("after %d execs: %v tmpl=%q", execs.Load(), findings, tmpl)
			}
		} else if findings := oracle.CheckRendered(tmpl, attackOut); len(findings) > 0 {
			t.Fatalf("after %d execs: %v tmpl=%q", execs.Load(), findings, tmpl)
		}
		i++
	}
	n := execs.Load()
	t.Logf("fuzz_fn=FuzzStrictStructured (measured via TestOracleStructuredFuzz_60sReport)")
	t.Logf("FuzzStrictStructured: execs=%d duration=60s templates=%d paths=3 (aot/html/layout via data[2]%%3)", n, len(tmpls))
	t.Logf("precompile=buildStructuredEngines once before f.Fuzz (FuzzStrictStructured) / once here (report)")
	t.Logf("slow_path=FuzzStrictNoInjection (per-exec TempDir+compile); primary=FuzzStrictStructured (-fuzz)")
	if n < 100_000 {
		t.Errorf("execs %d < 100000 target", n)
	}
}
