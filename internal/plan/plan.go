package plan

import (
	"fmt"
	"io"
	"strings"
)

type Status string

const (
	WillRun            Status = "will_run"
	AlreadyOK          Status = "already_ok"
	WillSkip           Status = "will_skip"
	NeedsConfirmation  Status = "needs_confirmation"
	Dangerous          Status = "dangerous"
	Unsupported        Status = "unsupported"
	FailedPrecondition Status = "failed_precondition"
)

type Step struct {
	ID                  string   `json:"id"`
	Module              string   `json:"module"`
	Description         string   `json:"description"`
	Status              Status   `json:"status"`
	Rationale           string   `json:"rationale,omitempty"`
	Command             []string `json:"command,omitempty"`
	Dangerous           bool     `json:"dangerous,omitempty"`
	Confirmation        string   `json:"confirmation,omitempty"`
	RollbackHint        string   `json:"rollbackHint,omitempty"`
	RedactCommandInLogs bool     `json:"redactCommandInLogs,omitempty"`
}

type Plan struct {
	Profile string
	Steps   []Step
}

func (p *Plan) Add(step Step) {
	p.Steps = append(p.Steps, step)
}

func (p Plan) Blocking() []Step {
	var out []Step
	for _, step := range p.Steps {
		if step.Status == Unsupported || step.Status == FailedPrecondition || step.Status == Dangerous || step.Status == NeedsConfirmation {
			out = append(out, step)
		}
	}
	return out
}

func (p Plan) Print(w io.Writer) {
	fmt.Fprintf(w, "Profile: %s\n", p.Profile)
	for i, step := range p.Steps {
		fmt.Fprintf(w, "%2d. [%s] %s: %s\n", i+1, step.Status, step.Module, step.Description)
		if len(step.Command) > 0 {
			fmt.Fprintf(w, "    command: %s\n", shellQuote(step.Command))
		}
		if step.Rationale != "" {
			fmt.Fprintf(w, "    why: %s\n", step.Rationale)
		}
		if step.RollbackHint != "" {
			fmt.Fprintf(w, "    recovery: %s\n", step.RollbackHint)
		}
	}
}

func shellQuote(args []string) string {
	var b strings.Builder
	for i, arg := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		if arg == "" || strings.ContainsAny(arg, " \t\n'\"$`\\") {
			b.WriteByte('\'')
			b.WriteString(strings.ReplaceAll(arg, "'", "'\\''"))
			b.WriteByte('\'')
			continue
		}
		b.WriteString(arg)
	}
	return b.String()
}
