package scrub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONStrings runs an inner ContentScrubber on JSON string leaves only.
//
// Hop capture bodies are JSON documents. Scrubbing the raw bytes lets
// prompt-scrub's Windows PathDetector treat JSON escapes like `Draft:\"…"`
// as a drive path (`t:\`), replace them with `«Path_N»`, and leave invalid
// JSON. Decoding first means the scrubber sees real quote characters instead
// of backslash-quote pairs.
type JSONStrings struct {
	Inner ContentScrubber
}

// Scrub walks JSON string values when content is a JSON object/array; otherwise
// it scrubs the whole blob as plain text.
func (j JSONStrings) Scrub(ctx context.Context, content string) (string, error) {
	inner := j.Inner
	if inner == nil {
		inner = Identity{}
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content, nil
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return inner.Scrub(ctx, content)
	}

	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		// Not parseable JSON — fall back to whole-blob scrub.
		return inner.Scrub(ctx, content)
	}
	scrubbed, err := scrubJSONValue(ctx, inner, v)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(scrubbed); err != nil {
		return "", err
	}
	// Encoder always appends a trailing newline; hop bodies are single-line JSON.
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func scrubJSONValue(ctx context.Context, inner ContentScrubber, v any) (any, error) {
	switch x := v.(type) {
	case string:
		if x == "" {
			return "", nil
		}
		out, err := inner.Scrub(ctx, x)
		if err != nil {
			return nil, err
		}
		// Same fail-closed rule as Pipeline.ScrubBody: empty scrub of non-empty
		// input means the scrubber no-op'd / failed.
		if out == "" {
			return nil, fmt.Errorf("scrub: empty output for non-empty JSON string")
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			sv, err := scrubJSONValue(ctx, inner, val)
			if err != nil {
				return nil, err
			}
			out[k] = sv
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			sv, err := scrubJSONValue(ctx, inner, val)
			if err != nil {
				return nil, err
			}
			out[i] = sv
		}
		return out, nil
	default:
		// bool, nil, json.Number (via UseNumber), float64
		return v, nil
	}
}
