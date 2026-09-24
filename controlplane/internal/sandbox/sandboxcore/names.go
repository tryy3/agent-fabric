package sandboxcore

import (
	"fmt"
	"regexp"
	"strings"
)

// Docker/Podman --name charset: [a-zA-Z0-9][a-zA-Z0-9_.-]*
var containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func ValidateContainerName(name string) error {
	return validateDockerObjectName("container", name)
}

func ValidateVolumeName(name string) error {
	return validateDockerObjectName("volume", name)
}

func validateDockerObjectName(kind, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s name is empty", kind)
	}
	if !containerNamePattern.MatchString(name) {
		return fmt.Errorf("invalid %s name %q", kind, name)
	}
	return nil
}
