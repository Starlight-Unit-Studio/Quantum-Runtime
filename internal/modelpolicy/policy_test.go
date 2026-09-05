package modelpolicy

import "testing"

func TestBuiltinPoliciesValidate(t *testing.T) {
	policies, err := Builtin()
	if err != nil {
		t.Fatalf("builtin policies rejected: %v", err)
	}
	if len(policies) == 0 {
		t.Fatal("expected at least one policy")
	}
}

func TestGemmaPolicyMatchesProfileDependentCoreUI(t *testing.T) {
	matched := Match(MustBuiltin(), "gemma4", "profile_dependent", "ollama-adapter")
	if len(matched) != 1 || matched[0].ID != "gemma4-ollama-turin-minimal" {
		t.Fatalf("unexpected policies: %#v", matched)
	}
}

func TestGemmaPolicyDoesNotLeakToOtherFamilies(t *testing.T) {
	if matched := Match(MustBuiltin(), "qwen", "30b-a3b", "ollama-adapter"); len(matched) != 0 {
		t.Fatalf("unexpected cross-family match: %#v", matched)
	}
}
