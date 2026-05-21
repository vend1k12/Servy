package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vend1k12/servy/internal/app"
	"github.com/vend1k12/servy/internal/config"
	"github.com/vend1k12/servy/internal/doctor"
	"github.com/vend1k12/servy/internal/logging"
	"github.com/vend1k12/servy/internal/modules"
	"github.com/vend1k12/servy/internal/plan"
	"github.com/vend1k12/servy/internal/platform"
	"github.com/vend1k12/servy/internal/runner"
	"github.com/vend1k12/servy/internal/safeops"
	"github.com/vend1k12/servy/internal/system"
	"github.com/vend1k12/servy/internal/update"
)

type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func Execute() error {
	return NewRoot(IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}).Execute()
}

func NewRoot(streams IO) *cobra.Command {
	if streams.In == nil {
		streams.In = os.Stdin
	}
	if streams.Out == nil {
		streams.Out = os.Stdout
	}
	if streams.Err == nil {
		streams.Err = os.Stderr
	}
	root := &cobra.Command{
		Use:           app.BinaryName,
		Short:         "Safe, repeatable Ubuntu/Debian VPS setup",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)
	root.AddCommand(versionCmd(streams), doctorCmd(streams), validateCmd(streams), planCmd(streams), applyCmd(streams), initCmd(streams), statusCmd(streams), logsCmd(streams), moduleCmd(streams), updateCmd(streams), completionCmd(streams), internalCmd())
	return root
}

func versionCmd(streams IO) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print version", Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(streams.Out, "%s %s (%s, %s)\n", app.BinaryName, app.Version, app.Commit, app.Date)
	}}
}

