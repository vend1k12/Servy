package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(msg, "no config file found") || !strings.Contains(msg, "servy init --profile base --output servy.yml") {
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

func execute(args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	cmd := NewRoot(IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}
