package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
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
	root.AddCommand(versionCmd(streams), doctorCmd(streams), validateCmd(streams), planCmd(streams), applyCmd(streams), initCmd(streams), statusCmd(streams), logsCmd(streams), moduleCmd(streams), internalCmd())
	return root
}

func versionCmd(streams IO) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print version", Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(streams.Out, "%s %s (%s, %s)\n", app.BinaryName, app.Version, app.Commit, app.Date)
	}}
}

func doctorCmd(streams IO) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Run read-only host diagnostics", RunE: func(cmd *cobra.Command, args []string) error {
		osInfo, err := platform.Detect()
		if err != nil {
			return err
		}
		for _, check := range doctor.Run(osInfo, system.RealState{}) {
			fmt.Fprintf(streams.Out, "[%s] %s - %s\n", check.Status, check.Name, check.Details)
		}
		return nil
	}}
}

func validateCmd(streams IO) *cobra.Command {
	var path string
	cmd := &cobra.Command{Use: "validate --config <file>", Short: "Validate YAML config", RunE: func(cmd *cobra.Command, args []string) error {
		if path == "" {
			return fmt.Errorf("--config is required")
		}
		if _, err := config.LoadFile(path); err != nil {
			return err
		}
		fmt.Fprintln(streams.Out, "config ok")
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file")
	return cmd
}

func planCmd(streams IO) *cobra.Command {
	var path string
	cmd := &cobra.Command{Use: "plan --config <file>", Short: "Build and print execution plan", RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadPlan(path)
		if err != nil {
			return err
		}
		p.Print(streams.Out)
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file")
	return cmd
}

func applyCmd(streams IO) *cobra.Command {
	var path string
	var dryRun bool
	var yes bool
	cmd := &cobra.Command{Use: "apply --config <file>", Short: "Apply YAML config", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, osInfo, p, err := loadPlanParts(path)
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
			_ = log.Write(logging.Entry{Timestamp: time.Now(), Command: "apply", Profile: cfg.Profile, ConfigPath: path, OS: osInfo, Result: res})
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(streams.Out, "applied %d steps; log: %s\n", len(results), log.Path)
		return nil
	}}
	cmd.Flags().StringVar(&path, "config", "", "config file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show plan without changing the system")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply non-dangerous confirmed plan")
	return cmd
}

func initCmd(streams IO) *cobra.Command {
	var profile, output string
	var apply, yes, force bool
	cmd := &cobra.Command{Use: "init", Short: "Interactive config wizard", RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(streams.In)
		if profile == "" {
			profile = ask(reader, streams.Out, "Profile [base/docker-only/node]", "docker-only")
		}
		cfg := config.Default(profile)
		if cfg.Profile == "node" {
			cfg.Modules.DeployUser.Enabled = askBool(reader, streams.Out, "Create/use deploy user for node tooling?", true)
			if cfg.Modules.DeployUser.Enabled {
				cfg.Modules.DeployUser.Name = ask(reader, streams.Out, "Deploy user", "deploy")
			}
			cfg.Modules.Node.User = cfg.Modules.DeployUser.Name
		}
		cfg.Modules.Firewall.Enabled = askBool(reader, streams.Out, "Enable UFW firewall?", false)
		if cfg.Modules.Firewall.Enabled {
			cfg.Confirmations.EnableFirewall = askBool(reader, streams.Out, "Confirm enabling firewall after allowing SSH?", false)
		}
		cfg.Modules.Swap.Enabled = askBool(reader, streams.Out, "Create swapfile?", false)
		if cfg.Modules.Swap.Enabled {
			cfg.Modules.Swap.Size = ask(reader, streams.Out, "Swap size", cfg.Modules.Swap.Size)
		}
		if cfg.Profile != "base" {
			cfg.Modules.Caddy.Mode = ask(reader, streams.Out, "Caddy mode [none/host/check-only]", "none")
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
		if apply && yes {
			fmt.Fprintln(streams.Out, "run `servy apply --config "+output+" --yes` after reviewing the plan")
		}
		return nil
	}}
	cmd.Flags().StringVar(&profile, "profile", "", "profile: base, docker-only, node")
	cmd.Flags().StringVar(&output, "output", "servy.yml", "config path to write")
	cmd.Flags().BoolVar(&apply, "apply", false, "print apply next step after generating config")
	cmd.Flags().BoolVar(&yes, "yes", false, "reserved; init never silently mutates the host")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing output config")
	return cmd
}

func statusCmd(streams IO) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show read-only status summary", RunE: func(cmd *cobra.Command, args []string) error {
		return doctorCmd(streams).RunE(cmd, args)
	}}
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
		p, err := loadPlan(configPath)
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
	status.Flags().StringVar(&configPath, "config", "", "config file")
	root.AddCommand(status)
	return root
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

func loadPlan(configPath string) (plan.Plan, error) {
	_, _, p, err := loadPlanParts(configPath)
	return p, err
}

func loadPlanParts(configPath string) (config.Config, platform.Info, plan.Plan, error) {
	if configPath == "" {
		return config.Config{}, platform.Info{}, plan.Plan{}, fmt.Errorf("--config is required")
	}
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return config.Config{}, platform.Info{}, plan.Plan{}, err
	}
	osInfo, err := platform.Detect()
	if err != nil {
		return config.Config{}, platform.Info{}, plan.Plan{}, err
	}
	p := modules.Build(modules.Context{Config: cfg, OS: osInfo, State: system.RealState{}, BinaryPath: executablePath()})
	return cfg, osInfo, p, nil
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
