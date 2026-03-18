package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type WorkspacePushEntry struct {
	LocalName string
	Pushed    bool
	Message   string
}

type WorkspacePushReport struct {
	Entries       []WorkspacePushEntry
	EligibleCount int
	PushedCount   int
	FailedCount   int
}

type WorkspacePushRunner struct {
	configInitializer *ConfigInitializer
	runGit            workspaceGitCommandRunner
}

func newDefaultWorkspacePushRunner(configInitializer *ConfigInitializer) *WorkspacePushRunner {
	if configInitializer == nil {
		configInitializer = newDefaultConfigInitializer()
	}

	return &WorkspacePushRunner{
		configInitializer: configInitializer,
		runGit:            runGitCommandInProject,
	}
}

func (r *WorkspacePushRunner) Run(stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	report, err := r.PushAll()
	if err == nil && report.EligibleCount == 0 {
		_, writeErr := fmt.Fprintln(stdout, "No clean, unsynced workspace projects with remotes to push.")
		if writeErr != nil {
			return writeErr
		}

		return err
	}

	for _, entry := range report.Entries {
		statusIcon := workspaceStatusUnsatisfiedIcon
		if entry.Pushed {
			statusIcon = workspaceStatusSatisfiedIcon
		}

		if entry.Message == "" {
			if _, writeErr := fmt.Fprintf(stdout, "%s - push: %s\n", entry.LocalName, statusIcon); writeErr != nil {
				return writeErr
			}
			continue
		}

		if _, writeErr := fmt.Fprintf(stdout, "%s - push: %s %s\n", entry.LocalName, statusIcon, entry.Message); writeErr != nil {
			return writeErr
		}
	}
	if report.EligibleCount == 0 {
		return err
	}

	_, writeErr := fmt.Fprintf(
		stdout,
		"Summary: eligible=%d, pushed=%d, failed=%d\n",
		report.EligibleCount,
		report.PushedCount,
		report.FailedCount,
	)
	if writeErr != nil {
		return writeErr
	}

	return err
}

func (r *WorkspacePushRunner) PushAll() (WorkspacePushReport, error) {
	if err := r.ensureDefaults(); err != nil {
		return WorkspacePushReport{}, err
	}

	workspaceRoot, err := resolvedWorkspaceRoot(r.configInitializer)
	if err != nil {
		return WorkspacePushReport{}, err
	}

	statusRunner := &WorkspaceStatusRunner{
		configInitializer: r.configInitializer,
		runGit:            r.runGit,
	}
	statuses, err := statusRunner.Evaluate()
	if err != nil {
		return WorkspacePushReport{}, err
	}

	report := WorkspacePushReport{}
	failedProjects := []string{}
	for _, status := range statuses {
		if !workspaceStatusEligibleForPush(status) {
			continue
		}

		report.EligibleCount++
		projectPath := filepath.Join(workspaceRoot, status.LocalName)
		output, pushErr := r.pushProject(projectPath)

		entry := WorkspacePushEntry{
			LocalName: status.LocalName,
		}
		if pushErr != nil {
			entry.Message = summarizeGitPushFailure(output, pushErr)
			report.FailedCount++
			failedProjects = append(failedProjects, status.LocalName)
		} else {
			entry.Pushed = true
			report.PushedCount++
		}

		report.Entries = append(report.Entries, entry)
	}

	if len(failedProjects) > 0 {
		return report, fmt.Errorf("push failed for workspace projects: %s", strings.Join(failedProjects, ", "))
	}

	return report, nil
}

func (r *WorkspacePushRunner) ensureDefaults() error {
	if r == nil {
		return fmt.Errorf("workspace push runner is not configured")
	}
	if r.configInitializer == nil {
		r.configInitializer = newDefaultConfigInitializer()
	}
	if r.runGit == nil {
		r.runGit = runGitCommandInProject
	}

	return nil
}
