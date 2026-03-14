package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	githubUserRepositoriesPath = "/user/repos"
	repoListPageSize           = 100
)

type gitHubAccessTokenProvider interface {
	EnsureValidToken(ctx context.Context) (string, error)
}

type RepoListRunner struct {
	initializer *ConfigInitializer
	authService gitHubAccessTokenProvider
	httpClient  *http.Client
	pageSize    int
}

type RepoListEntry struct {
	RemotePath string
	Present    bool
}

type RepoListReport struct {
	Entries           []RepoListEntry
	TotalCount        int
	LocalCloneCount   int
	LocalMissingCount int
}

type gitHubRepoOwner struct {
	Login string `json:"login"`
}

type gitHubRepositoryResponse struct {
	Name  string          `json:"name"`
	Owner gitHubRepoOwner `json:"owner"`
}

type gitHubCurrentUser struct {
	Login string `json:"login"`
}

func newRepoListRunner(initializer *ConfigInitializer) *RepoListRunner {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	return &RepoListRunner{
		initializer: initializer,
		authService: newGitHubAuthService(initializer),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		pageSize: repoListPageSize,
	}
}

func (r *RepoListRunner) Run(ctx context.Context, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if _, err := fmt.Fprintln(stdout, "Fetching repositories from GitHub..."); err != nil {
		return err
	}

	report, err := r.Evaluate(ctx)
	if err != nil {
		return err
	}

	for _, entry := range report.Entries {
		status := "❌"
		if entry.Present {
			status = "✅"
		}

		if _, err := fmt.Fprintf(stdout, "%s - %s\n", entry.RemotePath, status); err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(
		stdout,
		"Summary: total=%d, cloned=%d, missing=%d\n",
		report.TotalCount,
		report.LocalCloneCount,
		report.LocalMissingCount,
	)
	return err
}

func (r *RepoListRunner) Evaluate(ctx context.Context) (RepoListReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.ensureDefaults(); err != nil {
		return RepoListReport{}, err
	}

	config, err := loadGitHubAuthBaseConfig(r.initializer)
	if err != nil {
		return RepoListReport{}, err
	}

	accessToken, err := r.authService.EnsureValidToken(ctx)
	if err != nil {
		return RepoListReport{}, err
	}

	currentUser, err := r.fetchCurrentUser(ctx, config.APIBaseURL, accessToken)
	if err != nil {
		return RepoListReport{}, err
	}

	repositories, err := r.fetchRepositories(ctx, config.APIBaseURL, accessToken)
	if err != nil {
		return RepoListReport{}, err
	}

	workspaceEntries, err := r.loadWorkspaceEntries()
	if err != nil {
		return RepoListReport{}, err
	}

	workspaceByName := make(map[string]WorkspaceEntry, len(workspaceEntries))
	for _, entry := range workspaceEntries {
		workspaceByName[entry.LocalName] = entry
	}

	report := RepoListReport{}
	for _, repository := range repositories {
		if strings.EqualFold(strings.TrimSpace(repository.Owner.Login), strings.TrimSpace(currentUser.Login)) {
			continue
		}

		remotePath := githubRepositoryPath(repository.Owner.Login, repository.Name)
		workspaceEntry, ok := workspaceByName[repository.Name]
		present := ok && workspaceEntry.HasRemote && strings.EqualFold(workspaceEntry.RemotePath, remotePath)

		report.Entries = append(report.Entries, RepoListEntry{
			RemotePath: remotePath,
			Present:    present,
		})
		if present {
			report.LocalCloneCount++
		} else {
			report.LocalMissingCount++
		}
	}

	sort.Slice(report.Entries, func(i, j int) bool {
		return report.Entries[i].RemotePath < report.Entries[j].RemotePath
	})
	report.TotalCount = len(report.Entries)

	return report, nil
}

func (r *RepoListRunner) ensureDefaults() error {
	if r.initializer == nil {
		r.initializer = newDefaultConfigInitializer()
	}
	if r.authService == nil {
		r.authService = newGitHubAuthService(r.initializer)
	}
	if r.httpClient == nil {
		r.httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	if r.pageSize <= 0 {
		r.pageSize = repoListPageSize
	}

	return nil
}

func (r *RepoListRunner) fetchCurrentUser(ctx context.Context, apiBaseURL string, accessToken string) (gitHubCurrentUser, error) {
	endpoint, err := githubAPIEndpoint(apiBaseURL, githubCurrentUserPath)
	if err != nil {
		return gitHubCurrentUser{}, err
	}

	var currentUser gitHubCurrentUser
	if err := r.getJSON(ctx, endpoint, accessToken, &currentUser); err != nil {
		return gitHubCurrentUser{}, fmt.Errorf("fetch current user: %w", err)
	}
	if strings.TrimSpace(currentUser.Login) == "" {
		return gitHubCurrentUser{}, fmt.Errorf("fetch current user: GitHub response did not include a login")
	}

	return currentUser, nil
}

func (r *RepoListRunner) fetchRepositories(ctx context.Context, apiBaseURL string, accessToken string) ([]gitHubRepositoryResponse, error) {
	endpoint, err := githubAPIEndpoint(apiBaseURL, githubUserRepositoriesPath)
	if err != nil {
		return nil, err
	}

	repositories := []gitHubRepositoryResponse{}
	for page := 1; ; page++ {
		pageURL, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("parse repositories endpoint: %w", err)
		}

		query := pageURL.Query()
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(r.pageSize))
		query.Set("affiliation", "owner,collaborator,organization_member")
		query.Set("sort", "full_name")
		pageURL.RawQuery = query.Encode()

		var pageRepositories []gitHubRepositoryResponse
		if err := r.getJSON(ctx, pageURL.String(), accessToken, &pageRepositories); err != nil {
			return nil, fmt.Errorf("fetch repositories page %d: %w", page, err)
		}
		if len(pageRepositories) == 0 {
			break
		}

		repositories = append(repositories, pageRepositories...)
		if len(pageRepositories) < r.pageSize {
			break
		}
	}

	return repositories, nil
}

func (r *RepoListRunner) getJSON(ctx context.Context, endpoint string, accessToken string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	request.Header.Set("Accept", githubRESTAcceptHeader)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	response, err := r.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := decodeGitHubAPIErrorMessage(body)
		if strings.TrimSpace(message) == "" {
			message = response.Status
		}

		return fmt.Errorf("unexpected status: %s: %s", response.Status, message)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (r *RepoListRunner) loadWorkspaceEntries() ([]WorkspaceEntry, error) {
	workspaceRoot, err := resolvedWorkspaceRoot(r.initializer)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(workspaceRoot); errors.Is(err, os.ErrNotExist) {
		return []WorkspaceEntry{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat workspace directory: %w", err)
	}

	return listWorkspaceEntriesInRoot(workspaceRoot)
}

func githubRepositoryPath(owner string, name string) string {
	return fmt.Sprintf("github.com/%s/%s", strings.TrimSpace(owner), strings.TrimSpace(name))
}
