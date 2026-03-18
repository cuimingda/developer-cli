package cmd

import (
	"fmt"
	"io"
)

const (
	workspaceStatusSatisfiedIcon   = "✅"
	workspaceStatusUnsatisfiedIcon = "❌"
)

type WorkspaceStatusEntry struct {
	LocalName string
	HasGit    bool
	HasRemote bool
	IsClean   bool
	IsSynced  bool
}

type WorkspaceStatusRunner struct {
	configInitializer *ConfigInitializer
	runGit            workspaceGitCommandRunner
}

func newDefaultWorkspaceStatusRunner(configInitializer *ConfigInitializer) *WorkspaceStatusRunner {
	if configInitializer == nil {
		configInitializer = newDefaultConfigInitializer()
	}

	return &WorkspaceStatusRunner{
		configInitializer: configInitializer,
		runGit:            runGitCommandInProject,
	}
}

func (r *WorkspaceStatusRunner) Run(stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	entries, err := r.Evaluate()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if _, err := fmt.Fprintf(
			stdout,
			"%s - git: %s remote: %s clean: %s synced: %s\n",
			entry.LocalName,
			workspaceStatusIcon(entry.HasGit),
			workspaceStatusIcon(entry.HasRemote),
			workspaceStatusIcon(entry.IsClean),
			workspaceStatusIcon(entry.IsSynced),
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *WorkspaceStatusRunner) Evaluate() ([]WorkspaceStatusEntry, error) {
	if err := r.ensureDefaults(); err != nil {
		return nil, err
	}

	workspaceRoot, err := resolvedWorkspaceRoot(r.configInitializer)
	if err != nil {
		return nil, err
	}

	entries, err := listWorkspaceEntriesInRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}

	statuses := make([]WorkspaceStatusEntry, 0, len(entries))
	for _, entry := range entries {
		status, err := r.evaluateWorkspaceEntry(workspaceRoot, entry)
		if err != nil {
			return nil, err
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (r *WorkspaceStatusRunner) ensureDefaults() error {
	if r == nil {
		return fmt.Errorf("workspace status runner is not configured")
	}
	if r.configInitializer == nil {
		r.configInitializer = newDefaultConfigInitializer()
	}
	if r.runGit == nil {
		r.runGit = runGitCommandInProject
	}

	return nil
}

func workspaceStatusIcon(satisfied bool) string {
	if satisfied {
		return workspaceStatusSatisfiedIcon
	}

	return workspaceStatusUnsatisfiedIcon
}
