package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vend1k12/servy/internal/config"
	"github.com/vend1k12/servy/internal/doctor"
	"github.com/vend1k12/servy/internal/plan"
)

const baseConfig = `schemaVersion: v1
profile: base
modules:
  firewall:
    sshPort: 22
runtime:
  logDir: /var/log/servy
`

func TestValidateUsesDefaultConfigAndProfileArgument(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("servy.yml", []byte(baseConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	out, _, err := execute("validate", "base")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "config ok" {
		t.Fatalf("output = %q", out)
	}
}

func TestValidateMissingDefaultConfigExplainsInit(t *testing.T) {
	t.Chdir(t.TempDir())

	_, _, err := execute("validate", "base")
	if err == nil {
		t.Fatal("expected missing config to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no config file found") || !strings.Contains(msg, "servy init --preset base --output servy.yml") {
		t.Fatalf("unexpected error: %s", msg)
	}
	if strings.Contains(msg, "--config is required") {
		t.Fatalf("old error leaked: %s", msg)
	}
}

func TestValidateProfileMismatchFails(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := strings.Replace(baseConfig, "profile: base", "profile: docker-only", 1)
	if err := os.WriteFile("servy.yml", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := execute("validate", "base")
	if err == nil {
		t.Fatal("expected profile mismatch to fail")
	}
	if !strings.Contains(err.Error(), "does not match requested profile") {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestCompletionBashInstallsScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servy")
	out, _, err := execute("completion", "bash", "--yes", "--output", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "installed bash completion") {
		t.Fatalf("output = %q", out)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("servy")) || !bytes.Contains(b, []byte("complete")) {
		t.Fatalf("completion script was not written correctly")
	}
}

func TestPlanJSONShapeUsesLowerCamelKeys(t *testing.T) {
	p := plan.Plan{Profile: "base", Steps: []plan.Step{{
		ID:                  "base.packages",
		Module:              "base",
		Description:         "install base packages",
		Status:              plan.WillRun,
		Rationale:           "required for server bootstrap",
		Command:             []string{"apt-get", "install", "git"},
		Dangerous:           true,
		Confirmation:        "confirmations.example",
		RollbackHint:        "remove packages manually if needed",
		RedactCommandInLogs: true,
	}}}

	var out bytes.Buffer
	if err := writeJSON(&out, p); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Profile:") || strings.Contains(out.String(), "[will_run]") {
		t.Fatalf("json output contains human plan text: %q", out.String())
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["profile"] != "base" {
		t.Fatalf("profile = %v", got["profile"])
	}
	if _, ok := got["Profile"]; ok {
		t.Fatal("plan JSON leaked Go field name Profile")
	}
	steps, ok := got["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("steps = %#v", got["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("step = %#v", steps[0])
	}
	for _, key := range []string{"id", "module", "description", "status", "rationale", "command", "dangerous", "confirmation", "rollbackHint", "redactCommandInLogs"} {
		if _, ok := step[key]; !ok {
			t.Fatalf("missing step key %q in %#v", key, step)
		}
	}
	for _, key := range []string{"ID", "Module", "RollbackHint", "RedactCommandInLogs"} {
		if _, ok := step[key]; ok {
			t.Fatalf("step JSON leaked Go field name %s", key)
		}
	}
	command, ok := step["command"].([]any)
	if !ok || len(command) != 3 || command[0] != "apt-get" {
		t.Fatalf("command = %#v", step["command"])
	}
}

func TestDoctorJSONShapeUsesLowerCamelKeys(t *testing.T) {
	checks := []doctor.Check{
		{Name: "os", Status: "ok", Details: "ubuntu 24.04", Fix: ""},
		{Name: "apt", Status: "warn", Details: "missing", Fix: "install apt"},
	}

	var out bytes.Buffer
	if err := writeDoctorChecksJSON(&out, checks); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "[ok]") || strings.Contains(out.String(), "fix:") {
		t.Fatalf("json output contains human doctor text: %q", out.String())
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("checks = %#v", got)
	}
	if got[0]["name"] != "os" || got[0]["status"] != "ok" || got[0]["details"] != "ubuntu 24.04" {
		t.Fatalf("first check = %#v", got[0])
	}
	if _, ok := got[0]["Name"]; ok {
		t.Fatal("doctor JSON leaked Go field name Name")
	}
	if _, ok := got[0]["fix"]; ok {
		t.Fatalf("empty fix should be omitted: %#v", got[0])
	}
	if got[1]["fix"] != "install apt" {
		t.Fatalf("second check fix = %#v", got[1]["fix"])
	}
}

func TestInitListsPresets(t *testing.T) {
	out, _, err := execute("init", "--list-presets")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base", "docker-only", "node"} {
		if !strings.Contains(out, name) {
			t.Fatalf("preset list missing %q: %s", name, out)
		}
	}
}

func TestInitPresetWritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servy.yml")
	out, _, err := execute("init", "--preset", "node", "--output", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "wrote "+path) {
		t.Fatalf("output = %q", out)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "web-app" || !cfg.Modules.Node.Enabled || cfg.Modules.Node.User == "" {
		t.Fatalf("node preset config = %#v", cfg)
	}
}

func TestInitCustomWritesSelectedOptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servy.yml")
	input := strings.Join([]string{
		"y",                            // Docker
		"n",                            // Node
		"y",                            // deploy user
		"ops",                          // deploy user name
		"y",                            // sudo
		"adm,www-data",                 // groups
		"",                             // SSH keys
		"stable",                       // Docker channel
		"y",                            // docker group
		"n",                            // docker group confirmation
		"y",                            // firewall
		"2222",                         // SSH port
		"y",                            // allow web
		"n",                            // firewall confirmation
		"y",                            // swap
		"4G",                           // swap size
		"/var/lib/servy/swapfile.swap", // swap path
		"check-only",                   // Caddy mode
		"/etc/caddy/Caddyfile",         // Caddy config path
		"y",                            // fail2ban
		"y",                            // unattended upgrades
		"n",                            // sysctl
		"n",                            // disable root SSH
		"n",                            // disable password auth
		"n",                            // restrict SSH users
		"ubuntu,debian",                // OS distributions
		"noble,bookworm",               // OS codenames
		"amd64,arm64",                  // arches
		"/tmp/servy-logs",              // log dir
	}, "\n") + "\n"
	_, _, err := executeWithInput(input, "init", "--custom", "--output", path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "docker-only" || !cfg.Modules.Docker.Enabled || !cfg.Modules.Firewall.Enabled || !cfg.Modules.Swap.Enabled {
		t.Fatalf("custom config missed selected modules: %#v", cfg)
	}
	if cfg.Modules.DeployUser.Name != "ops" || len(cfg.Modules.DeployUser.Groups) != 2 || cfg.Modules.Firewall.SSHPort != 2222 {
		t.Fatalf("custom config missed selected values: %#v", cfg)
	}
	if cfg.Confirmations.EnableFirewall || cfg.Confirmations.DockerGroupRootEquivalent {
		t.Fatalf("custom config should keep declined confirmations false: %#v", cfg.Confirmations)
	}
	if len(cfg.OS.Distributions) != 2 || cfg.Runtime.LogDir != "/tmp/servy-logs" {
		t.Fatalf("custom config missed constraints/runtime: %#v", cfg)
	}
}

func execute(args ...string) (string, string, error) {
	return executeWithInput("", args...)
}

func executeWithInput(input string, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := NewRoot(IO{In: strings.NewReader(input), Out: &out, Err: &errOut})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}
