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
	Profile string `json:"profile"`
	Steps   []Step `json:"steps"`
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

// BlockingError is returned when a plan cannot be applied because one or more
// steps have a blocking status. It exposes every blocker so operators see the
// full picture in one message, not just the first hit.
type BlockingError struct {
	Blockers []Step
}

func (e *BlockingError) Error() string {
	if len(e.Blockers) == 0 {
		return "plan has blocking steps"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "cannot apply plan: %d blocking step(s)\n", len(e.Blockers))
	for _, s := range e.Blockers {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", s.Status, s.ID, s.Description)
		if s.Confirmation != "" {
			fmt.Fprintf(&b, "      set `%s: true` in your config to allow this step\n", s.Confirmation)
		}
		if s.Rationale != "" {
			fmt.Fprintf(&b, "      why: %s\n", s.Rationale)
		}
		if s.RollbackHint != "" {
			fmt.Fprintf(&b, "      recovery: %s\n", s.RollbackHint)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
