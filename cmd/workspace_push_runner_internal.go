package cmd

import (
	"fmt"
	"strings"
)

func workspaceStatusEligibleForPush(status WorkspaceStatusEntry) bool {
	return status.HasRemote && status.IsClean && !status.IsSynced
}

func (r *WorkspacePushRunner) pushProject(projectPath string) ([]byte, error) {
	if gitCurrentUpstreamConfigured(projectPath, r.runGit) {
		return r.runGit(projectPath, "push")
	}

	remoteName, ok, err := workspacePreferredRemoteName(projectPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no git remote configured")
	}

	return r.runGit(projectPath, "push", "--set-upstream", remoteName, "HEAD")
}

func workspacePreferredRemoteName(projectPath string) (string, bool, error) {
	configPath, err := gitConfigPath(projectPath)
	if err != nil {
		return "", false, err
	}
	if configPath == "" {
		return "", false, nil
	}

	remoteURLs, err := readGitRemoteURLs(configPath)
	if err != nil {
		return "", false, err
	}

	remoteName, ok := chooseGitRemoteName(remoteURLs)
	if !ok {
		return "", false, nil
	}

	return remoteName, true, nil
}

func summarizeGitPushFailure(output []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "failed to push") || strings.Contains(trimmed, "rejected") || strings.HasPrefix(trimmed, "error:") || strings.HasPrefix(trimmed, "fatal:") {
			return trimmed
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	if err != nil {
		return err.Error()
	}

	return ""
}
