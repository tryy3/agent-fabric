package skills

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Invocation is a parsed `/name args` prompt: the skill, the flags the user
// gave and the rest of the text as the question.
type Invocation struct {
	Name     string            `json:"name"`
	Flags    map[string]string `json:"flags,omitempty"`
	Question string            `json:"question"`
}

// flagCheck validates the value of a known flag; an unknown declared flag takes
// any non-empty value.
type flagCheck func(value string) error

var knownFlags = map[string]flagCheck{
	"depth": oneOf("quick", "normal", "deep"),
	"save":  oneOf("project", "scratch"),
	"rounds": func(v string) error {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			return fmt.Errorf("must be a whole number of at least 1")
		}
		return nil
	},
	"time": func(v string) error {
		if d, err := time.ParseDuration(v); err != nil || d <= 0 {
			return fmt.Errorf("must be a positive duration such as 10m")
		}
		return nil
	},
}

func oneOf(allowed ...string) flagCheck {
	return func(v string) error {
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s", strings.Join(allowed, ", "))
	}
}

// SplitCommand returns the skill name and the text after it when text starts
// with `/name` followed by whitespace or the end. It does not check that the
// skill exists.
func SplitCommand(text string) (name, rest string, ok bool) {
	text = strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	body := text[1:]
	end := strings.IndexAny(body, " \t\r\n")
	if end < 0 {
		end = len(body)
	}
	name = body[:end]
	if name == "" || !skillName.MatchString(name) {
		return "", "", false
	}
	return name, strings.TrimSpace(body[end:]), true
}

// ParseInvocation reads leading `--flag value` or `--flag=value` pairs from
// args and returns them with the remaining question. Only flags in declared are
// accepted; a bad or unknown flag returns an error written for the user.
func ParseInvocation(name, args string, declared []string) (Invocation, error) {
	inv := Invocation{Name: name}
	allowed := map[string]bool{}
	for _, f := range declared {
		allowed[f] = true
	}
	fields := strings.Fields(args)
	i := 0
	for i < len(fields) && strings.HasPrefix(fields[i], "--") {
		flag := strings.TrimPrefix(fields[i], "--")
		value := ""
		hasValue := false
		if k, v, found := strings.Cut(flag, "="); found {
			flag, value, hasValue = k, v, true
		}
		if !allowed[flag] {
			return Invocation{}, fmt.Errorf("/%s has no flag --%s%s", name, flag, allowedHint(declared))
		}
		i++
		if !hasValue {
			if i >= len(fields) {
				return Invocation{}, fmt.Errorf("/%s: --%s needs a value", name, flag)
			}
			value = fields[i]
			i++
		}
		if value == "" {
			return Invocation{}, fmt.Errorf("/%s: --%s needs a value", name, flag)
		}
		if check, known := knownFlags[flag]; known {
			if err := check(value); err != nil {
				return Invocation{}, fmt.Errorf("/%s: --%s %q %w", name, flag, value, err)
			}
		}
		if inv.Flags == nil {
			inv.Flags = map[string]string{}
		}
		inv.Flags[flag] = value
	}
	inv.Question = strings.TrimSpace(strings.Join(fields[i:], " "))
	return inv, nil
}

func allowedHint(declared []string) string {
	if len(declared) == 0 {
		return " (it takes no flags)"
	}
	flags := make([]string, len(declared))
	for i, f := range declared {
		flags[i] = "--" + f
	}
	return " (allowed: " + strings.Join(flags, ", ") + ")"
}

// RenderInvocation builds the user-side context for a `/name` prompt: the
// skill's instructions and the parsed flags, followed by the question.
func RenderInvocation(inv Invocation, source Source, hash, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<skill_invocation name=%q source=%q hash=%q>\n", inv.Name, source, hash)
	b.WriteString("<skill_instructions>\n")
	b.WriteString(body)
	b.WriteString("\n</skill_instructions>\n")
	if len(inv.Flags) > 0 {
		b.WriteString("<skill_flags>\n")
		keys := make([]string, 0, len(inv.Flags))
		for k := range inv.Flags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %s\n", k, inv.Flags[k])
		}
		b.WriteString("</skill_flags>\n")
	}
	b.WriteString("</skill_invocation>\n\n")
	if inv.Question == "" {
		b.WriteString("(The user gave no question after the command.)")
	} else {
		b.WriteString(inv.Question)
	}
	return b.String()
}
