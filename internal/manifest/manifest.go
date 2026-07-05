// Package manifest records Servy-owned side effects to /var/lib/servy/manifest.json.
//
// Every successful `apply` appends a record describing what that run added:
// apt packages installed, apt keyring files, apt list files, systemd services
// enabled, sshd/sysctl drop-ins, swapfile paths, and /etc/fstab lines.
// `servy revert <module>` reads this file and undoes only the entries Servy
// owns.
//
// Format is intentionally boring: JSON with a schemaVersion. Older manifests
// stay readable; when the schema grows, bump SchemaVersion and add a migration
// path in Load. `Save` is atomic (write-to-tempfile + rename same-fs).
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion pins the on-disk shape. Bump when adding new fields would
// break older readers.
const SchemaVersion = "1"

// DefaultPath is where apply writes and revert reads the manifest by default.
const DefaultPath = "/var/lib/servy/manifest.json"

// Manifest is the top-level record. Multiple applies accumulate as separate
// entries in Applies; revert walks in reverse order so the latest state is
// undone first.
type Manifest struct {
	SchemaVersion string  `json:"schemaVersion"`
	Applies       []Apply `json:"applies"`
}

// Apply captures one successful `servy apply` invocation.
type Apply struct {
	Timestamp    time.Time                `json:"timestamp"`
	ServyVersion string                   `json:"servyVersion,omitempty"`
	Profile      string                   `json:"profile,omitempty"`
	ConfigPath   string                   `json:"configPath,omitempty"`
	Modules      map[string]ModuleRecord  `json:"modules"`
}

// ModuleRecord is the union of every side-effect kind a single module may
// have produced. Empty slices are elided in JSON via omitempty; a module
// that produced nothing does not appear in Apply.Modules.
type ModuleRecord struct {
	// AptPackagesInstalled is the exact list Servy asked apt to install.
	// May include packages that were already present (dpkg-query gating is
	// per-plan, not per-record); revert must reconcile against dpkg state.
	AptPackagesInstalled []string `json:"aptPackagesInstalled,omitempty"`
	// AptLists is the absolute path of every /etc/apt/sources.list.d/*.list
	// or *.sources file Servy authored.
	AptLists []string `json:"aptLists,omitempty"`
	// AptKeyrings is the absolute path of every keyring file Servy installed.
	AptKeyrings []string `json:"aptKeyrings,omitempty"`
	// ServicesEnabled is systemd units Servy ran `systemctl enable --now` on.
	ServicesEnabled []string `json:"servicesEnabled,omitempty"`
	// SysctlDropIns is absolute paths under /etc/sysctl.d/ Servy wrote.
	SysctlDropIns []string `json:"sysctlDropIns,omitempty"`
	// SSHDDropInLines is the exact directive line(s) Servy appended to
	// /etc/ssh/sshd_config.d/99-servy-hardening.conf. Revert removes only
	// these lines from the file (and deletes the file if empty).
	SSHDDropInLines []string `json:"sshdDropInLines,omitempty"`
	// SwapFilePath is a swapfile path Servy allocated.
	SwapFilePath string `json:"swapFilePath,omitempty"`
	// FstabLine is the exact line Servy appended to /etc/fstab, whitespace
	// preserved so revert can match byte-for-byte.
	FstabLine string `json:"fstabLine,omitempty"`
	// UsersCreated is deploy-user names Servy `useradd`-created. Revert
	// requires --force to remove them.
	UsersCreated []string `json:"usersCreated,omitempty"`
	// GroupsJoined records `usermod -aG <group> <user>` invocations
	// (group -> users).
	GroupsJoined map[string][]string `json:"groupsJoined,omitempty"`
}

// Empty reports whether the record carries no side effects.
func (r ModuleRecord) Empty() bool {
	return len(r.AptPackagesInstalled) == 0 &&
		len(r.AptLists) == 0 &&
		len(r.AptKeyrings) == 0 &&
		len(r.ServicesEnabled) == 0 &&
		len(r.SysctlDropIns) == 0 &&
		len(r.SSHDDropInLines) == 0 &&
		r.SwapFilePath == "" &&
		r.FstabLine == "" &&
		len(r.UsersCreated) == 0 &&
		len(r.GroupsJoined) == 0
}

// Load reads a manifest from path. A missing file returns an empty manifest
// with the current SchemaVersion pre-filled, so callers can treat first-apply
// and subsequent-apply the same way.
func Load(path string) (*Manifest, error) {
	f, err := os.Open(path) //nolint:gosec // path is a fixed system location, never user input.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Manifest{SchemaVersion: SchemaVersion}, nil
		}
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, 8<<20)) // 8 MiB ceiling
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported manifest schemaVersion %q (want %q); do not run revert against a manifest from a newer Servy",
			m.SchemaVersion, SchemaVersion)
	}
	return &m, nil
}

// Save atomically writes m to path with mode 0600. Parents are created with
// mode 0755; nothing above /var/lib is touched.
func Save(path string, m *Manifest) error {
	if m == nil {
		return errors.New("nil manifest")
	}
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	// Write to a sibling temp file then rename so a crash never leaves a
	// half-written manifest at path. The manifest is 0600 because it enumerates
	// exactly which files Servy authored; useful reconnaissance if leaked.
	stage := path + ".tmp"
	if err := os.WriteFile(stage, append(body, '\n'), 0o600); err != nil {
		return fmt.Errorf("stage manifest: %w", err)
	}
	if err := os.Rename(stage, path); err != nil {
		_ = os.Remove(stage)
		return fmt.Errorf("install manifest at %s: %w", path, err)
	}
	return nil
}

// Latest returns the last recorded apply's module map, or nil if the manifest
// is empty. Revert-by-module reads from Latest; --all reverse iterates Applies.
func (m *Manifest) Latest() map[string]ModuleRecord {
	if m == nil || len(m.Applies) == 0 {
		return nil
	}
	return m.Applies[len(m.Applies)-1].Modules
}

// LatestModule returns the record for a single module in the latest apply,
// or an empty record when the module never ran.
func (m *Manifest) LatestModule(name string) ModuleRecord {
	latest := m.Latest()
	if latest == nil {
		return ModuleRecord{}
	}
	return latest[name]
}
