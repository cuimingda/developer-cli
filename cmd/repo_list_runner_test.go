package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

type stubGitHubAccessTokenProvider struct {
	token string
	err   error
}

func (s *stubGitHubAccessTokenProvider) EnsureValidToken(context.Context) (string, error) {
	return s.token, s.err
}

func TestRepoListRunnerEvaluateFiltersOwnedReposPaginatesAndMatchesLocalStatus(t *testing.T) {
	workspaceRoot := t.TempDir()
	createWorkspaceProject(t, workspaceRoot, "alpha", `[remote "origin"]`+"\n\turl = git@github.com:acme/alpha.git\n")
	createWorkspaceProject(t, workspaceRoot, "beta", `[remote "origin"]`+"\n\turl = git@github.com:someone-else/beta.git\n")
	createWorkspaceProject(t, workspaceRoot, "gamma", `[core]`+"\n\trepositoryformatversion = 0\n")

	var repositoryPageRequests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization header = %q, want %q", got, "Bearer access-token")
		}
		if got := r.Header.Get("Accept"); got != githubRESTAcceptHeader {
			t.Fatalf("Accept header = %q, want %q", got, githubRESTAcceptHeader)
		}

		switch r.URL.Path {
		case "/api/v3/user":
			writeGitHubJSONResponse(t, w, `{"login":"octocat"}`)
		case "/api/v3/user/repos":
			repositoryPageRequests = append(repositoryPageRequests, r.URL.RawQuery)
			switch r.URL.Query().Get("page") {
			case "1":
				writeGitHubJSONResponse(t, w, `[
					{"name":"self-repo","owner":{"login":"octocat"}},
					{"name":"alpha","owner":{"login":"acme"}}
				]`)
			case "2":
				writeGitHubJSONResponse(t, w, `[
					{"name":"beta","owner":{"login":"team"}},
					{"name":"gamma","owner":{"login":"org"}}
				]`)
			case "3":
				writeGitHubJSONResponse(t, w, `[
					{"name":"delta","owner":{"login":"org"}}
				]`)
			default:
				writeGitHubJSONResponse(t, w, `[]`)
			}
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

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
	if err := configInitializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	runner := &RepoListRunner{
		initializer: configInitializer,
		authService: &stubGitHubAccessTokenProvider{token: "access-token"},
		httpClient:  server.Client(),
		pageSize:    2,
	}

	report, err := runner.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	wantEntries := []RepoListEntry{
		{RemotePath: "github.com/acme/alpha", Present: true},
		{RemotePath: "github.com/org/delta", Present: false},
		{RemotePath: "github.com/org/gamma", Present: false},
		{RemotePath: "github.com/team/beta", Present: false},
	}
	if !reflect.DeepEqual(report.Entries, wantEntries) {
		t.Fatalf("Entries = %#v, want %#v", report.Entries, wantEntries)
	}
	if report.TotalCount != 4 {
		t.Fatalf("TotalCount = %d, want %d", report.TotalCount, 4)
	}
	if report.LocalCloneCount != 1 {
		t.Fatalf("LocalCloneCount = %d, want %d", report.LocalCloneCount, 1)
	}
	if report.LocalMissingCount != 3 {
		t.Fatalf("LocalMissingCount = %d, want %d", report.LocalMissingCount, 3)
	}
	if len(repositoryPageRequests) != 3 {
		t.Fatalf("repository page requests = %#v, want 3 pages", repositoryPageRequests)
	}
}

func TestRepoListRunnerEvaluateTreatsMissingWorkspaceRootAsNoLocalClones(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/user":
			writeGitHubJSONResponse(t, w, `{"login":"octocat"}`)
		case "/api/v3/user/repos":
			writeGitHubJSONResponse(t, w, `[{"name":"alpha","owner":{"login":"acme"}}]`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

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
	if err := configInitializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	runner := &RepoListRunner{
		initializer: configInitializer,
		authService: &stubGitHubAccessTokenProvider{token: "access-token"},
		httpClient:  server.Client(),
		pageSize:    100,
	}

	report, err := runner.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	wantEntries := []RepoListEntry{
		{RemotePath: "github.com/acme/alpha", Present: false},
	}
	if !reflect.DeepEqual(report.Entries, wantEntries) {
		t.Fatalf("Entries = %#v, want %#v", report.Entries, wantEntries)
	}
	if report.LocalCloneCount != 0 || report.LocalMissingCount != 1 || report.TotalCount != 1 {
		t.Fatalf("report = %#v, want total=1 cloned=0 missing=1", report)
	}
}

func TestRepoListRunnerRunPrintsProgressEntriesAndSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/user":
			writeGitHubJSONResponse(t, w, `{"login":"octocat"}`)
		case "/api/v3/user/repos":
			writeGitHubJSONResponse(t, w, `[{"name":"alpha","owner":{"login":"acme"}}]`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

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
	if err := configInitializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	runner := &RepoListRunner{
		initializer: configInitializer,
		authService: &stubGitHubAccessTokenProvider{token: "access-token"},
		httpClient:  server.Client(),
		pageSize:    100,
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	want := strings.Join([]string{
		"Fetching repositories from GitHub...",
		"github.com/acme/alpha - ❌",
		"Summary: total=1, cloned=0, missing=1",
		"",
	}, "\n")
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