func doctorCmd(streams IO) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{Use: "doctor", Short: "Run read-only host diagnostics", RunE: func(cmd *cobra.Command, args []string) error {
		osInfo, err := platform.Detect()
		if err != nil {
			return err
		}
		checks := doctor.Run(osInfo, system.RealState{})
		if jsonOut {
			return writeDoctorChecksJSON(streams.Out, checks)
		}
		for _, check := range checks {
			fmt.Fprintf(streams.Out, "[%s] %s - %s\n", check.Status, check.Name, check.Details)
			if check.Status != "ok" && check.Fix != "" {
				fmt.Fprintf(streams.Out, "    fix: %s\n", check.Fix)
			}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit diagnostics as JSON")
	return cmd
}

func validateCmd(streams IO) *cobra.Command {
	var path string
	cmd := &cobra.Command{Use: "validate [profile]", Args: cobra.MaximumNArgs(1), Short: "Validate YAML config", RunE: func(cmd *cobra.Command, args []string) error {
		requestedProfile := profileArg(args)
		if _, err := loadConfig(path, requestedProfile); err != nil {
			return err
		}
		fmt.Fprintln(streams.Out, "config ok")
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file (defaults to servy.yml, servy.yaml, .servy.yml)")
	return cmd
}

func planCmd(streams IO) *cobra.Command {
	var path string
	var jsonOut bool
	cmd := &cobra.Command{Use: "plan [profile]", Args: cobra.MaximumNArgs(1), Short: "Build and print execution plan", RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadPlan(path, profileArg(args))
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(streams.Out, p)
		}
		p.Print(streams.Out)
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file (defaults to servy.yml, servy.yaml, .servy.yml)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit plan as JSON")
	return cmd
}

type doctorCheckJSON struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details"`
	Fix     string `json:"fix,omitempty"`
}

func writeDoctorChecksJSON(w io.Writer, checks []doctor.Check) error {
	out := make([]doctorCheckJSON, 0, len(checks))
	for _, check := range checks {
		out = append(out, doctorCheckJSON{Name: check.Name, Status: check.Status, Details: check.Details, Fix: check.Fix})
	}
	return writeJSON(w, out)
}

func writeJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func applyCmd(streams IO) *cobra.Command {
	var path string
	var dryRun bool
	var yes bool
	cmd := &cobra.Command{Use: "apply [profile]", Args: cobra.MaximumNArgs(1), Short: "Apply YAML config", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, cfgPath, osInfo, p, err := loadPlanParts(path, profileArg(args))
		if err != nil {
			return err
		}
		p.Print(streams.Out)
		if dryRun {
			fmt.Fprintln(streams.Out, "dry-run: no changes applied")
			return nil
		}
		if !yes {
			return fmt.Errorf("refusing to apply without --yes after plan review")
		}
		if blockers := p.Blocking(); len(blockers) > 0 {
			return fmt.Errorf("plan contains blocking step %q with status %s; update config confirmations or options", blockers[0].ID, blockers[0].Status)
		}
		log, err := logging.Open(cfg.Runtime.LogDir)
		if err != nil {
			return fmt.Errorf("open log: %w", err)
		}
		defer log.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		results, err := runner.Apply(ctx, p, runner.CommandRunner{})
		for _, res := range results {
			_ = log.Write(logging.Entry{Timestamp: time.Now(), Command: "apply", Profile: cfg.Profile, ConfigPath: cfgPath, OS: osInfo, Result: res})
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(streams.Out, "applied %d steps; log: %s\n", len(results), log.Path)
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file (defaults to servy.yml, servy.yaml, .servy.yml)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show plan without changing the system")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply non-dangerous confirmed plan")
	return cmd
}

func initCmd(streams IO) *cobra.Command {
	var profile, output, preset string
	var apply, custom, force, listPresets, yes bool
	cmd := &cobra.Command{Use: "init", Short: "Create a Servy YAML config", RunE: func(cmd *cobra.Command, args []string) error {
		if listPresets {
			for _, preset := range config.Presets() {
				fmt.Fprintf(streams.Out, "%s\t%s\n", preset.Name, preset.Description)
			}
			return nil
		}

		reader := bufio.NewReader(streams.In)
		var cfg config.Config
		if custom {
			cfg = askCustomConfig(reader, streams.Out)
		} else {
			selected := preset
			if selected == "" {
				selected = profile
			}
			if selected == "" {
				selected = ask(reader, streams.Out, "Preset [base/docker-only/node/custom]", "docker-only")
			}
			if selected == "custom" {
				cfg = askCustomConfig(reader, streams.Out)
			} else {
				var ok bool
				cfg, ok = config.Preset(selected)
				if !ok {
					return fmt.Errorf("unknown preset %q; run `servy init --list-presets`", selected)
				}
			}
		}

		if output == "" {
			output = "servy.yml"
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		if err := config.WriteFile(output, cfg, force); err != nil {
			return err
		}
		fmt.Fprintf(streams.Out, "wrote %s\n", output)
		osInfo, err := platform.Detect()
		if err == nil {
			p := modules.Build(modules.Context{Config: cfg, OS: osInfo, State: system.RealState{}, BinaryPath: executablePath()})
			p.Print(streams.Out)
		} else {
			fmt.Fprintf(streams.Out, "plan preview skipped: %v\n", err)
		}
		if apply || yes {
			fmt.Fprintln(streams.Out, "run `servy apply --config "+output+" --yes` after reviewing the plan")
		}
		return nil
	}}
	cmd.Flags().StringVar(&preset, "preset", "", "config preset to write: base, docker-only, node, or custom")
	cmd.Flags().StringVar(&profile, "profile", "", "deprecated alias for --preset")
	cmd.Flags().StringVar(&output, "output", "servy.yml", "config path to write")
	cmd.Flags().BoolVar(&custom, "custom", false, "ask every configurable module option")
	cmd.Flags().BoolVar(&listPresets, "list-presets", false, "list available config presets")
	cmd.Flags().BoolVar(&apply, "apply", false, "print apply next step after generating config")
	cmd.Flags().BoolVar(&yes, "yes", false, "reserved; init never silently mutates the host")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing output config")
	return cmd
}

func statusCmd(streams IO) *cobra.Command {
	var path string
	cmd := &cobra.Command{Use: "status", Args: cobra.NoArgs, Short: "Show server and configured module state", RunE: func(cmd *cobra.Command, args []string) error {
		osInfo, err := platform.Detect()
		if err != nil {
			return err
		}
		fmt.Fprintf(streams.Out, "host: %s %s (%s, %s)\n", osInfo.ID, osInfo.VersionID, osInfo.DockerCodename(), osInfo.Arch)
		if ok, reason := osInfo.Supported(); ok {
			fmt.Fprintln(streams.Out, "platform: supported")
		} else {
			fmt.Fprintf(streams.Out, "platform: unsupported - %s\n", reason)
		}

		loaded, err := loadConfig(path, "")
		if err != nil {
			var notFound defaultConfigError
			if path == "" && errors.As(err, &notFound) {
				fmt.Fprintf(streams.Out, "config: not found (%s)\n", notFound.Error())
				return nil
			}
			return err
		}
		fmt.Fprintf(streams.Out, "config: %s\n", loaded.Path)
		fmt.Fprintf(streams.Out, "profile: %s\n", loaded.Config.Profile)

		p := modules.Build(modules.Context{Config: loaded.Config, OS: osInfo, State: system.RealState{}, BinaryPath: executablePath()})
		counts := map[plan.Status]int{}
		for _, step := range p.Steps {
			counts[step.Status]++
		}
		fmt.Fprintf(streams.Out, "steps: will_run=%d already_ok=%d will_skip=%d needs_confirmation=%d failed_precondition=%d unsupported=%d\n", counts[plan.WillRun], counts[plan.AlreadyOK], counts[plan.WillSkip], counts[plan.NeedsConfirmation], counts[plan.FailedPrecondition], counts[plan.Unsupported])
		for _, name := range modules.ModuleNames() {
			moduleCounts := map[plan.Status]int{}
			for _, step := range p.Steps {
				if step.Module == name {
					moduleCounts[step.Status]++
				}
			}
			fmt.Fprintf(streams.Out, "module %s: will_run=%d already_ok=%d will_skip=%d needs_confirmation=%d failed_precondition=%d\n", name, moduleCounts[plan.WillRun], moduleCounts[plan.AlreadyOK], moduleCounts[plan.WillSkip], moduleCounts[plan.NeedsConfirmation], moduleCounts[plan.FailedPrecondition])
		}
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file (defaults to servy.yml, servy.yaml, .servy.yml)")
	return cmd
}

func logsCmd(streams IO) *cobra.Command {
	return &cobra.Command{Use: "logs", Short: "Show log location", Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(streams.Out, "/var/log/servy")
	}}
}

func moduleCmd(streams IO) *cobra.Command {
	root := &cobra.Command{Use: "module", Short: "Inspect modules"}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List modules", Run: func(cmd *cobra.Command, args []string) {
		for _, name := range modules.ModuleNames() {
			fmt.Fprintln(streams.Out, name)
		}
	}})
	var configPath string
	status := &cobra.Command{Use: "status <name>", Args: cobra.ExactArgs(1), Short: "Show module plan status", RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadPlan(configPath, "")
		if err != nil {
			return err
		}
		for _, step := range p.Steps {
			if step.Module == args[0] {
				fmt.Fprintf(streams.Out, "[%s] %s\n", step.Status, step.Description)
			}
		}
		return nil
	}}
	status.Flags().StringVar(&configPath, "config", "", "config file (defaults to servy.yml, servy.yaml, .servy.yml)")
	root.AddCommand(status)
	return root
}

func updateCmd(streams IO) *cobra.Command {
	var repo, version, installDir string
	cmd := &cobra.Command{Use: "update", Args: cobra.NoArgs, Short: "Download and install the latest Servy release", RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		res, err := update.Install(ctx, update.Options{Repo: repo, Version: version, CurrentVersion: app.Version, InstallDir: installDir, BinaryName: app.BinaryName})
		if err != nil {
			return err
		}
		if !res.Updated {
			fmt.Fprintf(streams.Out, "%s is already up to date (%s)\n", app.BinaryName, res.Version)
			return nil
		}
		fmt.Fprintf(streams.Out, "updated %s to %s at %s\n", app.BinaryName, res.Version, res.Path)
		return nil
	}}
	cmd.PersistentFlags().StringVar(&repo, "repo", "vend1k12/servy", "GitHub repository owner/name")
	cmd.Flags().StringVar(&version, "version", "", "release tag to install (defaults to latest)")
	cmd.Flags().StringVar(&installDir, "install-dir", "", "directory to install into (defaults to current executable directory)")

	check := &cobra.Command{Use: "check", Args: cobra.NoArgs, Short: "Check whether a newer Servy release exists", RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := update.Check(ctx, update.Options{Repo: repo, CurrentVersion: app.Version})
		if err != nil {
			return err
		}
		if res.UpdateAvailable {
			fmt.Fprintf(streams.Out, "update available: %s -> %s\n", res.CurrentVersion, res.LatestVersion)
			if res.ReleaseURL != "" {
				fmt.Fprintf(streams.Out, "%s\n", res.ReleaseURL)
			}
			return nil
		}
		fmt.Fprintf(streams.Out, "%s is up to date (%s)\n", app.BinaryName, res.LatestVersion)
		return nil
	}}
	cmd.AddCommand(check)
	return cmd
}

