package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func useTestLocalTimezone(t *testing.T) {
	t.Helper()

	originalLocal := time.Local
	originalReadlink := readLocaltimeLink
	t.Cleanup(func() {
		time.Local = originalLocal
		readLocaltimeLink = originalReadlink
	})

	time.Local = time.FixedZone("CST", 8*60*60)
	readLocaltimeLink = func(string) (string, error) {
		return "/var/db/timezone/zoneinfo/Asia/Shanghai", nil
	}
}

func TestGitHubAuthStatusRunnerRunReportsNotLoggedInWhenTokenIsMissing(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		tokenStore: &stubGitHubTokenStore{
			loadErr: ErrGitHubTokenNotFound,
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	reportOutput := output.String()
	if !strings.Contains(reportOutput, "Status: Not logged in: no access token is stored in the macOS keychain.") {
		t.Fatalf("output = %q, want not logged in status", reportOutput)
	}
	if !strings.Contains(reportOutput, "Recommended next step: run `dev github login`") {
		t.Fatalf("output = %q, want login recommendation", reportOutput)
	}
	if !strings.Contains(reportOutput, "- GET /user: skipped because no access token is stored locally") {
		t.Fatalf("output = %q, want skipped remote probe", reportOutput)
	}
	if !strings.Contains(reportOutput, "- GitHub App installed: unknown (skipped because no access token is stored locally)") {
		t.Fatalf("output = %q, want skipped app installation probe", reportOutput)
	}
}

func TestGitHubAuthStatusRunnerEvaluateReturnsValidStateWhenRemoteProbeSucceeds(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization header = %q, want %q", got, "Bearer access-token")
		}

		switch r.URL.Path {
		case "/api/v3/user":
			writeGitHubJSONResponse(t, w, `{"login":"octocat"}`)
		case "/api/v3/user/installations":
			writeGitHubJSONResponse(t, w, `{"total_count":1,"installations":[{"target_type":"Organization","account":{"login":"acme"}}]}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token",
				RefreshToken:          "refresh-token",
				AccessTokenExpiresAt:  timePointer(now.Add(2 * time.Hour)),
				RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
			},
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	report, err := runner.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	if report.State != GitHubAuthStateTokenValid {
		t.Fatalf("report.State = %q, want %q", report.State, GitHubAuthStateTokenValid)
	}
	if report.NextAction != GitHubAuthNextActionCallAPI {
		t.Fatalf("report.NextAction = %q, want %q", report.NextAction, GitHubAuthNextActionCallAPI)
	}
	if report.Username != "octocat" {
		t.Fatalf("report.Username = %q, want %q", report.Username, "octocat")
	}
	if report.RemoteProbeStatusCode != http.StatusOK {
		t.Fatalf("report.RemoteProbeStatusCode = %d, want %d", report.RemoteProbeStatusCode, http.StatusOK)
	}
	if report.AppInstallationState != GitHubAppInstallationInstalled {
		t.Fatalf("report.AppInstallationState = %q, want %q", report.AppInstallationState, GitHubAppInstallationInstalled)
	}
	if report.AppInstallationCount != 1 {
		t.Fatalf("report.AppInstallationCount = %d, want %d", report.AppInstallationCount, 1)
	}
}

func TestGitHubAuthStatusRunnerEvaluateReturnsRefreshableStateWhenAccessTokenExpired(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token",
				RefreshToken:          "refresh-token",
				AccessTokenExpiresAt:  timePointer(now.Add(-1 * time.Minute)),
				RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
			},
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	report, err := runner.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	if report.State != GitHubAuthStateAccessTokenExpiredRefreshable {
		t.Fatalf("report.State = %q, want %q", report.State, GitHubAuthStateAccessTokenExpiredRefreshable)
	}
	if report.NextAction != GitHubAuthNextActionRefresh {
		t.Fatalf("report.NextAction = %q, want %q", report.NextAction, GitHubAuthNextActionRefresh)
	}
	if report.RemoteProbeState != GitHubRemoteProbeSkipped {
		t.Fatalf("report.RemoteProbeState = %q, want %q", report.RemoteProbeState, GitHubRemoteProbeSkipped)
	}
}

func TestGitHubAuthStatusRunnerEvaluateReturnsAuthorizationInvalidOn401(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeGitHubJSONResponse(t, w, `{"message":"Bad credentials"}`)
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:          "access-token",
				AccessTokenExpiresAt: timePointer(now.Add(2 * time.Hour)),
			},
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	report, err := runner.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	if report.State != GitHubAuthStateAuthorizationInvalid {
		t.Fatalf("report.State = %q, want %q", report.State, GitHubAuthStateAuthorizationInvalid)
	}
	if report.NextAction != GitHubAuthNextActionLogin {
		t.Fatalf("report.NextAction = %q, want %q", report.NextAction, GitHubAuthNextActionLogin)
	}
	if report.RemoteProbeStatusCode != http.StatusUnauthorized {
		t.Fatalf("report.RemoteProbeStatusCode = %d, want %d", report.RemoteProbeStatusCode, http.StatusUnauthorized)
	}
}

func TestGitHubAuthStatusRunnerEvaluateReturnsReauthenticationRequiredWhenRefreshTokenExpired(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token",
				RefreshToken:          "refresh-token",
				AccessTokenExpiresAt:  timePointer(now.Add(-1 * time.Minute)),
				RefreshTokenExpiresAt: timePointer(now.Add(-1 * time.Minute)),
			},
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	report, err := runner.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	if report.State != GitHubAuthStateReauthenticationRequired {
		t.Fatalf("report.State = %q, want %q", report.State, GitHubAuthStateReauthenticationRequired)
	}
	if report.NextAction != GitHubAuthNextActionLogin {
		t.Fatalf("report.NextAction = %q, want %q", report.NextAction, GitHubAuthNextActionLogin)
	}
}

func TestGitHubAuthStatusRunnerRunRecommendsFixingConfigWhenClientIDIsMissing(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)

	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		tokenStore: &stubGitHubTokenStore{
			loadErr: ErrGitHubTokenNotFound,
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if !strings.Contains(output.String(), "Recommended next step: configure `github.client_id` before logging in or refreshing") {
		t.Fatalf("output = %q, want config recommendation", output.String())
	}
}

func TestGitHubAuthStatusRunnerRunDisplaysTokenTimesInLocalTimezone(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/user":
			writeGitHubJSONResponse(t, w, `{"login":"octocat"}`)
		case "/api/v3/user/installations":
			writeGitHubJSONResponse(t, w, `{"total_count":0,"installations":[]}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token",
				RefreshToken:          "refresh-token",
				AccessTokenExpiresAt:  timePointer(now.Add(2 * time.Hour)),
				RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
			},
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	reportOutput := output.String()
	if !strings.Contains(reportOutput, "valid until 2026-03-13T22:00:00+08:00 (Asia/Shanghai) (in 2h0m0s)") {
		t.Fatalf("output = %q, want access token time in local timezone", reportOutput)
	}
	if !strings.Contains(reportOutput, "valid until 2026-03-14T20:00:00+08:00 (Asia/Shanghai) (in 24h0m0s)") {
		t.Fatalf("output = %q, want refresh token time in local timezone", reportOutput)
	}
	if !strings.Contains(reportOutput, "- GitHub App installed: no") {
		t.Fatalf("output = %q, want app installation status", reportOutput)
	}
}

func TestGitHubAuthStatusRunnerRunReportsInstalledGitHubApp(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/user":
			writeGitHubJSONResponse(t, w, `{"login":"octocat"}`)
		case "/api/v3/user/installations":
			writeGitHubJSONResponse(t, w, `{"total_count":2,"installations":[{"target_type":"User","account":{"login":"octocat"}},{"target_type":"Organization","account":{"login":"acme"}}]}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token",
				RefreshToken:          "refresh-token",
				AccessTokenExpiresAt:  timePointer(now.Add(2 * time.Hour)),
				RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
			},
		},
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if !strings.Contains(output.String(), "- GitHub App installed: yes (2 accessible installations: octocat (User), acme (Organization))") {
		t.Fatalf("output = %q, want installed app status", output.String())
	}
}

func TestTokenTimeDescriptionDisplaysExpiredTimeInLocalTimezone(t *testing.T) {
	useTestLocalTimezone(t)

	expiresAt := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	description := tokenTimeDescription(true, &expiresAt, true, false, expiresAt)

	if got, want := description, "expired at 2026-03-13T20:00:00+08:00 (Asia/Shanghai)"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestRefreshTokenTimeDescriptionDisplaysExpiredTimeInLocalTimezone(t *testing.T) {
	useTestLocalTimezone(t)

	expiresAt := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	description := refreshTokenTimeDescription(true, &expiresAt, true, expiresAt)

	if got, want := description, "expired at 2026-03-13T20:00:00+08:00 (Asia/Shanghai)"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
