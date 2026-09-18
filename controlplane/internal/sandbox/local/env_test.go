package local_test

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox/local"
)

func TestNewRejectsEmptyWorkspaceRoot(t *testing.T) {
	for _, root := range []string{"", " \t\n"} {
		_, err := local.New(root)
		if err == nil || !strings.Contains(err.Error(), "workspace root") {
			t.Fatalf("New(%q) error = %v", root, err)
		}
	}
}
