package cmd

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type workspaceGitCommandRunner func(projectPath string, args ...string) ([]byte, error)

func (r *WorkspaceStatusRunner) evaluateWorkspaceEntry(workspaceRoot string, entry WorkspaceEntry) (WorkspaceStatusEntry, error) {
	projectPath := filepath.Join(workspaceRoot, entry.LocalName)
	hasGit, err := workspaceHasGit(projectPath)
	if err != nil {
		return WorkspaceStatusEntry{}, err
	}

	status := WorkspaceStatusEntry{
		LocalName: entry.LocalName,
		HasGit:    hasGit,
		HasRemote: entry.HasRemote,
	}
	if !status.HasGit {
		return status, nil
	}

	status.IsClean = gitWorkingTreeClean(projectPath, r.runGit)
	if status.HasRemote && status.IsClean {
		status.IsSynced = gitBranchSynchronized(projectPath, r.runGit)
	}

	return status, nil
}

func workspaceHasGit(projectPath string) (bool, error) {
	configPath, err := gitConfigPath(projectPath)
	if err != nil {
		return false, err
	}

	return configPath != "", nil
}

func gitWorkingTreeClean(projectPath string, runGit workspaceGitCommandRunner) bool {
	output, err := runGit(projectPath, "status", "--porcelain")
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == ""
}

func gitBranchSynchronized(projectPath string, runGit workspaceGitCommandRunner) bool {
	if !gitCurrentUpstreamConfigured(projectPath, runGit) {
		return false
	}

	output, err := runGit(projectPath, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if err != nil {
		return false
	}

	behind, ahead, ok := parseGitRevisionCounts(string(output))
	return ok && behind == 0 && ahead == 0
}

func gitCurrentUpstreamConfigured(projectPath string, runGit workspaceGitCommandRunner) bool {
	if _, err := runGit(projectPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err != nil {
		return false
	}

	return true
}

func parseGitRevisionCounts(output string) (behind int, ahead int, ok bool) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 {
		return 0, 0, false
	}

	behind, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, false
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, false
	}

	return behind, ahead, true
}

func runGitCommandInProject(projectPath string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = projectPath

	return command.CombinedOutput()
}
