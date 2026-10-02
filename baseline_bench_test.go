package canvas_test

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

// Plain html/template baselines vs dataGet strawman.
// Expectations written BEFORE running (do not rewrite after):
//   - dataGet path is slower than plain html/template (map or struct)
//   - README "≥50× vs dataGet" may overstate vs plain html/template

func BenchmarkBaseline_HTMLTemplateMap(b *testing.B) {
	tmpl := template.Must(template.New("page").Parse(
		`<h1>{{.Title}}</h1>{{range .Items}}<li>{{.Name}}</li>{{end}}`,
	))
	items := make([]map[string]any, 50)
	for i := 0; i < 50; i++ {
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

func BenchmarkBaseline_HTMLTemplateStruct(b *testing.B) {
	type item struct{ Name string }
	type page struct {
		Title string
		Items []item
	}
	tmpl := template.Must(template.New("page").Parse(
		`<h1>{{.Title}}</h1>{{range .Items}}<li>{{.Name}}</li>{{end}}`,
	))
	items := make([]item, 50)
	for i := range items {
		items[i].Name = fmt.Sprintf("Item-%d", i)
	}
	data := page{Title: "Bench", Items: items}
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

func BenchmarkBaseline_HTMLTemplateDataGet(b *testing.B) {
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
	items := make([]map[string]any, 50)
	for i := 0; i < 50; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{"title": "Bench", "items": items}
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

func BenchmarkBaseline_CanvasRTTo(b *testing.B) {
	dir := b.TempDir()
	src := `<h1>{{ $title }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(src), 0o644); err != nil {
		b.Fatal(err)
	}
	eng := canvas.New(dir)
	items := make([]map[string]any, 50)
	for i := 0; i < 50; i++ {
		items[i] = map[string]any{"name": fmt.Sprintf("Item-%d", i)}
	}
	data := map[string]any{"title": "Bench", "items": items}
	w := rt.AcquireWriter()
	if err := eng.RenderTo(w, "page", data); err != nil {
		b.Fatal(err)
	}
	rt.ReleaseWriter(w)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := rt.AcquireWriter()
		if err := eng.RenderTo(w, "page", data); err != nil {
			b.Fatal(err)
		}
		rt.ReleaseWriter(w)
	}
}
