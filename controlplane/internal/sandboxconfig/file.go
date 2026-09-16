package sandboxconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

// LoadFile reads path and maps it through Load. Relative dockerfile/mount
// sources resolve against the file's directory.
func LoadFile(path string) (sandbox.OpenOptions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sandbox.OpenOptions{}, fmt.Errorf("read sandbox config %q: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return sandbox.OpenOptions{}, fmt.Errorf("resolve sandbox config path %q: %w", path, err)
	}
	return Load(filepath.Dir(abs), data)
}
