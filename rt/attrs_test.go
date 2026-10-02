package rt_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/rt"
)

func TestFormatAttrs(t *testing.T) {
	got := rt.FormatAttrs(map[string]string{"id": "a", "class": "c"})
	if !strings.Contains(got, ` class="c"`) || !strings.Contains(got, ` id="a"`) {
		t.Fatalf("got %q", got)
	}
	if rt.FormatAttrs(nil) != "" {
		t.Fatal("nil")
	}
	got = rt.FormatAttrs(map[string]any{"disabled": true, "hidden": false})
	if !strings.Contains(got, " disabled") || strings.Contains(got, "hidden") {
		t.Fatalf("bool: %q", got)
	}
	got = rt.FormatAttrs(map[string]string{"onclick": "x", "href": "javascript:alert(1)"})
	if strings.Contains(got, "onclick") || strings.Contains(got, "javascript") {
		t.Fatalf("reject: %q", got)
	}
	if !strings.Contains(got, `#unsafe`) {
		t.Fatalf("url filter: %q", got)
	}
}
