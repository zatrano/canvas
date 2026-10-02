package rt_test

import (
	"strings"
	"testing"

	"github.com/zatrano/canvas/rt"
)

func TestBeforeValueURLIsUnquoted(t *testing.T) {
	k := rt.ScanEscapeKind("<div poster=", false, rt.EscapeStrict)
	if k != rt.EscURLUnquoted {
		t.Fatalf("got %s want url-unquoted", k)
	}
	k2 := rt.ScanEscapeKind("<div href=", false, rt.EscapeStrict)
	if k2 != rt.EscURLUnquoted {
		t.Fatalf("href: got %s", k2)
	}
}

func TestUnquotedURLFormFeed(t *testing.T) {
	// EscURLUnquoted: scheme filter then unquoted escape — FF must not survive raw.
	s := rt.EscapeURLAttr("9*000\f", true)
	got := rt.EscapeUnquotedAttr(s)
	if strings.Contains(got, "\f") {
		t.Fatalf("raw FF survived: %q", got)
	}
}
