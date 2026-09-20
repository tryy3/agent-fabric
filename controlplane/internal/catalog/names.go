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
		return "", fmt.Errorf("name template is empty")
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
				expandErr = fmt.Errorf("name template %q requires projectID", template)
				return ""
			}
			return vars.ProjectID
		case "{threadID}":
			if strings.TrimSpace(vars.ThreadID) == "" {
				expandErr = fmt.Errorf("name template %q requires threadID", template)
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
			expandErr = fmt.Errorf("unknown name variable %s", match)
			return ""
		}
	})
	if expandErr != nil {
		return "", expandErr
	}
	return out, nil
}

type ResolvedVolume struct {
	ID       string
	Name     string
	Target   string
	ReadOnly bool
}

func ExpandVolumes(rows []VolumeRow, vars NameVars, prefix string) ([]ResolvedVolume, error) {
	out := make([]ResolvedVolume, 0, len(rows))
	for _, row := range rows {
		if row.Enabled != nil && !*row.Enabled {
			continue
		}
		name := strings.TrimSpace(stringValue(row.Name))
		target := strings.TrimSpace(stringValue(row.Target))
		if name == "" || target == "" {
			continue
		}
		expanded, err := ExpandName(name, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, ResolvedVolume{
			ID:       row.ID,
			Name:     ApplyIdentityPrefix(expanded, prefix),
			Target:   target,
			ReadOnly: row.Write != nil && !*row.Write,
		})
	}
	return out, nil
}

func HasVolumeTarget(volumes []ResolvedVolume, target string) bool {
	for _, volume := range volumes {
		if volume.Target == target {
			return true
		}
	}
	return false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
