package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const redNoRemote = "\x1b[31m<no remote>\x1b[0m"

type WorkspaceEntry struct {
	LocalName  string
	RemotePath string
	HasRemote  bool
}

type WorkspaceLister struct {
	configInitializer *ConfigInitializer
}

func newDefaultWorkspaceLister(configInitializer *ConfigInitializer) *WorkspaceLister {
	if configInitializer == nil {
		configInitializer = newDefaultConfigInitializer()
	}

	return &WorkspaceLister{
		configInitializer: configInitializer,
	}
}

func (w *WorkspaceLister) List() ([]WorkspaceEntry, error) {
	workspaceRoot, err := resolvedWorkspaceRoot(w.configInitializer)
	if err != nil {
		return nil, err
	}

	return listWorkspaceEntriesInRoot(workspaceRoot)
}

func listWorkspaceEntriesInRoot(workspaceRoot string) ([]WorkspaceEntry, error) {
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("read workspace directory: %w", err)
	}

	workspaces := make([]WorkspaceEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		remotePath, hasRemote, err := workspaceRemotePath(filepath.Join(workspaceRoot, entry.Name()))
		if err != nil {
			return nil, err
		}

		workspaces = append(workspaces, WorkspaceEntry{
			LocalName:  entry.Name(),
			RemotePath: remotePath,
			HasRemote:  hasRemote,
		})
	}

	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].LocalName < workspaces[j].LocalName
	})

	return workspaces, nil
}

func workspaceRemotePath(projectPath string) (string, bool, error) {
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

	remoteURL, ok := chooseGitRemoteURL(remoteURLs)
	if !ok {
		return "", false, nil
	}

	return normalizeGitHubRemotePath(remoteURL), true, nil
}

func gitConfigPath(projectPath string) (string, error) {
	gitPath := filepath.Join(projectPath, ".git")
	info, err := os.Stat(gitPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stat git metadata for %s: %w", projectPath, err)
	}

	if info.IsDir() {
		return filepath.Join(gitPath, "config"), nil
	}

	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read git metadata for %s: %w", projectPath, err)
	}

	gitDirDirective := strings.TrimSpace(string(content))
	if !strings.HasPrefix(gitDirDirective, "gitdir:") {
		return "", nil
	}

	gitDirPath := strings.TrimSpace(strings.TrimPrefix(gitDirDirective, "gitdir:"))
	if !filepath.IsAbs(gitDirPath) {
		gitDirPath = filepath.Join(projectPath, gitDirPath)
	}

	return filepath.Join(gitDirPath, "config"), nil
}

func readGitRemoteURLs(configPath string) (map[string]string, error) {
	content, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read git config %s: %w", configPath, err)
	}

	remoteURLs := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	currentRemote := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentRemote = parseGitRemoteSection(line)
			continue
		}

		if currentRemote == "" {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if strings.TrimSpace(key) != "url" {
			continue
		}

		remoteURLs[currentRemote] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan git config %s: %w", configPath, err)
	}

	return remoteURLs, nil
}

func parseGitRemoteSection(sectionLine string) string {
	section := strings.TrimSuffix(strings.TrimPrefix(sectionLine, "["), "]")
	if !strings.HasPrefix(section, `remote "`) || !strings.HasSuffix(section, `"`) {
		return ""
	}

	return strings.TrimSuffix(strings.TrimPrefix(section, `remote "`), `"`)
}

func chooseGitRemoteURL(remoteURLs map[string]string) (string, bool) {
	remoteName, ok := chooseGitRemoteName(remoteURLs)
	if !ok {
		return "", false
	}

	return strings.TrimSpace(remoteURLs[remoteName]), true
}

func chooseGitRemoteName(remoteURLs map[string]string) (string, bool) {
	if remoteURL := strings.TrimSpace(remoteURLs["origin"]); remoteURL != "" {
		return "origin", true
	}

	remoteNames := make([]string, 0, len(remoteURLs))
	for name := range remoteURLs {
		remoteNames = append(remoteNames, name)
	}
	sort.Strings(remoteNames)

	for _, name := range remoteNames {
		if remoteURL := strings.TrimSpace(remoteURLs[name]); remoteURL != "" {
			return name, true
		}
	}

	return "", false
}

func normalizeGitHubRemotePath(remoteURL string) string {
	trimmedURL := strings.TrimSpace(remoteURL)
	if trimmedURL == "" {
		return trimmedURL
	}

	if strings.HasPrefix(trimmedURL, "git@github.com:") {
		return "github.com/" + trimGitRemoteSuffix(strings.TrimPrefix(trimmedURL, "git@github.com:"))
	}

	if parsedURL, err := url.Parse(trimmedURL); err == nil && strings.EqualFold(parsedURL.Host, "github.com") {
		return "github.com/" + trimGitRemoteSuffix(strings.TrimPrefix(parsedURL.Path, "/"))
	}

	return trimGitRemoteSuffix(trimmedURL)
}

func trimGitRemoteSuffix(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(value), ".git")
}
