package deploymentprofile

import "testing"

func TestBuiltinEmberProduction(t *testing.T) {
	registry, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := registry.Lookup("ember-production")
	if !ok {
		t.Fatal("ember-production profile missing")
	}
	if profile.Hardware.Memory.MinimumBytes != 64<<30 {
		t.Fatalf("memory minimum = %d", profile.Hardware.Memory.MinimumBytes)
	}
	if !profile.Hardware.Memory.ECCRequired {
		t.Fatal("ECC must be required")
	}
	if profile.Hardware.CPU.MinimumPhysicalCores != 8 {
		t.Fatalf("core minimum = %d", profile.Hardware.CPU.MinimumPhysicalCores)
	}
	if profile.Model.ArchitectureClass != "moe" {
		t.Fatalf("architecture = %q", profile.Model.ArchitectureClass)
	}
	if profile.Hardware.Accelerator.Required {
		t.Fatal("GPU must remain optional")
	}
}
