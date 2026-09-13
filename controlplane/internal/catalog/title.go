package catalog

import "strings"

func AutoTitle(prompt string) string {
	fields := strings.Fields(prompt)
	if len(fields) == 0 {
		return "Untitled"
	}
	if len(fields) > 8 {
		fields = fields[:8]
	}
	return strings.Join(fields, " ")
}
