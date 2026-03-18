package cmd

import (
	"bytes"
	"io"
	"testing"
)

func TestWorkspaceStatusCommandUsesRunnerOutput(t *testing.T) {
	runner := &stubWorkspaceStatusCommandRunner{
		run: func(stdout io.Writer) error {
			_, err := io.WriteString(stdout, "alpha - git: ✅ remote: ✅ clean: ✅ synced: ✅\n")
			return err
		},
	}

	cmd := newWorkspaceStatusCmd(runner)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	want := "alpha - git: ✅ remote: ✅ clean: ✅ synced: ✅\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	if runner.runCount != 1 {
		t.Fatalf("runCount = %d, want %d", runner.runCount, 1)
	}
}

func TestWorkspaceCommandIncludesStatusSubcommand(t *testing.T) {
	cmd := newWorkspaceCmd(newWorkspaceStatusTestInitializer(t, t.TempDir()))

	found := false
	for _, subcommand := range cmd.Commands() {
		if subcommand.Name() == "status" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected workspace command to include the status subcommand")
	}
}

type stubWorkspaceStatusCommandRunner struct {
	run      func(stdout io.Writer) error
	runCount int
}

func (s *stubWorkspaceStatusCommandRunner) Run(stdout io.Writer) error {
	s.runCount++
	if s.run == nil {
		return nil
	}

	return s.run(stdout)
}
