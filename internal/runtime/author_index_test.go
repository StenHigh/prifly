package runtime

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The references kept up with every contract this engine gained; the page that
// leads to them did not. It described the step reference as covering
// prifly-step/1 when the file had moved to /2 and contract 10, and it named
// none of blocked, external_write, model_profile or technical_retries -- the
// last of which appeared in no authoring file at all. An author choosing what
// to build reads that page, so a capability missing from it is a capability
// nobody can find.
//
// This is the rule made checkable: what the build says it can do and what the
// author index lists are the same set, in both directions. A capability without
// a row fails, and a row for a capability this build does not declare fails too,
// because a promise the engine has withdrawn is worse than a missing one.
func TestEveryDeclaredCapabilityIsInTheAuthorIndex(t *testing.T) {
	const index = "../../examples/README.md"
	page, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^\| `+"`"+`([a-z_.]+)`+"`"+` \|`).FindAllStringSubmatch(string(page), -1) {
		rows[match[1]] = true
	}
	if len(rows) == 0 {
		t.Fatalf("%s has no capability table: the check would pass by reading nothing", index)
	}
	declared := Capabilities().Profiles[1].Capabilities
	for _, capability := range declared {
		if !rows[capability] {
			t.Errorf("this build declares %s and the author index has no row for it: nobody can find it", capability)
		}
	}
	for row := range rows {
		if !slices.Contains(declared, row) {
			t.Errorf("the author index promises %s and this build does not declare it", row)
		}
	}
	// The table is the map, not a list of names: a row that names no authoring
	// surface and no reason for its absence teaches nothing.
	for _, line := range strings.Split(string(page), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		columns := strings.Split(strings.Trim(line, "| "), " | ")
		if len(columns) < 3 || strings.TrimSpace(columns[1]) == "" || strings.TrimSpace(columns[2]) == "" {
			t.Errorf("a capability row says where nothing: %s", line)
		}
	}
}
