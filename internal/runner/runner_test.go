package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vend1k12/servy/internal/plan"
)

type fakeRunner struct{ ran []string }

func (f *fakeRunner) Run(ctx context.Context, step plan.Step) Result {
	f.ran = append(f.ran, step.ID)
	return Result{Step: step, StartedAt: time.Now(), EndedAt: time.Now(), ExitCode: 0}
}

func TestApplyRunsOnlyWillRunSteps(t *testing.T) {
	p := plan.Plan{Steps: []plan.Step{
		{ID: "skip", Status: plan.WillSkip},
		{ID: "ok", Status: plan.AlreadyOK},
		{ID: "run", Status: plan.WillRun, Command: []string{"true"}},
	}}
	fr := &fakeRunner{}
	if _, err := Apply(context.Background(), p, fr); err != nil {
		t.Fatal(err)
	}
	if len(fr.ran) != 1 || fr.ran[0] != "run" {
		t.Fatalf("ran = %#v", fr.ran)
	}
}

func TestApplyRefusesBlockingSteps(t *testing.T) {
	p := plan.Plan{Steps: []plan.Step{
		{ID: "danger", Status: plan.NeedsConfirmation, Description: "enable firewall", Confirmation: "confirmations.enableFirewall", Rationale: "SSH lockout risk"},
		{ID: "other", Status: plan.FailedPrecondition, Description: "not applicable"},
	}}
	fr := &fakeRunner{}
	_, err := Apply(context.Background(), p, fr)
	if err == nil {
		t.Fatal("expected blocking plan to fail")
	}
	var be *plan.BlockingError
	if !errors.As(err, &be) {
		t.Fatalf("expected BlockingError, got %T: %v", err, err)
	}
	if len(be.Blockers) != 2 {
		t.Fatalf("expected 2 blockers, got %d", len(be.Blockers))
	}
	msg := err.Error()
	for _, want := range []string{"danger", "confirmations.enableFirewall", "SSH lockout risk", "not applicable"} {
		if !strings.Contains(msg, want) {
			t.Errorf("blocking error missing %q; got:\n%s", want, msg)
		}
	}
	if len(fr.ran) != 0 {
		t.Fatalf("blocking plan ran steps: %#v", fr.ran)
	}
}
