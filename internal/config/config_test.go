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

func TestWebAppRequiresTargetUser(t *testing.T) {
	cfg := Default("web-app")
	cfg.Modules.DeployUser.Name = ""
	cfg.Modules.Node.User = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected web-app profile without target user to fail")
	}
}

func TestNodeProfileIsAliasForWebApp(t *testing.T) {
	cfg := Default("node")
	// ApplyProfileDefaults normalises to the canonical profile name.
	if cfg.Profile != "web-app" {
		t.Fatalf("expected node -> web-app normalisation, got %q", cfg.Profile)
	}
	if !cfg.Modules.Node.Enabled || !cfg.Modules.Docker.Enabled {
		t.Fatal("web-app defaults must enable Docker and Node")
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
	want := []string{"base", "docker-only", "web-app", "node"}
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
	// 'node' resolves to a web-app config with profile normalised; test both.
	for _, tc := range []struct {
		name          string
		wantProfile   string
		wantValidates bool
	}{
		{"base", "base", true},
		{"docker-only", "docker-only", true},
		{"web-app", "web-app", true},
		{"node", "web-app", true},
	} {
		cfg, ok := Preset(tc.name)
		if !ok {
			t.Fatalf("Preset(%q) returned ok=false", tc.name)
		}
		if cfg.Profile != tc.wantProfile {
			t.Fatalf("Preset(%q).Profile = %q, want %q", tc.name, cfg.Profile, tc.wantProfile)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Preset(%q).Validate() = %v", tc.name, err)
		}
		if cfg.Confirmations != (Confirmations{}) {
			t.Fatalf("Preset(%q) enabled confirmations: %#v", tc.name, cfg.Confirmations)
		}
	}
}

func TestWebAppPresetIncludesTargetUser(t *testing.T) {
	cfg, ok := Preset("web-app")
	if !ok {
		t.Fatal("expected web-app preset")
	}
	if !cfg.Modules.DeployUser.Enabled {
		t.Fatal("web-app preset must enable deploy user")
	}
	if cfg.Modules.DeployUser.Name == "" {
		t.Fatal("web-app preset must set deploy user name")
	}
	if cfg.Modules.Node.User == "" {
		t.Fatal("web-app preset must set node user")
	}
}

func TestUnknownPresetFails(t *testing.T) {
	if _, ok := Preset("unknown"); ok {
		t.Fatal("expected unknown preset to fail")
	}
}
