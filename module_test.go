package toml

import (
	"os"
	"strings"
	"testing"
)

// The reason this module exists is that it depends on nothing. Every consumer
// gave up a third-party dependency for it; a require line would hand one back.
func TestTheModuleRequiresNothing(t *testing.T) {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require") {
			t.Fatalf("go.mod gained a dependency: %q", line)
		}
	}
}