func completionCmd(streams IO) *cobra.Command {
	var printScript bool
	var yes bool
	var outputPath string
	root := &cobra.Command{Use: "completion", Short: "Install or print shell completion"}
	bash := &cobra.Command{Use: "bash", Args: cobra.NoArgs, Short: "Install bash completion", RunE: func(cmd *cobra.Command, args []string) error {
		var script bytes.Buffer
		if err := cmd.Root().GenBashCompletion(&script); err != nil {
			return err
		}
		if printScript {
			_, err := streams.Out.Write(script.Bytes())
			return err
		}
		path := outputPath
		if path == "" {
			path = defaultBashCompletionPath()
		}
		if !yes {
			reader := bufio.NewReader(streams.In)
			if !askBool(reader, streams.Out, "Install bash completion to "+path+"?", true) {
				fmt.Fprintln(streams.Out, "completion install cancelled")
				return nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, script.Bytes(), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(streams.Out, "installed bash completion: %s\n", path)
		fmt.Fprintln(streams.Out, "open a new shell or source your bash completion files to activate it")
		return nil
	}}
	bash.Flags().BoolVar(&printScript, "print", false, "print completion script instead of installing")
	bash.Flags().BoolVar(&yes, "yes", false, "install without prompting")
	bash.Flags().StringVar(&outputPath, "output", "", "completion file to write")
	root.AddCommand(bash)
	return root
}

func defaultBashCompletionPath() string {
	if os.Geteuid() == 0 {
		return "/etc/bash_completion.d/servy"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "bash-completion", "completions", "servy")
	}
	return filepath.Join(".", "servy.bash")
}

func internalCmd() *cobra.Command {
	root := &cobra.Command{Use: "internal", Hidden: true}
	var userName, key, line string
	appendKey := &cobra.Command{Use: "append-authorized-key", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		return safeops.AppendAuthorizedKey(userName, key)
	}}
	appendKey.Flags().StringVar(&userName, "user", "", "target user")
	appendKey.Flags().StringVar(&key, "key", "", "SSH public key")
	writeSSH := &cobra.Command{Use: "write-sshd-dropin", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		return safeops.WriteSSHDDropIn(line)
	}}
	writeSSH.Flags().StringVar(&line, "line", "", "single sshd directive")
	root.AddCommand(appendKey, writeSSH)
	return root
}

