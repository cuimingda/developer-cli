package cmd

import (
	"bytes"
	"strings"
	"testing"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

func TestWorkspaceListCommandPrintsDirectoriesAndRemotePaths(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "beta", `[remote "origin"]`+"\n\turl = https://github.com/openai/beta.git\n")
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:openai/alpha.git\n")
	createWorkspaceProject(t, workspaceRoot, "gamma", `[core]`+"\n\trepositoryformatversion = 0\n")

	configInitializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
		defaultYAML:  configtemplate.DefaultYAML(),
	}
	if _, err := configInitializer.Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if err := configInitializer.SetValue("workspace.root", workspaceRoot); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	cmd := newRootCmdWithConfigInitializer(configInitializer)
	var output bytes.Buffer

	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"workspace", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	want := strings.Join([]string{
		"alpha - github.com/openai/alpha",
		"beta - github.com/openai/beta",
		"gamma - " + redNoRemote,
		"",
	}, "\n")
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestLSCommandPrintsDirectoriesAndRemotePaths(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "beta", `[remote "origin"]`+"\n\turl = https://github.com/openai/beta.git\n")
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:openai/alpha.git\n")
	createWorkspaceProject(t, workspaceRoot, "gamma", `[core]`+"\n\trepositoryformatversion = 0\n")

	configInitializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
		defaultYAML:  configtemplate.DefaultYAML(),
	}
	if _, err := configInitializer.Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if err := configInitializer.SetValue("workspace.root", workspaceRoot); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	cmd := newRootCmdWithConfigInitializer(configInitializer)
	var output bytes.Buffer

	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"ls"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	want := strings.Join([]string{
		"alpha - github.com/openai/alpha",
		"beta - github.com/openai/beta",
		"gamma - " + redNoRemote,
		"",
	}, "\n")
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
