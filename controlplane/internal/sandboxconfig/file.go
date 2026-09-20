package sandboxconfig

import (
	"fmt"
	"os"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func LoadFile(path string) (Engine, catalog.DeprecatedSandbox, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Engine{}, catalog.DeprecatedSandbox{}, fmt.Errorf("read sandbox config %q: %w", path, err)
	}
	return Load(data)
}
