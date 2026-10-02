package canvas_test

import (
	"testing"

	"github.com/zatrano/canvas/rt"
)

// Differential: our context class vs what we expect html/template would treat as URL/attr/script.
func TestEscapeContext_DifferentialClasses(t *testing.T) {
	type row struct {
		prefix string
		json   bool
		want   rt.EscapeKind
		note   string
	}
	rows := []row{
		{`<p>`, false, rt.EscHTML, "text"},
		{`<a title="`, false, rt.EscHTML, "quoted attr"},
		{`<a title=`, false, rt.EscUnquoted, "unquoted attr"},
		{`<a href="`, false, rt.EscURLAttr, "url href start"},
		{`<a href="/p/`, false, rt.EscHTML, "url with static prefix"},
		{`<a onclick="`, false, rt.EscForbid, "event handler"},
		{`<script>`, false, rt.EscForbid, "script body"},
		{`<style>`, false, rt.EscForbid, "style body"},
		{`<script>var a=`, true, rt.EscJSON, "json in script"},
		{`<div data-x="`, true, rt.EscJSONAttr, "json in attr"},
	}
	var diffs []string
	for _, r := range rows {
		got := rt.ScanEscapeKind(r.prefix, r.json, rt.EscapeStrict)
		if got != r.want {
			diffs = append(diffs, r.note+": got "+got.String()+" want "+r.want.String())
		}
	}
	if len(diffs) > 0 {
		t.Fatalf("classification diffs: %v", diffs)
	}
}
