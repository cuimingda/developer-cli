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

func TestGitHubAuthServiceRefreshExchangesRefreshTokenAndReplacesStoredState(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)

	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	var refreshRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshRequests++
		assertGitHubFormRequest(t, r, map[string]string{
			"client_id":     "client-123",
			"grant_type":    "refresh_token",
			"refresh_token": "refresh-token-1",
		})
		if got := r.PostFormValue("client_secret"); got != "" {
			t.Fatalf("client_secret = %q, want empty", got)
		}

		writeGitHubJSONResponse(t, w, `{"access_token":"access-token-2","token_type":"bearer","refresh_token":"refresh-token-2","expires_in":28800,"refresh_token_expires_in":15897600}`)
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	stubStore := &stubGitHubTokenStore{
		loadToken: GitHubStoredToken{
			AccessToken:           "access-token-1",
			RefreshToken:          "refresh-token-1",
			AccessTokenExpiresAt:  timePointer(now.Add(-1 * time.Minute)),
			RefreshTokenExpiresAt: timePointer(now.Add(30 * 24 * time.Hour)),
			Host:                  strings.TrimPrefix(server.URL, "http://"),
		},
	}
	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore:    stubStore,
		refreshLeeway: githubRefreshLeeway,
	}

	if err := service.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}

	if refreshRequests != 1 {
		t.Fatalf("refresh request count = %d, want %d", refreshRequests, 1)
	}
	if stubStore.savedToken.AccessToken != "access-token-2" {
		t.Fatalf("saved access token = %q, want %q", stubStore.savedToken.AccessToken, "access-token-2")
	}
	if stubStore.savedToken.RefreshToken != "refresh-token-2" {
		t.Fatalf("saved refresh token = %q, want %q", stubStore.savedToken.RefreshToken, "refresh-token-2")
	}
	if stubStore.deletedAccount != "" {
		t.Fatalf("deleted account = %q, want empty", stubStore.deletedAccount)
	}
}

func TestGitHubAuthServiceRefreshReturnsErrorWhenRefreshTokenIsMissing(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken: "access-token",
				Host:        "github.com",
			},
		},
		refreshLeeway: githubRefreshLeeway,
	}

	err := service.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected Refresh() to return an error")
	}

	if got := err.Error(); got != "no refresh token is stored locally; please run `dev github login` again" {
		t.Fatalf("error = %q, want %q", got, "no refresh token is stored locally; please run `dev github login` again")
	}
}

func TestGitHubAuthServiceRefreshReturnsErrorWhenRefreshTokenExpired(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	stubStore := &stubGitHubTokenStore{
		loadToken: GitHubStoredToken{
			AccessToken:           "access-token",
			RefreshToken:          "refresh-token",
			RefreshTokenExpiresAt: timePointer(now.Add(-1 * time.Minute)),
			Host:                  "github.com",
		},
	}
	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return now
		},
		tokenStore:    stubStore,
		refreshLeeway: githubRefreshLeeway,
	}

	err := service.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected Refresh() to return an error")
	}

	if got := err.Error(); got != "refresh token expired; please run `dev github login` again" {
		t.Fatalf("error = %q, want %q", got, "refresh token expired; please run `dev github login` again")
	}
	if stubStore.deletedAccount != "github.com" {
		t.Fatalf("deleted account = %q, want %q", stubStore.deletedAccount, "github.com")
	}
}

func TestGitHubAuthServiceRefreshInvalidatesLocalStateOnBadRefreshToken(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGitHubJSONResponse(t, w, `{"error":"bad_refresh_token","error_description":"The refresh token passed is incorrect or expired."}`)
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	stubStore := &stubGitHubTokenStore{
		loadToken: GitHubStoredToken{
			AccessToken:           "access-token",
			RefreshToken:          "refresh-token",
			RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
			Host:                  strings.TrimPrefix(server.URL, "http://"),
		},
	}
	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore:    stubStore,
		refreshLeeway: githubRefreshLeeway,
	}

	err := service.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected Refresh() to return an error")
	}

	if got := err.Error(); got != "GitHub authorization is no longer valid; please run `dev github login` again" {
		t.Fatalf("error = %q, want %q", got, "GitHub authorization is no longer valid; please run `dev github login` again")
	}
	if stubStore.deletedAccount != strings.TrimPrefix(server.URL, "http://") {
		t.Fatalf("deleted account = %q, want host from %q", stubStore.deletedAccount, server.URL)
	}
}

