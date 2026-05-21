package runner

import (
	"context"
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
	p := plan.Plan{Steps: []plan.Step{{ID: "danger", Status: plan.NeedsConfirmation}}}
	fr := &fakeRunner{}
	if _, err := Apply(context.Background(), p, fr); err == nil {
		t.Fatal("expected blocking plan to fail")
	}
	if len(fr.ran) != 0 {
		t.Fatalf("blocking plan ran steps: %#v", fr.ran)
	}
}
