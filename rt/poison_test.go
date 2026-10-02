//go:build canvas_poison

package rt_test

import (
	"testing"

	"github.com/zatrano/canvas/rt"
)

func TestPoison_BytesAfterReleaseAreFilled(t *testing.T) {
	w := rt.AcquireWriter()
	w.WriteString("SENSITIVE_PAYLOAD_AAAA")
	alias := w.Bytes()
	if len(alias) == 0 {
		t.Fatal("empty")
	}
	rt.ReleaseWriter(w)
	for i, b := range alias {
		if b != 0xDE {
			t.Fatalf("alias[%d]=0x%02x want 0xDE (full=%x)", i, b, alias)
		}
	}
}
