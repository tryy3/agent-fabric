package agent

import (
	"testing"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

func TestGateRequestCarriesPinnedInstructions(t *testing.T) {
	req := gateRequest("run_command", nil, sandbox.OpenOptions{}, nil, gateContext{Instructions: "no force pushes"})
	if req.GateInstructions != "no force pushes" {
		t.Fatalf("GateInstructions = %q", req.GateInstructions)
	}
}
