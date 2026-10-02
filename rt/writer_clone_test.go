package rt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zatrano/canvas"
	"github.com/zatrano/canvas/rt"
)

func TestWriter_CloneBytesIndependentOfPool(t *testing.T) {
	w := rt.AcquireWriter()
	w.WriteString("CLONE_ME")
	cp := w.CloneBytes()
	rt.ReleaseWriter(w)
	w2 := rt.AcquireWriter()
	w2.WriteString("XXXXXXXX")
	rt.ReleaseWriter(w2)
	if string(cp) != "CLONE_ME" {
		t.Fatalf("CloneBytes corrupted: %q", cp)
	}
}

// TestRenderTo_NonPoolWriterUnaffectedByPoolPoison: RenderTo into a caller-owned
// Writer (not from AcquireWriter). Releasing other pooled writers must not touch it.
func TestRenderTo_NonPoolWriterUnaffectedByPoolPoison(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "p.html"), []byte(`hi {{ $x }}`), 0o644)
	eng := canvas.New(dir)

	var external rt.Writer
	if err := eng.RenderTo(&external, "p", map[string]any{"x": "ok"}); err != nil {
		t.Fatal(err)
	}
	got := string(external.Bytes())
	if got != "hi ok" {
		t.Fatalf("got %q", got)
	}

	// churn the pool with poison-capable releases
	for i := 0; i < 32; i++ {
		w := rt.AcquireWriter()
		w.WriteString("POISON_CHURN___________")
		rt.ReleaseWriter(w)
	}
	if string(external.Bytes()) != "hi ok" {
		t.Fatalf("external writer corrupted: %q", external.Bytes())
	}
}