var defaultConfigPaths = []string{"servy.yml", "servy.yaml", ".servy.yml"}

type loadedConfig struct {
	Config config.Config
	Path   string
}

type defaultConfigError struct {
	paths   []string
	profile string
}

func (e defaultConfigError) Error() string {
	preset := e.profile
	if preset == "" {
		preset = "docker-only"
	}
	return "no config file found (looked for " + strings.Join(e.paths, ", ") + "); run `servy init --preset " + preset + " --output servy.yml` or pass --config <file>"
}

func profileArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func loadPlan(configPath, requestedProfile string) (plan.Plan, error) {
	_, _, _, p, err := loadPlanParts(configPath, requestedProfile)
	return p, err
}

func loadPlanParts(configPath, requestedProfile string) (config.Config, string, platform.Info, plan.Plan, error) {
	loaded, err := loadConfig(configPath, requestedProfile)
	if err != nil {
		return config.Config{}, "", platform.Info{}, plan.Plan{}, err
	}
	osInfo, err := platform.Detect()
	if err != nil {
		return config.Config{}, "", platform.Info{}, plan.Plan{}, err
	}
	p := modules.Build(modules.Context{Config: loaded.Config, OS: osInfo, State: system.RealState{}, BinaryPath: executablePath()})
	return loaded.Config, loaded.Path, osInfo, p, nil
}

