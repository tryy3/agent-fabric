package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Inference holds optional generation knobs stored under agent settings.inference.
// Omitted fields mean "do not send" (upstream defaults).
type Inference struct {
	Temperature       *float64 `json:"temperature,omitempty"`
	TopP              *float64 `json:"topP,omitempty"`
	MaxTokens         *int     `json:"maxTokens,omitempty"`
	ReasoningEffort   *string  `json:"reasoningEffort,omitempty"`
	TopK              *int     `json:"topK,omitempty"`
	MinP              *float64 `json:"minP,omitempty"`
	RepetitionPenalty *float64 `json:"repetitionPenalty,omitempty"`
	PresencePenalty   *float64 `json:"presencePenalty,omitempty"`
	FrequencyPenalty  *float64 `json:"frequencyPenalty,omitempty"`
	EnableThinking    *bool    `json:"enableThinking,omitempty"`
	ThinkingType      *string  `json:"thinkingType,omitempty"`
}

var knownInferenceKeys = map[string]struct{}{
	"temperature":       {},
	"topP":              {},
	"maxTokens":         {},
	"reasoningEffort":   {},
	"topK":              {},
	"minP":              {},
	"repetitionPenalty": {},
	"presencePenalty":   {},
	"frequencyPenalty":  {},
	"enableThinking":    {},
	"thinkingType":      {},
}

var knownReasoningEfforts = map[string]struct{}{
	"none":    {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
	"max":     {},
}

var knownThinkingTypes = map[string]struct{}{
	"disabled": {},
	"enabled":  {},
	"adaptive": {},
}

// InferenceFromSettings extracts settings.inference from an agent/project settings blob.
func InferenceFromSettings(settings json.RawMessage) (Inference, error) {
	if len(settings) == 0 || bytes.Equal(bytes.TrimSpace(settings), []byte("null")) {
		return Inference{}, nil
	}
	var bag map[string]json.RawMessage
	if err := json.Unmarshal(settings, &bag); err != nil {
		return Inference{}, fmt.Errorf("decode settings: %w", err)
	}
	raw, ok := bag["inference"]
	if !ok || isJSONNull(raw) {
		return Inference{}, nil
	}
	return DecodeInference(raw)
}

// DecodeInference validates and decodes an inference JSON object.
func DecodeInference(raw json.RawMessage) (Inference, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Inference{}, nil
	}
	if !isJSONObject(raw) {
		return Inference{}, fmt.Errorf("inference must be an object")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return Inference{}, fmt.Errorf("inference must be an object")
	}
	for key := range keys {
		if _, ok := knownInferenceKeys[key]; !ok {
			return Inference{}, fmt.Errorf("inference: unknown key %q", key)
		}
	}
	var inf Inference
	if err := json.Unmarshal(raw, &inf); err != nil {
		return Inference{}, fmt.Errorf("decode inference: %w", err)
	}
	if err := validateInference(inf); err != nil {
		return Inference{}, err
	}
	return inf, nil
}

func validateInference(inf Inference) error {
	if inf.Temperature != nil && (*inf.Temperature < 0 || *inf.Temperature > 2) {
		return fmt.Errorf("inference.temperature must be between 0 and 2")
	}
	if inf.TopP != nil && (*inf.TopP < 0 || *inf.TopP > 1) {
		return fmt.Errorf("inference.topP must be between 0 and 1")
	}
	if inf.MaxTokens != nil && *inf.MaxTokens < 1 {
		return fmt.Errorf("inference.maxTokens must be >= 1")
	}
	if inf.ReasoningEffort != nil {
		if _, ok := knownReasoningEfforts[*inf.ReasoningEffort]; !ok {
			return fmt.Errorf("inference.reasoningEffort must be one of none, minimal, low, medium, high, xhigh, max")
		}
	}
	if inf.TopK != nil && (*inf.TopK < -1 || *inf.TopK > 1000) {
		return fmt.Errorf("inference.topK must be between -1 and 1000")
	}
	if inf.MinP != nil && (*inf.MinP < 0 || *inf.MinP > 1) {
		return fmt.Errorf("inference.minP must be between 0 and 1")
	}
	if inf.RepetitionPenalty != nil && (*inf.RepetitionPenalty < 1 || *inf.RepetitionPenalty > 2) {
		return fmt.Errorf("inference.repetitionPenalty must be between 1 and 2")
	}
	if inf.PresencePenalty != nil && (*inf.PresencePenalty < -2 || *inf.PresencePenalty > 2) {
		return fmt.Errorf("inference.presencePenalty must be between -2 and 2")
	}
	if inf.FrequencyPenalty != nil && (*inf.FrequencyPenalty < -2 || *inf.FrequencyPenalty > 2) {
		return fmt.Errorf("inference.frequencyPenalty must be between -2 and 2")
	}
	if inf.ThinkingType != nil {
		if _, ok := knownThinkingTypes[*inf.ThinkingType]; !ok {
			return fmt.Errorf("inference.thinkingType must be one of disabled, enabled, adaptive")
		}
	}
	return nil
}
