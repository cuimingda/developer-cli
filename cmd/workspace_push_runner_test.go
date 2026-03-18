package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspacePushRunnerRunPushesEligibleWorkspaceProjects(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:openai/alpha.git\n")
	createWorkspaceProject(t, workspaceRoot, "beta", `[remote "origin"]`+"\n\turl = git@github.com:openai/beta.git\n")
	createWorkspaceProject(t, workspaceRoot, "epsilon", `[remote "origin"]`+"\n\turl = git@github.com:openai/epsilon.git\n")
	createWorkspaceProject(t, workspaceRoot, "gamma", `[remote "origin"]`+"\n\turl = git@github.com:openai/gamma.git\n")
	createWorkspaceProject(t, workspaceRoot, "zeta", `[core]`+"\n\trepositoryformatversion = 0\n")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "delta"), 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}

	initializer := newWorkspaceStatusTestInitializer(t, workspaceRoot)
	gitCalls := []string{}
	runner := &WorkspacePushRunner{
		configInitializer: initializer,
		runGit: func(projectPath string, args ...string) ([]byte, error) {
			call := filepath.Base(projectPath) + ":" + strings.Join(args, " ")
			gitCalls = append(gitCalls, call)

			switch call {
			case "alpha:status --porcelain":
				return []byte(""), nil
			case "alpha:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return []byte("origin/main\n"), nil
			case "alpha:rev-list --left-right --count @{upstream}...HEAD":
				return []byte("0\t1\n"), nil
			case "beta:status --porcelain":
				return []byte(""), nil
			case "beta:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return nil, errors.New("no upstream")
			case "epsilon:status --porcelain":
				return []byte(""), nil
			case "epsilon:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return []byte("origin/main\n"), nil
			case "epsilon:rev-list --left-right --count @{upstream}...HEAD":
				return []byte("0\t0\n"), nil
			case "gamma:status --porcelain":
				return []byte(" M README.md\n"), nil
			case "zeta:status --porcelain":
				return []byte(""), nil
			case "alpha:push":
				return []byte(""), nil
			case "beta:push --set-upstream origin HEAD":
				return []byte(""), nil
			default:
				t.Fatalf("unexpected git invocation: %s", call)
				return nil, errors.New("unexpected git invocation")
			}
		},
	}

	var output bytes.Buffer
	if err := runner.Run(&output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	wantOutput := strings.Join([]string{
		"alpha - push: ✅",
		"beta - push: ✅",
		"Summary: eligible=2, pushed=2, failed=0",
		"",
	}, "\n")
	if output.String() != wantOutput {
		t.Fatalf("output = %q, want %q", output.String(), wantOutput)
	}

	wantGitCalls := []string{
		"alpha:status --porcelain",
		"alpha:rev-parse --abbrev-ref --symbolic-full-name @{upstream}",
		"alpha:rev-list --left-right --count @{upstream}...HEAD",
		"beta:status --porcelain",
		"beta:rev-parse --abbrev-ref --symbolic-full-name @{upstream}",
		"epsilon:status --porcelain",
		"epsilon:rev-parse --abbrev-ref --symbolic-full-name @{upstream}",
		"epsilon:rev-list --left-right --count @{upstream}...HEAD",
		"gamma:status --porcelain",
		"zeta:status --porcelain",
		"alpha:rev-parse --abbrev-ref --symbolic-full-name @{upstream}",
		"alpha:push",
		"beta:rev-parse --abbrev-ref --symbolic-full-name @{upstream}",
		"beta:push --set-upstream origin HEAD",
	}
	if !reflect.DeepEqual(gitCalls, wantGitCalls) {
		t.Fatalf("git calls = %#v, want %#v", gitCalls, wantGitCalls)
	}
}

func TestWorkspacePushRunnerRunContinuesAfterPushFailures(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:openai/alpha.git\n")
	createWorkspaceProject(t, workspaceRoot, "beta", `[remote "origin"]`+"\n\turl = git@github.com:openai/beta.git\n")

	initializer := newWorkspaceStatusTestInitializer(t, workspaceRoot)
	runner := &WorkspacePushRunner{
		configInitializer: initializer,
		runGit: func(projectPath string, args ...string) ([]byte, error) {
			call := filepath.Base(projectPath) + ":" + strings.Join(args, " ")

			switch call {
			case "alpha:status --porcelain":
				return []byte(""), nil
			case "alpha:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return []byte("origin/main\n"), nil
			case "alpha:rev-list --left-right --count @{upstream}...HEAD":
				return []byte("0\t1\n"), nil
			case "beta:status --porcelain":
				return []byte(""), nil
			case "beta:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return []byte("origin/main\n"), nil
			case "beta:rev-list --left-right --count @{upstream}...HEAD":
				return []byte("0\t2\n"), nil
			case "alpha:push":
				return []byte("error: failed to push some refs to 'origin'\n"), errors.New("push failed")
			case "beta:push":
				return []byte(""), nil
			default:
				t.Fatalf("unexpected git invocation: %s", call)
				return nil, errors.New("unexpected git invocation")
			}
		},
	}

	var output bytes.Buffer
	err := runner.Run(&output)
	if err == nil {
		t.Fatal("expected Run() to return an error when a push fails")
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("error = %q, want project name alpha", err.Error())
	}

	wantOutput := strings.Join([]string{
		"alpha - push: ❌ error: failed to push some refs to 'origin'",
		"beta - push: ✅",
		"Summary: eligible=2, pushed=1, failed=1",
		"",
	}, "\n")
	if output.String() != wantOutput {
		t.Fatalf("output = %q, want %q", output.String(), wantOutput)
	}
}

func TestWorkspacePushRunnerRunReportsWhenNothingNeedsPush(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:openai/alpha.git\n")

	initializer := newWorkspaceStatusTestInitializer(t, workspaceRoot)
	runner := &WorkspacePushRunner{
		configInitializer: initializer,
		runGit: func(projectPath string, args ...string) ([]byte, error) {
			call := filepath.Base(projectPath) + ":" + strings.Join(args, " ")

			switch call {
			case "alpha:status --porcelain":
				return []byte(""), nil
			case "alpha:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return []byte("origin/main\n"), nil
			case "alpha:rev-list --left-right --count @{upstream}...HEAD":
				return []byte("0\t0\n"), nil
			default:
				t.Fatalf("unexpected git invocation: %s", call)
				return nil, errors.New("unexpected git invocation")
			}
		},
	}

	var output bytes.Buffer
	if err := runner.Run(&output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	want := "No clean, unsynced workspace projects with remotes to push.\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestWorkspacePushRunnerRunReturnsErrorWhenWorkspaceRootDoesNotExist(t *testing.T) {
	runner := &WorkspacePushRunner{
		configInitializer: newWorkspaceStatusTestInitializer(t, filepath.Join(t.TempDir(), "missing")),
		runGit: func(projectPath string, args ...string) ([]byte, error) {
			t.Fatalf("unexpected git invocation for %s %v", projectPath, args)
			return nil, nil
		},
	}

	var output bytes.Buffer
	err := runner.Run(&output)
	if err == nil {
		t.Fatal("expected Run() to return an error when workspace root does not exist")
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty output", output.String())
	}
}
