package skills

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const maxNameLen = 64

// parseSkillMD splits SKILL.md into frontmatter fields and body. The
// frontmatter is the flat `key: value` subset of YAML that SKILL.md files use;
// `flags` may be an inline `[a, b]` list or a dash list. Unknown keys are
// ignored. dirName fills in a missing name and must match a given one.
func parseSkillMD(raw []byte, dirName string) (Skill, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	if !strings.HasPrefix(text, "---\n") {
		return Skill{}, fmt.Errorf("SKILL.md must start with a --- frontmatter block")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Skill{}, fmt.Errorf("SKILL.md frontmatter is not closed with ---")
	}
	front := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")

	fields, lists, err := parseFlat(front)
	if err != nil {
		return Skill{}, err
	}
	s := Skill{
		Name:         fields["name"],
		Description:  fields["description"],
		ArgumentHint: fields["argumenthint"],
		Requires:     RequiresNone,
		Body:         strings.TrimSpace(body),
	}
	if s.Name == "" {
		s.Name = dirName
	}
	if s.Name != dirName {
		return Skill{}, fmt.Errorf("name %q must match its folder %q", s.Name, dirName)
	}
	if len(s.Name) > maxNameLen || !skillName.MatchString(s.Name) {
		return Skill{}, fmt.Errorf("name %q must be lowercase letters, digits and single hyphens, at most %d characters", s.Name, maxNameLen)
	}
	if strings.TrimSpace(s.Description) == "" {
		return Skill{}, fmt.Errorf("description is required")
	}
	s.Flags = lists["flags"]
	if v, ok := fields["maxtoolrounds"]; ok {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 0 {
			return Skill{}, fmt.Errorf("maxToolRounds must be a non-negative integer, got %q", v)
		}
		s.MaxToolRounds = n
	}
	if v, ok := fields["requires"]; ok && v != "" {
		if v != RequiresNone && v != RequiresSandbox {
			return Skill{}, fmt.Errorf("requires must be %q or %q, got %q", RequiresNone, RequiresSandbox, v)
		}
		s.Requires = v
	}
	return s, nil
}

// parseFlat reads `key: value` lines. Keys are lower-cased. A key with an
// empty value followed by `- item` lines, or an inline `[a, b]` value, is a
// list. Indented or blank lines that do not belong to a list are ignored.
func parseFlat(front string) (map[string]string, map[string][]string, error) {
	fields := map[string]string{}
	lists := map[string][]string{}
	var listKey string
	for i, line := range strings.Split(front, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if listKey == "" {
				return nil, nil, fmt.Errorf("frontmatter line %d: list item without a key", i+1)
			}
			lists[listKey] = append(lists[listKey], unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))))
			continue
		}
		if line != strings.TrimLeft(line, " \t") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, nil, fmt.Errorf("frontmatter line %d: expected key: value", i+1)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		listKey = ""
		switch {
		case value == "":
			listKey = key
			fields[key] = ""
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			inner := strings.TrimSpace(value[1 : len(value)-1])
			var items []string
			if inner != "" {
				for _, part := range strings.Split(inner, ",") {
					items = append(items, unquote(strings.TrimSpace(part)))
				}
			}
			lists[key] = items
		default:
			fields[key] = unquote(value)
		}
	}
	return fields, lists, nil
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
