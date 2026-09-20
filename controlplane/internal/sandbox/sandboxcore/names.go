package sandboxcore

import (
	"fmt"
	"regexp"
	"strings"
)

// Docker/Podman --name charset: [a-zA-Z0-9][a-zA-Z0-9_.-]*
var containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func ValidateContainerName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("container name is empty")
	}
	if !containerNamePattern.MatchString(name) {
		return fmt.Errorf("invalid container name %q", name)
	}
	return nil
}
