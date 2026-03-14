package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

func TestWorkspaceListerListReturnsDirectoriesAndRemotePaths(t *testing.T) {
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

	lister := &WorkspaceLister{
		configInitializer: configInitializer,
	}

	entries, err := lister.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}

	want := []WorkspaceEntry{
		{LocalName: "alpha", RemotePath: "github.com/openai/alpha", HasRemote: true},
		{LocalName: "beta", RemotePath: "github.com/openai/beta", HasRemote: true},
		{LocalName: "gamma", RemotePath: "", HasRemote: false},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("List() = %#v, want %#v", entries, want)
	}
}

func TestWorkspaceListerListReturnsErrorWhenWorkspaceRootDoesNotExist(t *testing.T) {
	configInitializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
		defaultYAML:  configtemplate.DefaultYAML(),
	}
	if _, err := configInitializer.Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if err := configInitializer.SetValue("workspace.root", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	lister := &WorkspaceLister{
		configInitializer: configInitializer,
	}

	_, err := lister.List()
	if err == nil {
		t.Fatal("expected List() to return an error when workspace root does not exist")
	}
}

func createWorkspaceProject(t *testing.T, workspaceRoot string, name string, gitConfig string) {
	t.Helper()

	projectPath := filepath.Join(workspaceRoot, name)
	if err := os.MkdirAll(filepath.Join(projectPath, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectPath, ".git", "config"), []byte(gitConfig), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}
}
