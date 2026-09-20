package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var namePlaceholder = regexp.MustCompile(`\{[^{}]+\}`)

type NameVars struct {
	ProjectID string
	ThreadID  string
}

func ExpandName(template string, vars NameVars) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", fmt.Errorf("container name template is empty")
	}
	if strings.Contains(template, "{userID}") {
		return "", fmt.Errorf("{userID} is not expanded in v1")
	}

	var random string
	var expandErr error
	out := namePlaceholder.ReplaceAllStringFunc(template, func(match string) string {
		if expandErr != nil {
			return ""
		}
		switch match {
		case "{projectID}":
			if strings.TrimSpace(vars.ProjectID) == "" {
				expandErr = fmt.Errorf("container name template %q requires projectID", template)
				return ""
			}
			return vars.ProjectID
		case "{threadID}":
			if strings.TrimSpace(vars.ThreadID) == "" {
				expandErr = fmt.Errorf("container name template %q requires threadID", template)
				return ""
			}
			return vars.ThreadID
		case "{random}":
			if random == "" {
				value, err := randomHex8()
				if err != nil {
					expandErr = err
					return ""
				}
				random = value
			}
			return random
		default:
			expandErr = fmt.Errorf("unknown container name variable %s", match)
			return ""
		}
	})
	if expandErr != nil {
		return "", expandErr
	}
	return out, nil
}

func ApplyIdentityPrefix(name, prefix string) string {
	name = strings.TrimSpace(name)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || name == "" {
		return name
	}
	if strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

func randomHex8() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random container name: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
