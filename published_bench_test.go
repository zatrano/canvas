package canvas_test

import (
	"os"
	"strings"
	"testing"
)

// Gate and microbench tests live in bench/ and the root *_bench_test.go files.
// This test locks the published host snapshot for this repository.
func TestPublishedCanvasBench(t *testing.T) {
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, piece := range []string{
		"Current release is **v1.0.1**",
		"go get github.com/zatrano/canvas@v1.0.1",
		"| Typed list, gate | 1061 ns | 3913 ns | — | 3.69× |",
		"| Dynamic `RenderTo`, gate | 4865 ns | 3870 ns | — | 0.80× |",
		"| Typed vs quicktemplate | ≥ 2.0× |",
		"| Dynamic `RenderTo` vs quicktemplate | ≥ 1.05× |",
	} {
		if !strings.Contains(text, piece) {
			t.Fatalf("README is missing %s", piece)
		}
	}
}
