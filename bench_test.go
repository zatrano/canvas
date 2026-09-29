package canvas_test

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cv "github.com/zatrano/canvas"
	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/rt"
)

const benchItems = 50

func benchData() map[string]any {
	items := make([]map[string]any, benchItems)
	for i := 0; i < benchItems; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i), "n": i}
	}
	return map[string]any{
		"title":  "Bench",
		"items":  items,
		"_token": "tok",
	}
}

func benchDataN(n int) map[string]any {
	items := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i), "n": i}
	}
	return map[string]any{"title": "Bench", "items": items, "_token": "tok"}
}

func writePage(b *testing.B, dir, src string) {
	b.Helper()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkCanvasRT is the Canvas fast runtime (AST → Program), cached.
func BenchmarkCanvasRT(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	writePage(b, dir, src)
	e := cv.New(dir)
	e.EnableCache(true)
	data := benchData()
	if _, err := e.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Render("page", data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCanvasRTTo pools the writer (no string copy).
func BenchmarkCanvasRTTo(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	writePage(b, dir, src)
	e := cv.New(dir)
	e.EnableCache(true)
	data := benchData()
	w := rt.AcquireWriter()
	if err := e.RenderTo(w, "page", data); err != nil {
		b.Fatal(err)
	}
	rt.ReleaseWriter(w)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		if err := e.RenderTo(w, "page", data); err != nil {
			b.Fatal(err)
		}
		rt.ReleaseWriter(w)
	}
}

// BenchmarkHTMLTemplate is Go's standard html/template (cached parse).
func BenchmarkHTMLTemplate(b *testing.B) {
	tmpl := template.Must(template.New("page").Parse(
		`<h1>{{.Title}}</h1>{{range .Items}}<li>{{.Name}}</li>{{end}}`,
	))
	items := make([]map[string]any, benchItems)
	for i := 0; i < benchItems; i++ {
		items[i] = map[string]any{"Name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{"Title": "Bench", "Items": items}
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tmpl.Execute(&buf, data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTMLTemplateDataGet mirrors the old Canvas→html/template pipeline
// (FuncMap dataGet + range) — the industry-default slow path.
func BenchmarkHTMLTemplateDataGet(b *testing.B) {
	funcMap := template.FuncMap{
		"dataGet": func(data any, path string) any {
			m, _ := data.(map[string]any)
			if m == nil {
				return nil
			}
			parts := strings.Split(path, ".")
			var cur any = m
			for _, p := range parts {
				mm, ok := cur.(map[string]any)
				if !ok {
					return nil
				}
				cur = mm[p]
			}
			return cur
		},
	}
	tmpl := template.Must(template.New("page").Funcs(funcMap).Parse(
		`<h1>{{ dataGet . "title" }}</h1>{{ range $i, $item := dataGet . "items" }}<li>{{ dataGet $item "name" }}</li>{{ end }}`,
	))
	data := benchData()
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tmpl.Execute(&buf, data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCeilingQTStyle is the theoretical floor: typed data, no maps, no
// engine — what code-generated engines (quicktemplate-class) approximate.
// Canvas must stay within a small factor of this on the same HTML shape.
func BenchmarkCeilingQTStyle(b *testing.B) {
	type item struct{ Name string }
	items := make([]item, benchItems)
	for i := range items {
		items[i].Name = fmt.Sprintf("Item-%d", i)
	}
	title := "Bench"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		w.WriteString("<h1>")
		rtWriteEsc(w, title)
		w.WriteString("</h1>")
		for j := 0; j < len(items); j++ {
			w.WriteString("<li>")
			rtWriteEsc(w, items[j].Name)
			w.WriteString("</li>")
		}
		_ = w.Bytes()
		rt.ReleaseWriter(w)
	}
}

func rtWriteEsc(w *rt.Writer, s string) {
	// mirror rt fast escape without exporting
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&', '<', '>', '"', '\'':
			for j := 0; j < len(s); j++ {
				switch s[j] {
				case '&':
					w.WriteString("&amp;")
				case '<':
					w.WriteString("&lt;")
				case '>':
					w.WriteString("&gt;")
				case '"':
					w.WriteString("&#34;")
				case '\'':
					w.WriteString("&#39;")
				default:
					w.AppendByte(s[j])
				}
			}
			return
		}
	}
	w.WriteString(s)
}

// BenchmarkHandWritten is map-based manual render (fair vs Canvas data model).
func BenchmarkHandWritten(b *testing.B) {
	data := benchData()
	items := data["items"].([]map[string]any)
	title := data["title"].(string)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		buf.WriteString("<h1>")
		template.HTMLEscape(&buf, []byte(title))
		buf.WriteString("</h1>")
		for _, item := range items {
			buf.WriteString("<li>")
			template.HTMLEscape(&buf, []byte(item["name"].(string)))
			buf.WriteString("</li>")
		}
		_ = buf.String()
	}
}

// BenchmarkCanvasRT200 is 200-item list (amortizes fixed overhead).
func BenchmarkCanvasRT200(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	writePage(b, dir, src)
	e := cv.New(dir)
	e.EnableCache(true)
	data := benchDataN(200)
	if _, err := e.Render("page", data); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Render("page", data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTMLTemplate200 pairs with BenchmarkCanvasRT200.
func BenchmarkHTMLTemplate200(b *testing.B) {
	tmpl := template.Must(template.New("page").Parse(
		`<h1>{{.Title}}</h1>{{range .Items}}<li>{{.Name}}</li>{{end}}`,
	))
	items := make([]map[string]any, 200)
	for i := 0; i < 200; i++ {
		items[i] = map[string]any{"Name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{"Title": "Bench", "Items": items}
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tmpl.Execute(&buf, data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTMLTemplateDataGet200 is the legacy Canvas→html/template FuncMap path.
func BenchmarkHTMLTemplateDataGet200(b *testing.B) {
	funcMap := template.FuncMap{
		"dataGet": func(data any, path string) any {
			parts := strings.Split(path, ".")
			cur := data
			for _, p := range parts {
				mm, ok := cur.(map[string]any)
				if !ok {
					return nil
				}
				cur = mm[p]
			}
			return cur
		},
	}
	tmpl := template.Must(template.New("page").Funcs(funcMap).Parse(
		`<h1>{{ dataGet . "title" }}</h1>{{ range $i, $item := dataGet . "items" }}<li>{{ dataGet $item "name" }}</li>{{ end }}`,
	))
	data := benchDataN(200)
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tmpl.Execute(&buf, data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCanvasCompileOnly measures one-shot compile (AST+rt).
func BenchmarkCanvasCompileOnly(b *testing.B) {
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc, err := ast.ParseSource(src)
		if err != nil {
			b.Fatal(err)
		}
		if _, ok, err := rt.CompileFunc(doc, "local"); err != nil || !ok {
			b.Fatal(err)
		}
	}
}
