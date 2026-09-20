package sandboxcore

import "testing"

func TestValidateContainerName(t *testing.T) {
	if err := ValidateContainerName("agent-fabric-container-proj_abc"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContainerName("shared-build-box"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContainerName(""); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := ValidateContainerName("-leading-dash"); err == nil {
		t.Fatal("expected invalid name")
	}
	if err := ValidateContainerName("has space"); err == nil {
		t.Fatal("expected invalid name")
	}
}

func TestValidateVolumeName(t *testing.T) {
	if err := ValidateVolumeName("agent-fabric-vol-proj_abc"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVolumeName("agent-fabric.proj.proj_abc"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVolumeName(""); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := ValidateVolumeName("has space"); err == nil {
		t.Fatal("expected invalid name")
	}
}