func TestGitHubAuthServiceRefreshReturnsErrorOnServerFailure(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("upstream failure")); err != nil {
			t.Fatalf("Write() returned error: %v", err)
		}
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	stubStore := &stubGitHubTokenStore{
		loadToken: GitHubStoredToken{
			AccessToken:           "access-token",
			RefreshToken:          "refresh-token",
			RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
			Host:                  strings.TrimPrefix(server.URL, "http://"),
		},
	}
	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore:    stubStore,
		refreshLeeway: githubRefreshLeeway,
	}

	err := service.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected Refresh() to return an error")
	}

	if !strings.Contains(err.Error(), "upstream failure") {
		t.Fatalf("error = %q, want upstream failure", err.Error())
	}
	if stubStore.deletedAccount != "" {
		t.Fatalf("deleted account = %q, want empty", stubStore.deletedAccount)
	}
}

func TestGitHubAuthServiceEnsureValidTokenReturnsExistingAccessToken(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token",
				RefreshToken:          "refresh-token",
				AccessTokenExpiresAt:  timePointer(now.Add(10 * time.Minute)),
				RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
				Host:                  "github.com",
			},
		},
		refreshLeeway: githubRefreshLeeway,
	}

	accessToken, err := service.EnsureValidToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureValidToken() returned error: %v", err)
	}

	if accessToken != "access-token" {
		t.Fatalf("access token = %q, want %q", accessToken, "access-token")
	}
}

func TestGitHubAuthServiceEnsureValidTokenRefreshesNearExpiryToken(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGitHubJSONResponse(t, w, `{"access_token":"access-token-2","token_type":"bearer","refresh_token":"refresh-token-2","expires_in":28800,"refresh_token_expires_in":15897600}`)
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  server.Client(),
		now: func() time.Time {
			return now
		},
		tokenStore: &stubGitHubTokenStore{
			loadToken: GitHubStoredToken{
				AccessToken:           "access-token-1",
				RefreshToken:          "refresh-token-1",
				AccessTokenExpiresAt:  timePointer(now.Add(4 * time.Minute)),
				RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
				Host:                  strings.TrimPrefix(server.URL, "http://"),
			},
		},
		refreshLeeway: githubRefreshLeeway,
	}

	accessToken, err := service.EnsureValidToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureValidToken() returned error: %v", err)
	}

	if accessToken != "access-token-2" {
		t.Fatalf("access token = %q, want %q", accessToken, "access-token-2")
	}
}

func TestGitHubAuthServiceEnsureValidTokenReturnsErrorOnInvalidLocalState(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	service := &GitHubAuthService{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		tokenStore: &stubGitHubTokenStore{
			loadErr: ErrGitHubStoredTokenInvalid,
		},
		refreshLeeway: githubRefreshLeeway,
	}

	_, err := service.EnsureValidToken(context.Background())
	if err == nil {
		t.Fatal("expected EnsureValidToken() to return an error")
	}

	if got := err.Error(); got != "local GitHub token state is invalid; please run `dev github login` again" {
		t.Fatalf("error = %q, want %q", got, "local GitHub token state is invalid; please run `dev github login` again")
	}
}

func TestGitHubRefreshRunnerRunRefreshesTokenAndWritesExpirations(t *testing.T) {
	useTestLocalTimezone(t)

	initializer := newGitHubLoginTestInitializer(t)
	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGitHubJSONResponse(t, w, `{"access_token":"access-token-2","token_type":"bearer","refresh_token":"refresh-token-2","expires_in":28800,"refresh_token_expires_in":15897600}`)
	}))
	defer server.Close()

	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
	runner := &GitHubRefreshRunner{
		service: &GitHubAuthService{
			initializer: initializer,
			httpClient:  server.Client(),
			now: func() time.Time {
				return now
			},
			tokenStore: &stubGitHubTokenStore{
				loadToken: GitHubStoredToken{
					AccessToken:           "access-token-1",
					RefreshToken:          "refresh-token-1",
					RefreshTokenExpiresAt: timePointer(now.Add(24 * time.Hour)),
					Host:                  strings.TrimPrefix(server.URL, "http://"),
				},
			},
			refreshLeeway: githubRefreshLeeway,
		},
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	commandOutput := output.String()
	if !strings.Contains(commandOutput, "GitHub token refresh succeeded. Token saved to the macOS keychain") {
		t.Fatalf("output = %q, want success message", commandOutput)
	}
	if !strings.Contains(commandOutput, "Access token expires at 2026-03-14T04:00:00+08:00 (Asia/Shanghai).") {
		t.Fatalf("output = %q, want access token expiration in local time", commandOutput)
	}
	if !strings.Contains(commandOutput, "Refresh token expires at 2026-09-13T20:00:00+08:00 (Asia/Shanghai).") {
		t.Fatalf("output = %q, want refresh token expiration in local time", commandOutput)
	}
}

func TestGitHubRefreshCommandRequiresNoArgs(t *testing.T) {
	cmd := newGitHubRefreshCmd(&GitHubRefreshRunner{})
	cmd.SetArgs([]string{"unexpected"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected Execute() to return an error")
	}

	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg(s), received 1") {
		t.Fatalf("unexpected error: %q", err.Error())
	}
}
