package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsUnknownKeys(t *testing.T) {
	_, err := Load(strings.NewReader(`schemaVersion: v1
profile: docker-only
unexpected: true
`))
	if err == nil {
		t.Fatal("expected unknown key to fail validation")
	}
}

func TestDefaultDockerOnlyEnablesDocker(t *testing.T) {
	cfg := Default("docker-only")
	if cfg.Profile != "docker-only" {
		t.Fatalf("profile = %q", cfg.Profile)
	}
	if !cfg.Modules.Docker.Enabled {
		t.Fatal("docker-only profile must enable Docker")
	}
	if cfg.Modules.Node.Enabled {
		t.Fatal("docker-only profile must not enable host Node tooling")
	}
}

func TestNodeRequiresTargetUser(t *testing.T) {
	cfg := Default("node")
	cfg.Modules.DeployUser.Name = ""
	cfg.Modules.Node.User = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected node profile without target user to fail")
	}
}

func TestSwapValidation(t *testing.T) {
	cfg := Default("base")
	cfg.Modules.Swap.Enabled = true
	cfg.Modules.Swap.Size = "0G"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid swap size to fail")
	}
}

func TestPresetsHaveStableNamesAndDescriptions(t *testing.T) {
	presets := Presets()
	want := []string{"base", "docker-only", "node"}
	if len(presets) != len(want) {
		t.Fatalf("len(Presets()) = %d, want %d", len(presets), len(want))
	}
	for i, name := range want {
		if presets[i].Name != name {
			t.Fatalf("Presets()[%d].Name = %q, want %q", i, presets[i].Name, name)
		}
		if presets[i].Description == "" {
			t.Fatalf("Presets()[%d].Description is empty", i)
		}
	}
	presets[0].Name = "changed"
	if Presets()[0].Name != "base" {
		t.Fatal("Presets must not expose mutable package state")
	}
}

func TestPresetConfigsValidate(t *testing.T) {
	for _, name := range []string{"base", "docker-only", "node"} {
		cfg, ok := Preset(name)
		if !ok {
			t.Fatalf("Preset(%q) returned ok=false", name)
		}
		if cfg.Profile != name {
			t.Fatalf("Preset(%q).Profile = %q", name, cfg.Profile)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Preset(%q).Validate() = %v", name, err)
		}
		if cfg.Confirmations != (Confirmations{}) {
			t.Fatalf("Preset(%q) enabled confirmations: %#v", name, cfg.Confirmations)
		}
	}
}

func TestNodePresetIncludesTargetUser(t *testing.T) {
	cfg, ok := Preset("node")
	if !ok {
		t.Fatal("expected node preset")
	}
	if !cfg.Modules.DeployUser.Enabled {
		t.Fatal("node preset must enable deploy user")
	}
	if cfg.Modules.DeployUser.Name == "" {
		t.Fatal("node preset must set deploy user name")
	}
	if cfg.Modules.Node.User == "" {
		t.Fatal("node preset must set node user")
	}
}

func TestUnknownPresetFails(t *testing.T) {
	if _, ok := Preset("unknown"); ok {
		t.Fatal("expected unknown preset to fail")
	}
}
