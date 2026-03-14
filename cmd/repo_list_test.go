package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type stubRepoListCommandRunner struct {
	runErr error
	output string
}

func (s *stubRepoListCommandRunner) Run(_ context.Context, writer io.Writer) error {
	if s.output != "" {
		if _, err := io.WriteString(writer, s.output); err != nil {
			return err
		}
	}

	return s.runErr
}

func TestRepoListCommandRunsRunner(t *testing.T) {
	runner := &stubRepoListCommandRunner{
		output: "repo output\n",
	}

	cmd := newRepoListCmd(runner)
	var output bytes.Buffer

	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if output.String() != "repo output\n" {
		t.Fatalf("output = %q, want %q", output.String(), "repo output\n")
	}
}

func TestRepoListCommandReturnsRunnerError(t *testing.T) {
	runner := &stubRepoListCommandRunner{
		runErr: errors.New("boom"),
	}

	cmd := newRepoListCmd(runner)
	var output bytes.Buffer

	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(nil)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected Execute() to return an error")
	}

	if err.Error() != "boom" {
		t.Fatalf("error = %q, want %q", err.Error(), "boom")
	}
	if output.String() != "" {
		t.Fatalf("expected command output to be empty on error, got %q", output.String())
	}
}