func loadConfig(configPath, requestedProfile string) (loadedConfig, error) {
	path := configPath
	if path == "" {
		var found bool
		for _, candidate := range defaultConfigPaths {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				found = true
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				return loadedConfig{}, err
			}
		}
		if !found {
			return loadedConfig{}, defaultConfigError{paths: defaultConfigPaths, profile: requestedProfile}
		}
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return loadedConfig{}, fmt.Errorf("config file %q not found; run `servy init --output servy.yml` or pass an existing --config path", path)
		}
		return loadedConfig{}, err
	}
	if requestedProfile != "" && cfg.Profile != requestedProfile {
		return loadedConfig{}, fmt.Errorf("config profile %q does not match requested profile %q", cfg.Profile, requestedProfile)
	}
	return loadedConfig{Config: cfg, Path: path}, nil
}

func executablePath() string {
	path, err := os.Executable()
	if err != nil || path == "" {
		return app.BinaryName
	}
	return path
}

func ask(r *bufio.Reader, w io.Writer, prompt, def string) string {
	fmt.Fprintf(w, "%s [%s]: ", prompt, def)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func askBool(r *bufio.Reader, w io.Writer, prompt string, def bool) bool {
	defText := "y/N"
	if def {
		defText = "Y/n"
	}
	answer := strings.ToLower(ask(r, w, prompt, defText))
	if answer == "y" || answer == "yes" {
		return true
	}
	if answer == "n" || answer == "no" {
		return false
	}
	return def
}

func askCustomConfig(reader *bufio.Reader, w io.Writer) config.Config {
	dockerEnabled := askBool(reader, w, "Install Docker Engine from the official apt repository?", true)
	nodeEnabled := askBool(reader, w, "Install host-level Node.js tooling with nvm?", false)
	if nodeEnabled {
		dockerEnabled = true
	}

	profile := "base"
	if dockerEnabled {
		profile = "docker-only"
	}
	if nodeEnabled {
		profile = "node"
	}
	cfg := config.Default(profile)
	cfg.Modules.Docker.Enabled = dockerEnabled
	cfg.Modules.Node.Enabled = nodeEnabled
	if !dockerEnabled {
		cfg.Modules.Docker = config.Docker{}
	}
	if !nodeEnabled {
		cfg.Modules.Node = config.Node{}
	}

	deployDefault := dockerEnabled || nodeEnabled
	cfg.Modules.DeployUser.Enabled = askBool(reader, w, "Create or reuse a deploy user?", deployDefault)
	if cfg.Modules.DeployUser.Enabled {
		cfg.Modules.DeployUser.Name = ask(reader, w, "Deploy user name", "deploy")
		cfg.Modules.DeployUser.Sudo = askBool(reader, w, "Allow deploy user passwordless sudo for setup tasks?", true)
		cfg.Modules.DeployUser.Groups = askCSV(reader, w, "Extra deploy user groups, comma-separated", "")
		cfg.Modules.DeployUser.SSHAuthorizedKeys = askCSV(reader, w, "SSH public keys to append, comma-separated", "")
	}

	if dockerEnabled {
		cfg.Modules.Docker.Channel = ask(reader, w, "Docker apt channel", cfg.Modules.Docker.Channel)
		if cfg.Modules.DeployUser.Enabled {
			cfg.Modules.Docker.AddDeployUserToGroup = askBool(reader, w, "Add deploy user to docker group?", false)
			if cfg.Modules.Docker.AddDeployUserToGroup {
				cfg.Confirmations.DockerGroupRootEquivalent = askBool(reader, w, "Confirm docker group is root-equivalent?", false)
			}
		}
	}

	cfg.Modules.Firewall.Enabled = askBool(reader, w, "Enable UFW firewall?", false)
	if cfg.Modules.Firewall.Enabled {
		cfg.Modules.Firewall.SSHPort = askInt(reader, w, "SSH port to allow before enabling UFW", cfg.Modules.Firewall.SSHPort)
		cfg.Modules.Firewall.AllowWeb = askBool(reader, w, "Allow HTTP/HTTPS through UFW?", false)
		cfg.Confirmations.EnableFirewall = askBool(reader, w, "Confirm firewall enablement after SSH allow rule?", false)
	}

	cfg.Modules.Swap.Enabled = askBool(reader, w, "Create swapfile?", false)
	if cfg.Modules.Swap.Enabled {
		cfg.Modules.Swap.Size = ask(reader, w, "Swap size", cfg.Modules.Swap.Size)
		cfg.Modules.Swap.Path = ask(reader, w, "Swap path", cfg.Modules.Swap.Path)
	}

	cfg.Modules.Caddy.Mode = ask(reader, w, "Caddy mode [none/host/check-only]", cfg.Modules.Caddy.Mode)
	if cfg.Modules.Caddy.Mode != "none" {
		cfg.Modules.Caddy.ConfigPath = ask(reader, w, "Existing Caddy config path to document (optional)", cfg.Modules.Caddy.ConfigPath)
	}

	cfg.Modules.Hardening.Fail2Ban = askBool(reader, w, "Install/enable fail2ban?", false)
	cfg.Modules.Hardening.UnattendedUpgrades = askBool(reader, w, "Enable unattended security upgrades?", false)
	cfg.Modules.Hardening.BasicSysctl = askBool(reader, w, "Apply basic sysctl hardening?", false)
	cfg.Modules.Hardening.DisableRootSSHLogin = askBool(reader, w, "Disable root SSH login?", false)
	if cfg.Modules.Hardening.DisableRootSSHLogin {
		cfg.Confirmations.DisableRootSSHLogin = askBool(reader, w, "Confirm root SSH login disablement?", false)
	}
	cfg.Modules.Hardening.DisablePasswordAuth = askBool(reader, w, "Disable SSH password authentication?", false)
	if cfg.Modules.Hardening.DisablePasswordAuth {
		cfg.Confirmations.DisablePasswordAuth = askBool(reader, w, "Confirm SSH password authentication disablement?", false)
	}
	cfg.Modules.Hardening.RestrictSSHUsers = askBool(reader, w, "Restrict SSH users?", false)
	if cfg.Modules.Hardening.RestrictSSHUsers {
		cfg.Confirmations.RestrictSSHUsers = askBool(reader, w, "Confirm SSH user restriction?", false)
	}

	if nodeEnabled {
		defaultUser := cfg.Modules.DeployUser.Name
		if defaultUser == "" {
			defaultUser = "deploy"
		}
		cfg.Modules.Node.User = ask(reader, w, "Node tooling target user", defaultUser)
		cfg.Modules.Node.Version = ask(reader, w, "Node version [lts or numeric]", cfg.Modules.Node.Version)
		cfg.Modules.Node.InstallPNPM = askBool(reader, w, "Install pnpm via Corepack?", true)
		cfg.Modules.Node.InstallBun = askBool(reader, w, "Install Bun?", false)
		cfg.Confirmations.InstallUserTooling = askBool(reader, w, "Confirm official user-level tooling installers?", false)
	}

	cfg.OS.Distributions = askCSV(reader, w, "Restrict OS distributions, comma-separated (optional)", "")
	cfg.OS.Codenames = askCSV(reader, w, "Restrict OS codenames, comma-separated (optional)", "")
	cfg.OS.Architectures = askCSV(reader, w, "Restrict architectures, comma-separated (optional)", "")
	cfg.Runtime.LogDir = ask(reader, w, "Apply log directory", cfg.Runtime.LogDir)
	return cfg
}

func askCSV(reader *bufio.Reader, w io.Writer, prompt, def string) []string {
	value := ask(reader, w, prompt, def)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func askInt(reader *bufio.Reader, w io.Writer, prompt string, def int) int {
	answer := ask(reader, w, prompt, fmt.Sprintf("%d", def))
	var parsed int
	if _, err := fmt.Sscanf(answer, "%d", &parsed); err != nil {
		return def
	}
	return parsed
}
