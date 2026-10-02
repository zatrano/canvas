package oracle_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMutationResults_BreakingTestsExist ensures every mutation_results.json
// breaking_test names a test that `go test -list` reports in a related module
// (top-level name; subtest suffixes are exercised by the mutation runner).
func TestMutationResults_BreakingTestsExist(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "oracle", "mutation_results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Rule         string `json:"rule"`
		BreakingTest string `json:"breaking_test"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("mutation_results.json empty")
	}

	listTests := func(dir string) map[string]struct{} {
		t.Helper()
		cmd := exec.Command("go", "test", "-list", ".*")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go test -list in %s: %v\n%s", dir, err, out)
		}
		set := map[string]struct{}{}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Test") {
				set[line] = struct{}{}
			}
		}
		return set
	}

	listed := map[string]struct{}{}
	for _, dir := range []string{
		root,
		filepath.Join(root, "oracle"),
		filepath.Join(root, "rt"),
		filepath.Join(root, "ast"),
		filepath.Join(root, "lex"),
	} {
		for name := range listTests(dir) {
			listed[name] = struct{}{}
		}
	}

	for _, row := range rows {
		if row.BreakingTest == "" {
			t.Errorf("rule %q: empty breaking_test", row.Rule)
			continue
		}
		top, _, _ := strings.Cut(row.BreakingTest, "/")
		if _, ok := listed[top]; !ok {
			t.Errorf("rule %q: breaking_test %q (top-level %q) not in go test -list of canvas modules",
				row.Rule, row.BreakingTest, top)
		}
	}
}
