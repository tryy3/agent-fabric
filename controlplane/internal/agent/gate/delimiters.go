package gate

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

// A scorer prompt mixes the user's trusted text (gate instructions, questions
// the user wrote) with text the agent controls (arguments, referenced files).
// Every block delimiter carries a nonce drawn per evaluation, which the agent
// never sees, and text the agent controls is made inert so it cannot imitate
// a delimiter and pass itself off as the user's.

// newNonce returns a fresh random delimiter nonce.
func newNonce() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func nonceOf(gen func() string) string {
	if gen != nil {
		return gen()
	}
	return newNonce()
}

var (
	delimiterTag = regexp.MustCompile(`(?i)<(/?)(gate_instructions|user_questions|arguments)`)
	// delimiterLine matches a line that starts like a file header or the end
	// of the files.
	delimiterLine = regexp.MustCompile(`(?m)^([ \t]*)---`)
)

// inert escapes anything in s that looks like a prompt block delimiter.
func inert(s string) string {
	s = delimiterTag.ReplaceAllString(s, "&lt;$1$2")
	return delimiterLine.ReplaceAllString(s, "${1}- - -")
}

// inertJSON renders tool arguments with every key and string value made
// inert. Arguments that are not one JSON value are made inert as text.
func inertJSON(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return inert(string(raw))
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(inertValue(v)); err != nil {
		return inert(string(raw))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func inertValue(v any) any {
	switch x := v.(type) {
	case string:
		return inert(x)
	case []any:
		for i := range x {
			x[i] = inertValue(x[i])
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[inert(k)] = inertValue(e)
		}
		return out
	}
	return v
}
