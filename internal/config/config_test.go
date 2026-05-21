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
