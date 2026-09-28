package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestDecodeInferenceValid(t *testing.T) {
	raw := json.RawMessage(`{
		"temperature": 0.8,
		"topP": 0.9,
		"maxTokens": 512,
		"reasoningEffort": "medium",
		"topK": 20,
		"minP": 0.05,
		"repetitionPenalty": 1.1,
		"presencePenalty": 0.2,
		"enableThinking": true
	}`)
	inf, err := catalog.DecodeInference(raw)
	if err != nil {
		t.Fatalf("DecodeInference: %v", err)
	}
	if inf.Temperature == nil || *inf.Temperature != 0.8 {
		t.Fatalf("temperature = %v", inf.Temperature)
	}
	if inf.ReasoningEffort == nil || *inf.ReasoningEffort != "medium" {
		t.Fatalf("reasoningEffort = %v", inf.ReasoningEffort)
	}
}

func TestDecodeInferenceRejectsUnknownKey(t *testing.T) {
	_, err := catalog.DecodeInference(json.RawMessage(`{"foo":1}`))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("err = %v, want unknown key", err)
	}
}

func TestDecodeInferenceRejectsBadRanges(t *testing.T) {
	_, err := catalog.DecodeInference(json.RawMessage(`{"temperature": 3}`))
	if err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Fatalf("err = %v, want temperature range", err)
	}
}

func TestMergeSettingsInference(t *testing.T) {
	got, err := catalog.MergeSettings(
		json.RawMessage(`{}`),
		json.RawMessage(`{"inference":{"temperature":0.5,"maxTokens":1024}}`),
	)
	if err != nil {
		t.Fatalf("MergeSettings: %v", err)
	}
	inf, err := catalog.InferenceFromSettings(got)
	if err != nil {
		t.Fatalf("InferenceFromSettings: %v", err)
	}
	if inf.Temperature == nil || *inf.Temperature != 0.5 {
		t.Fatalf("temperature = %v", inf.Temperature)
	}
	if inf.MaxTokens == nil || *inf.MaxTokens != 1024 {
		t.Fatalf("maxTokens = %v", inf.MaxTokens)
	}
}

func TestMergeSettingsRejectsBadInference(t *testing.T) {
	_, err := catalog.MergeSettings(
		json.RawMessage(`{}`),
		json.RawMessage(`{"inference":{"temperature":"hot"}}`),
	)
	if err == nil {
		t.Fatal("expected error")
	}
}
