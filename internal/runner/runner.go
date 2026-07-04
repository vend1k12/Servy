package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	"strings"
	"time"

	"github.com/vend1k12/servy/internal/plan"
	"github.com/vend1k12/servy/internal/safepath"
)

const maxOutputBytes = 1 << 20

type Result struct {
	Step      plan.Step
	StartedAt time.Time
	EndedAt   time.Time
	Stdout    string
	Stderr    string
	ExitCode  int
	Err       error
}

type Runner interface {
	Run(context.Context, plan.Step) Result
}

type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, step plan.Step) Result {
	res := Result{Step: step, StartedAt: time.Now(), ExitCode: 0}
	defer func() { res.EndedAt = time.Now() }()
	if len(step.Command) == 0 {
		res.Err = fmt.Errorf("step %s has no command", step.ID)
		res.ExitCode = -1
		return res
	}
	argv0, err := safepath.LookPath(step.Command[0])
	if err != nil {
		res.Err = err
		res.ExitCode = -1
		return res
	}
	cmd := exec.CommandContext(ctx, argv0, step.Command[1:]...)
	cmd.Env = safepath.Env()
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	if err != nil {
		res.Err = err
		res.ExitCode = 1
		if exit, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exit.ExitCode()
		}
	}
	return res
}

func Apply(ctx context.Context, p plan.Plan, r Runner) ([]Result, error) {
	if blockers := p.Blocking(); len(blockers) > 0 {
		return nil, &plan.BlockingError{Blockers: blockers}
	}
	var results []Result
	for _, step := range p.Steps {
		if step.Status != plan.WillRun {
			continue
		}
		res := r.Run(ctx, step)
		results = append(results, res)
		if res.Err != nil {
			return results, stepError(res)
		}
	}
	return results, nil
}

func stepError(res Result) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s failed with exit code %d", res.Step.Description, res.ExitCode)
	if res.Stderr != "" {
		fmt.Fprintf(&b, ": %s", strings.TrimSpace(res.Stderr))
	}
	if res.Step.RollbackHint != "" {
		fmt.Fprintf(&b, "\nSuggested recovery: %s", res.Step.RollbackHint)
	}
	return fmt.Errorf("%s", b.String())
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := maxOutputBytes - b.Buffer.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		_, _ = io.WriteString(&b.Buffer, "\n<output truncated>\n")
		return len(p), nil
	}
	_, err := b.Buffer.Write(p)
	return len(p), err
}
