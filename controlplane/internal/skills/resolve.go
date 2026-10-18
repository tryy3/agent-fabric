package skills

import "sort"

// Layer is the skills found in one source, listed in precedence order from
// lowest (built-in) to highest (assistant) when passed to Resolve.
type Layer struct {
	Source Source
	Skills []Skill
}

// Toggles is the `disabled` name list of one settings layer: plane settings,
// project settings.skills or assistant settings.skills.
type Toggles struct {
	Disabled []string `json:"disabled,omitempty"`
}

// Resolve merges layers (lowest precedence first; a same-named skill replaces
// the one below) and applies the union of the disabled lists, so a skill turned
// off at any level is off. New skills are on by default. The result is sorted
// by name.
func Resolve(layers []Layer, toggles ...Toggles) []Entry {
	disabled := map[string]bool{}
	for _, t := range toggles {
		for _, name := range t.Disabled {
			disabled[name] = true
		}
	}
	byName := map[string]*Entry{}
	for _, layer := range layers {
		for _, s := range layer.Skills {
			s.Source = layer.Source
			if prev, ok := byName[s.Name]; ok {
				overrides := append(append([]Source(nil), prev.Overrides...), prev.Source)
				byName[s.Name] = &Entry{Skill: s, Overrides: overrides}
				continue
			}
			byName[s.Name] = &Entry{Skill: s}
		}
	}
	out := make([]Entry, 0, len(byName))
	for name, e := range byName {
		e.Enabled = !disabled[name]
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// PinEnabled returns the enabled entries in the form pinned at session/new.
func PinEnabled(entries []Entry) []Pinned {
	var pins []Pinned
	for _, e := range entries {
		if !e.Enabled {
			continue
		}
		pins = append(pins, Pinned{Name: e.Name, Description: e.Description, Source: e.Source, Hash: e.Hash})
	}
	return pins
}
