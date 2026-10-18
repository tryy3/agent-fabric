package catalog

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var skillSettingName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// SkillsSettings is the skills object of plane settings, project settings and
// assistant settings: the names of skills switched off at that level.
type SkillsSettings struct {
	Disabled []string `json:"disabled,omitempty"`
}

// DecodeSkillsSettings reads a skills object leniently: null, empty and
// malformed input mean no toggles, so a bad value never hides every skill.
func DecodeSkillsSettings(raw json.RawMessage) SkillsSettings {
	var s SkillsSettings
	if len(raw) == 0 || isJSONNull(raw) {
		return s
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return SkillsSettings{}
	}
	return s
}

// SkillsFromSettings returns the skills object inside a settings JSON blob
// (project settings or assistant settings).
func SkillsFromSettings(settings json.RawMessage) SkillsSettings {
	if len(settings) == 0 || isJSONNull(settings) {
		return SkillsSettings{}
	}
	var wrapper struct {
		Skills json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(settings, &wrapper); err != nil {
		return SkillsSettings{}
	}
	return DecodeSkillsSettings(wrapper.Skills)
}

// normalizeSkillsSettings validates a plane-wide skills patch and returns the
// stored form: trimmed, de-duplicated and sorted names. Null clears it.
func normalizeSkillsSettings(raw json.RawMessage) (json.RawMessage, error) {
	if isJSONNull(raw) {
		return json.RawMessage(`{}`), nil
	}
	var s SkillsSettings
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	seen := map[string]bool{}
	var names []string
	for _, name := range s.Disabled {
		name = strings.TrimSpace(name)
		if len(name) > 64 || !skillSettingName.MatchString(name) {
			return nil, fmt.Errorf("skills.disabled: %q is not a valid skill name", name)
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return json.Marshal(SkillsSettings{Disabled: names})
}
