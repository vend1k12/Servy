package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = "v1"

type Config struct {
	SchemaVersion string        `yaml:"schemaVersion"`
	Profile       string        `yaml:"profile"`
	OS            OSConstraints `yaml:"os,omitempty"`
	Modules       Modules       `yaml:"modules"`
	Users         Users         `yaml:"users,omitempty"`
	Runtime       Runtime       `yaml:"runtime,omitempty"`
	Confirmations Confirmations `yaml:"confirmations,omitempty"`
}

type OSConstraints struct {
	Distributions []string `yaml:"distributions,omitempty"`
	Codenames     []string `yaml:"codenames,omitempty"`
	Architectures []string `yaml:"architectures,omitempty"`
}

type Modules struct {
	Docker     Docker     `yaml:"docker,omitempty"`
	Caddy      Caddy      `yaml:"caddy,omitempty"`
	Firewall   Firewall   `yaml:"firewall,omitempty"`
	Swap       Swap       `yaml:"swap,omitempty"`
	DeployUser DeployUser `yaml:"deployUser,omitempty"`
	Hardening  Hardening  `yaml:"hardening,omitempty"`
	Node       Node       `yaml:"node,omitempty"`
}

type Docker struct {
	Enabled              bool   `yaml:"enabled,omitempty"`
	AddDeployUserToGroup bool   `yaml:"addDeployUserToGroup,omitempty"`
	Channel              string `yaml:"channel,omitempty"`
}

type Caddy struct {
	Mode       string `yaml:"mode,omitempty"`
	ConfigPath string `yaml:"configPath,omitempty"`
}

type Firewall struct {
	Enabled  bool `yaml:"enabled,omitempty"`
	SSHPort  int  `yaml:"sshPort,omitempty"`
	AllowWeb bool `yaml:"allowWeb,omitempty"`
}

type Swap struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Size    string `yaml:"size,omitempty"`
	Path    string `yaml:"path,omitempty"`
}

type DeployUser struct {
	Enabled           bool     `yaml:"enabled,omitempty"`
	Name              string   `yaml:"name,omitempty"`
	SSHAuthorizedKeys []string `yaml:"sshAuthorizedKeys,omitempty"`
	Sudo              bool     `yaml:"sudo,omitempty"`
	Groups            []string `yaml:"groups,omitempty"`
}

type Hardening struct {
	Fail2Ban            bool `yaml:"fail2ban,omitempty"`
	UnattendedUpgrades  bool `yaml:"unattendedUpgrades,omitempty"`
	BasicSysctl         bool `yaml:"basicSysctl,omitempty"`
	DisableRootSSHLogin bool `yaml:"disableRootSSHLogin,omitempty"`
	DisablePasswordAuth bool `yaml:"disablePasswordAuth,omitempty"`
	RestrictSSHUsers    bool `yaml:"restrictSSHUsers,omitempty"`
}

type Node struct {
	Enabled     bool   `yaml:"enabled,omitempty"`
	User        string `yaml:"user,omitempty"`
	Version     string `yaml:"version,omitempty"`
	InstallPNPM bool   `yaml:"installPNPM,omitempty"`
	InstallBun  bool   `yaml:"installBun,omitempty"`
}

type Users struct {
	Deploy string `yaml:"deploy,omitempty"`
}

type Runtime struct {
	LogDir string `yaml:"logDir,omitempty"`
}

type Confirmations struct {
	EnableFirewall            bool `yaml:"enableFirewall,omitempty"`
	DisableRootSSHLogin       bool `yaml:"disableRootSSHLogin,omitempty"`
	DisablePasswordAuth       bool `yaml:"disablePasswordAuth,omitempty"`
	RestrictSSHUsers          bool `yaml:"restrictSSHUsers,omitempty"`
	DockerGroupRootEquivalent bool `yaml:"dockerGroupRootEquivalent,omitempty"`
	InstallUserTooling        bool `yaml:"installUserTooling,omitempty"`
}

