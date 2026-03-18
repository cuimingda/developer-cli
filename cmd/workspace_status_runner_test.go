package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

func TestWorkspaceStatusRunnerRunReportsWorkspaceProjectStatuses(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:openai/alpha.git\n")
	createWorkspaceProject(t, workspaceRoot, "beta", `[remote "origin"]`+"\n\turl = git@github.com:openai/beta.git\n")
	createWorkspaceProject(t, workspaceRoot, "epsilon", `[remote "origin"]`+"\n\turl = git@github.com:openai/epsilon.git\n")
	createWorkspaceProject(t, workspaceRoot, "gamma", `[core]`+"\n\trepositoryformatversion = 0\n")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "delta"), 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}

	initializer := newWorkspaceStatusTestInitializer(t, workspaceRoot)
	gitCalls := []string{}
	runner := &WorkspaceStatusRunner{
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
				return []byte("0\t0\n"), nil
			case "beta:status --porcelain":
				return []byte(""), nil
			case "beta:rev-parse --abbrev-ref --symbolic-full-name @{upstream}":
				return []byte("origin/main\n"), nil
			case "beta:rev-list --left-right --count @{upstream}...HEAD":
				return []byte("0\t1\n"), nil
			case "epsilon:status --porcelain":
				return []byte(" M README.md\n"), nil
			case "gamma:status --porcelain":
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
		"alpha - git: ✅ remote: ✅ clean: ✅ synced: ✅",
		"beta - git: ✅ remote: ✅ clean: ✅ synced: ❌",
		"delta - git: ❌ remote: ❌ clean: ❌ synced: ❌",
		"epsilon - git: ✅ remote: ✅ clean: ❌ synced: ❌",
		"gamma - git: ✅ remote: ❌ clean: ✅ synced: ❌",
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
		"beta:rev-list --left-right --count @{upstream}...HEAD",
		"epsilon:status --porcelain",
		"gamma:status --porcelain",
	}
	if !reflect.DeepEqual(gitCalls, wantGitCalls) {
		t.Fatalf("git calls = %#v, want %#v", gitCalls, wantGitCalls)
	}
}

func TestWorkspaceStatusRunnerEvaluateReturnsErrorWhenWorkspaceRootDoesNotExist(t *testing.T) {
	runner := &WorkspaceStatusRunner{
		configInitializer: newWorkspaceStatusTestInitializer(t, filepath.Join(t.TempDir(), "missing")),
		runGit: func(projectPath string, args ...string) ([]byte, error) {
			t.Fatalf("unexpected git invocation for %s %v", projectPath, args)
			return nil, nil
		},
	}

	_, err := runner.Evaluate()
	if err == nil {
		t.Fatal("expected Evaluate() to return an error when workspace root does not exist")
	}
}

func newWorkspaceStatusTestInitializer(t *testing.T, workspaceRoot string) *ConfigInitializer {
	t.Helper()

	initializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
		defaultYAML:  configtemplate.DefaultYAML(),
	}
	if _, err := initializer.Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if err := initializer.SetValue("workspace.root", workspaceRoot); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	return initializer
}
