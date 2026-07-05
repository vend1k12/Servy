package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_MissingReturnsEmptyManifest(t *testing.T) {
	dir := t.TempDir()
	m, err := Load(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion=%q, want %q", m.SchemaVersion, SchemaVersion)
	}
	if len(m.Applies) != 0 {
		t.Errorf("Applies=%v, want empty", m.Applies)
	}
}

func TestSave_Load_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	want := &Manifest{
		SchemaVersion: SchemaVersion,
		Applies: []Apply{{
			Timestamp:    time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
			ServyVersion: "v0.1.0-preview",
			Profile:      "docker-only",
			ConfigPath:   "/etc/servy.yml",
			Modules: map[string]ModuleRecord{
				"docker": {
					AptPackagesInstalled: []string{"docker-ce", "docker-ce-cli"},
					AptLists:             []string{"/etc/apt/sources.list.d/docker.sources"},
					AptKeyrings:          []string{"/etc/apt/keyrings/docker.asc"},
					ServicesEnabled:      []string{"docker"},
				},
				"hardening": {
					SysctlDropIns:   []string{"/etc/sysctl.d/99-servy.conf"},
					SSHDDropInLines: []string{"PermitRootLogin no"},
				},
			},
		}},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode=%o, want 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stage file survived: err=%v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Applies) != 1 {
		t.Fatalf("Applies=%d, want 1", len(got.Applies))
	}
	if !got.Applies[0].Timestamp.Equal(want.Applies[0].Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", got.Applies[0].Timestamp, want.Applies[0].Timestamp)
	}
	if got.Applies[0].Profile != "docker-only" {
		t.Errorf("Profile=%q", got.Applies[0].Profile)
	}
	docker := got.LatestModule("docker")
	if len(docker.AptPackagesInstalled) != 2 || docker.AptPackagesInstalled[0] != "docker-ce" {
		t.Errorf("docker.AptPackagesInstalled=%v", docker.AptPackagesInstalled)
	}
	if docker.ServicesEnabled[0] != "docker" {
		t.Errorf("docker.ServicesEnabled=%v", docker.ServicesEnabled)
	}
}

func TestLoad_RejectsFutureSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	body := []byte(`{"schemaVersion":"999","applies":[]}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for future schemaVersion")
	}
}

func TestModuleRecord_Empty(t *testing.T) {
	if !(ModuleRecord{}).Empty() {
		t.Fatal("zero ModuleRecord must be Empty")
	}
	if (ModuleRecord{AptPackagesInstalled: []string{"gh"}}).Empty() {
		t.Fatal("record with packages must not be Empty")
	}
	if (ModuleRecord{SwapFilePath: "/swapfile"}).Empty() {
		t.Fatal("record with swap must not be Empty")
	}
	if (ModuleRecord{GroupsJoined: map[string][]string{"docker": {"deploy"}}}).Empty() {
		t.Fatal("record with groupsJoined must not be Empty")
	}
}

func TestManifest_LatestModuleReturnsZeroForUnknown(t *testing.T) {
	m := &Manifest{Applies: []Apply{{Modules: map[string]ModuleRecord{"docker": {AptKeyrings: []string{"/etc/apt/keyrings/docker.asc"}}}}}}
	if !m.LatestModule("caddy").Empty() {
		t.Fatal("unknown module must return Empty")
	}
	if m.LatestModule("docker").AptKeyrings[0] != "/etc/apt/keyrings/docker.asc" {
		t.Fatal("docker lookup mismatch")
	}
}

func TestManifest_LatestNilOnEmpty(t *testing.T) {
	if (&Manifest{}).Latest() != nil {
		t.Fatal("empty manifest Latest must be nil")
	}
	var m *Manifest
	if m.Latest() != nil {
		t.Fatal("nil manifest Latest must be nil")
	}
	if !m.LatestModule("docker").Empty() {
		t.Fatal("nil manifest LatestModule must be Empty")
	}
}

func TestSave_RejectsNil(t *testing.T) {
	if err := Save(filepath.Join(t.TempDir(), "m.json"), nil); err == nil {
		t.Fatal("Save(nil) must error")
	}
}

func TestManifest_JSONShape(t *testing.T) {
	// Lock the on-disk shape so an accidental struct rename is caught by a
	// unit test rather than a broken production revert. Fields elided when
	// empty must stay elided.
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		Applies: []Apply{{
			Timestamp: time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
			Modules: map[string]ModuleRecord{
				"swap": {SwapFilePath: "/swapfile", FstabLine: "/swapfile none swap sw 0 0"},
			},
		}},
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{
		`"schemaVersion": "1"`,
		`"swapFilePath": "/swapfile"`,
		`"fstabLine": "/swapfile none swap sw 0 0"`,
		`"timestamp": "2026-07-05T12:00:00Z"`,
	} {
		if !contains(got, want) {
			t.Errorf("json missing %q; got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		`"aptPackagesInstalled"`,
		`"aptLists"`,
		`"servicesEnabled"`,
		`"sshdDropInLines"`,
	} {
		if contains(got, unwanted) {
			t.Errorf("empty field %q must be omitted; got:\n%s", unwanted, got)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