func Default(profile string) Config {
	if profile == "" {
		profile = "docker-only"
	}
	cfg := Config{SchemaVersion: SchemaVersion, Profile: profile}
	cfg.Modules.Docker.Channel = "stable"
	cfg.Modules.Caddy.Mode = "none"
	cfg.Modules.Firewall.SSHPort = 22
	cfg.Modules.Swap.Size = "2G"
	cfg.Modules.Swap.Path = "/swapfile"
	cfg.Modules.Node.Version = "lts"
	cfg.Runtime.LogDir = "/var/log/servy"
	ApplyProfileDefaults(&cfg)
	return cfg
}

func ApplyProfileDefaults(cfg *Config) {
	switch cfg.Profile {
	case "base":
	case "docker-only":
		cfg.Modules.Docker.Enabled = true
	case "node":
		cfg.Modules.Docker.Enabled = true
		cfg.Modules.Node.Enabled = true
		cfg.Modules.Node.InstallPNPM = true
	}
	if cfg.Modules.Firewall.SSHPort == 0 {
		cfg.Modules.Firewall.SSHPort = 22
	}
	if cfg.Modules.Swap.Size == "" {
		cfg.Modules.Swap.Size = "2G"
	}
	if cfg.Modules.Swap.Path == "" {
		cfg.Modules.Swap.Path = "/swapfile"
	}
	if cfg.Runtime.LogDir == "" {
		cfg.Runtime.LogDir = "/var/log/servy"
	}
}

func LoadFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	return Load(f)
}

func Load(r io.Reader) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("invalid YAML config: %w", err)
	}
	ApplyProfileDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func WriteFile(path string, cfg Config, force bool) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schemaVersion must be %q", SchemaVersion)
	}
	switch c.Profile {
	case "base", "docker-only", "node":
	default:
		return fmt.Errorf("unsupported profile %q", c.Profile)
	}
	if c.Modules.Caddy.Mode == "" {
		c.Modules.Caddy.Mode = "none"
	}
	switch c.Modules.Caddy.Mode {
	case "none", "host", "check-only":
	default:
		return fmt.Errorf("modules.caddy.mode must be one of: none, host, check-only")
	}
	if c.Modules.Firewall.Enabled && (c.Modules.Firewall.SSHPort < 1 || c.Modules.Firewall.SSHPort > 65535) {
		return errors.New("modules.firewall.sshPort must be between 1 and 65535")
	}
	if c.Modules.Swap.Enabled {
		if !regexp.MustCompile(`^[1-9][0-9]*[KMGTP]?$`).MatchString(c.Modules.Swap.Size) {
			return errors.New("modules.swap.size must be a positive size like 2G")
		}
		if !safeSwapPath(c.Modules.Swap.Path) {
			return errors.New("modules.swap.path must be /swapfile or under /var/lib/servy/")
		}
	}
	if c.Modules.DeployUser.Enabled {
		if c.Modules.DeployUser.Name == "" {
			return errors.New("modules.deployUser.name is required when deploy user is enabled")
		}
		if !validAccountName(c.Modules.DeployUser.Name) {
			return errors.New("modules.deployUser.name must be a safe POSIX account name")
		}
		for _, group := range c.Modules.DeployUser.Groups {
			if !validAccountName(group) {
				return fmt.Errorf("modules.deployUser.groups contains unsafe group name %q", group)
			}
		}
	}
	if c.Modules.Node.Enabled {
		user := c.Modules.Node.User
		if user == "" {
			user = c.Modules.DeployUser.Name
		}
		if user == "" {
			return errors.New("modules.node.user or modules.deployUser.name is required when node tooling is enabled")
		}
		if user == "root" || !validAccountName(user) {
			return errors.New("modules.node.user must be a non-root safe POSIX account name")
		}
		if c.Modules.Node.Version != "" && c.Modules.Node.Version != "lts" && !regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`).MatchString(c.Modules.Node.Version) {
			return errors.New("modules.node.version must be lts or a numeric Node version")
		}
	}
	return nil
}

func validAccountName(name string) bool {
	return regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`).MatchString(name)
}

func safeSwapPath(path string) bool {
	if path == "/swapfile" {
		return true
	}
	return regexp.MustCompile(`^/var/lib/servy/[a-zA-Z0-9._-]+\.swap$`).MatchString(path)
}
