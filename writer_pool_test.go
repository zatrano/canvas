package canvas_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

// Writer pool lifetime + concurrent Engine use.
// Expectation: Bytes() alias into pooled buffer; after ReleaseWriter may be overwritten.
// Concurrent Render on same Engine should not cross-contaminate (race detector + equality).

func TestWriterPool_BytesAfterRelease(t *testing.T) {
	w := rt.AcquireWriter()
	w.WriteString("FIRST_PAYLOAD_AAAAAAAA")
	b := w.Bytes()
	snapshot := string(b)
	rt.ReleaseWriter(w)

	w2 := rt.AcquireWriter()
	w2.WriteString("SECOND_PAYLOAD_BBBBBBBBBBBBBBBBBBBB")
	_ = w2.Bytes()
	// Do NOT release yet — overlap window
	corrupted := string(b) != snapshot
	t.Logf("BEFORE_RELEASE_SNAPSHOT=%q AFTER_REUSE_VIEW=%q CORRUPTED=%v", snapshot, string(b), corrupted)
	rt.ReleaseWriter(w2)

	// Also check after second release
	t.Logf("FINAL_VIEW=%q", string(b))
}

func TestWriterPool_ConcurrentRender(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(
		`<h1>{{ $id }}</h1>@foreach($items as $item)<li>{{ $item.name }}</li>@endforeach`,
	), 0o644)
	eng := canvas.New(dir)

	const goroutines = 100
	const iters = 200
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			items := []map[string]any{{"name": fmt.Sprintf("g%d", id)}}
			data := map[string]any{"id": id, "items": items}
			wantPrefix := fmt.Sprintf("<h1>%d</h1>", id)
			for i := 0; i < iters; i++ {
				out, err := eng.Render("page", data)
				if err != nil {
					errCh <- err
					return
				}
				if !bytes.HasPrefix([]byte(out), []byte(wantPrefix)) {
					errCh <- fmt.Errorf("cross-talk: want prefix %q got %q", wantPrefix, out)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestWriterPool_ConcurrentRenderTo(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "page.html"), []byte(`{{ $id }}`), 0o644)
	eng := canvas.New(dir)
	const goroutines = 50
	const iters = 500
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := map[string]any{"id": id}
			want := fmt.Sprintf("%d", id)
			for i := 0; i < iters; i++ {
				w := rt.AcquireWriter()
				if err := eng.RenderTo(w, "page", data); err != nil {
					rt.ReleaseWriter(w)
					errCh <- err
					return
				}
				got := string(w.Bytes())
				rt.ReleaseWriter(w)
				if got != want {
					errCh <- fmt.Errorf("want %q got %q", want, got)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}
