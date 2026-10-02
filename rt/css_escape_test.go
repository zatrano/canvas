package rt_test

import (
	"testing"

	"github.com/zatrano/canvas/rt"
)

func TestEscapeCSSValue(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"10", "10"},
		{"#fff", "#fff"},
		{"red", "red"},
		{"red;background:url(javascript:x)", "unsafe"},
		{"expression(alert(1))", "unsafe"},
		{`}</style><script>`, "unsafe"},
		{`\3c script`, "unsafe"},
		{"url(javascript:x)", "unsafe"},
		{"/*x*/", "unsafe"},
	}
	for _, tc := range cases {
		if got := rt.EscapeCSSValue(tc.in); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestStyleAttrKind_Scan(t *testing.T) {
	if k := rt.ScanEscapeKind(`<div style="`, false, rt.EscapeStrict); k != rt.EscForbid {
		t.Fatalf("whole style value: got %v", k)
	}
	if k := rt.ScanEscapeKind(`<div style="width: `, false, rt.EscapeStrict); k != rt.EscCSS {
		t.Fatalf("css value position: got %v want EscCSS", k)
	}
	if k := rt.ScanEscapeKind(`<div style="`, true, rt.EscapeStrict); k != rt.EscForbid {
		t.Fatalf("json in style: got %v", k)
	}
}
