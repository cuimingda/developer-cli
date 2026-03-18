package cmd

import (
	"bytes"
	"io"
	"testing"
)

func TestWorkspacePushCommandUsesRunnerOutput(t *testing.T) {
	runner := &stubWorkspacePushCommandRunner{
		run: func(stdout io.Writer) error {
			_, err := io.WriteString(stdout, "alpha - push: ✅\n")
			return err
		},
	}

	cmd := newWorkspacePushCmd(runner)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	want := "alpha - push: ✅\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	if runner.runCount != 1 {
		t.Fatalf("runCount = %d, want %d", runner.runCount, 1)
	}
}

func TestWorkspaceCommandIncludesPushSubcommand(t *testing.T) {
	cmd := newWorkspaceCmd(newWorkspaceStatusTestInitializer(t, t.TempDir()))

	found := false
	for _, subcommand := range cmd.Commands() {
		if subcommand.Name() == "push" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected workspace command to include the push subcommand")
	}
}

type stubWorkspacePushCommandRunner struct {
	run      func(stdout io.Writer) error
	runCount int
}

func (s *stubWorkspacePushCommandRunner) Run(stdout io.Writer) error {
	s.runCount++
	if s.run == nil {
		return nil
	}

	return s.run(stdout)
}
