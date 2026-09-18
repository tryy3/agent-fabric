package sandbox

import "github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"

func ResolveUnderRoot(root, userPath string) (string, error) {
	return sandboxcore.ResolveUnderRoot(root, userPath)
}
