//go:build !canvas_poison

package rt_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPoison_TagAbsentFromDefaultBinary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Dir(filepath.Dir(file))
	cmd := exec.Command("go", "list", "-f", "{{range .GoFiles}}{{.}} {{end}}", ".")
	cmd.Dir = filepath.Join(root, "rt")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	s := string(out)
	if !strings.Contains(s, "poison_stub.go") {
		t.Fatalf("want poison_stub.go in default GoFiles, got %s", s)
	}
	if strings.Contains(s, "poison.go") && !strings.Contains(s, "poison_stub.go") {
		t.Fatalf("poison.go without stub: %s", s)
	}
	// tagged poison.go must not appear in default file list
	files := strings.Fields(s)
	for _, f := range files {
		if f == "poison.go" {
			t.Fatalf("canvas_poison file included in default build: %s", s)
		}
	}
}
