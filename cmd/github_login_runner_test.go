package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

func TestGitHubLoginRunnerRunExchangesDeviceCodeAndStoresToken(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
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

	var deviceCodeRequests int
	var accessTokenRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case githubDeviceCodePath:
			deviceCodeRequests++
			assertGitHubFormRequest(t, r, map[string]string{
				"client_id": "client-123",
			})
			writeGitHubJSONResponse(t, w, `{"device_code":"device-123","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":900,"interval":2}`)
		case githubAccessTokenPath:
			accessTokenRequests++
			assertGitHubFormRequest(t, r, map[string]string{
				"client_id":   "client-123",
				"device_code": "device-123",
				"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			})

			switch accessTokenRequests {
			case 1:
				writeGitHubJSONResponse(t, w, `{"error":"authorization_pending"}`)
			case 2:
				writeGitHubJSONResponse(t, w, `{"error":"slow_down"}`)
			default:
				writeGitHubJSONResponse(t, w, `{"access_token":"access-token","token_type":"bearer","refresh_token":"refresh-token","expires_in":28800,"refresh_token_expires_in":15897600}`)
			}
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}
	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	stubStore := &stubGitHubTokenStore{}
	var openedURL string
	var sleepDurations []time.Duration
	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)

	runner := &GitHubLoginRunner{
		initializer: initializer,
		httpClient:  server.Client(),
		browserOpener: func(rawURL string) error {
			openedURL = rawURL
			return nil
		},
		sleep: func(duration time.Duration) {
			sleepDurations = append(sleepDurations, duration)
		},
		now: func() time.Time {
			return now
		},
		tokenStore: stubStore,
	}

	var output bytes.Buffer
	if err := runner.Run(context.Background(), &output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if deviceCodeRequests != 1 {
		t.Fatalf("device code request count = %d, want 1", deviceCodeRequests)
	}
	if accessTokenRequests != 3 {
		t.Fatalf("access token request count = %d, want 3", accessTokenRequests)
	}
	if openedURL != "https://github.com/login/device" {
		t.Fatalf("opened URL = %q, want %q", openedURL, "https://github.com/login/device")
	}

	wantSleepDurations := []time.Duration{2 * time.Second, 7 * time.Second}
	if !reflect.DeepEqual(sleepDurations, wantSleepDurations) {
		t.Fatalf("sleep durations = %#v, want %#v", sleepDurations, wantSleepDurations)
	}

	if stubStore.savedAccount != strings.TrimPrefix(server.URL, "http://") && stubStore.savedAccount != strings.TrimPrefix(server.URL, "https://") {
		t.Fatalf("saved account = %q, want host from %q", stubStore.savedAccount, server.URL)
	}

	if stubStore.savedToken.AccessToken != "access-token" {
		t.Fatalf("saved access token = %q, want %q", stubStore.savedToken.AccessToken, "access-token")
	}
	if stubStore.savedToken.RefreshToken != "refresh-token" {
		t.Fatalf("saved refresh token = %q, want %q", stubStore.savedToken.RefreshToken, "refresh-token")
	}
	if stubStore.savedToken.Host != stubStore.savedAccount {
		t.Fatalf("saved host = %q, want %q", stubStore.savedToken.Host, stubStore.savedAccount)
	}
	if stubStore.savedToken.AccessTokenExpiresAt == nil {
		t.Fatal("expected access token expiration to be set")
	}
	if got, want := stubStore.savedToken.AccessTokenExpiresAt.Format(time.RFC3339), "2026-03-13T20:00:00Z"; got != want {
		t.Fatalf("access token expires at = %q, want %q", got, want)
	}
	if stubStore.savedToken.RefreshTokenExpiresAt == nil {
		t.Fatal("expected refresh token expiration to be set")
	}

	commandOutput := output.String()
	if !strings.Contains(commandOutput, "Open https://github.com/login/device and enter this code: ABCD-EFGH") {
		t.Fatalf("output = %q, want device instructions", commandOutput)
	}
	if !strings.Contains(commandOutput, "Opened the browser automatically.") {
		t.Fatalf("output = %q, want browser success message", commandOutput)
	}
	if !strings.Contains(commandOutput, "GitHub login succeeded. Token saved to the macOS keychain") {
		t.Fatalf("output = %q, want success message", commandOutput)
	}
	if !strings.Contains(commandOutput, "Access token expires at 2026-03-14T04:00:00+08:00 (Asia/Shanghai).") {
		t.Fatalf("output = %q, want access token expiration in local time", commandOutput)
	}
	if !strings.Contains(commandOutput, "Refresh token expires at 2026-09-13T20:00:00+08:00 (Asia/Shanghai).") {
		t.Fatalf("output = %q, want refresh token expiration in local time", commandOutput)
	}
}

func TestGitHubLoginRunnerRunReturnsErrorWhenDeviceCodeExpires(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case githubDeviceCodePath:
			writeGitHubJSONResponse(t, w, `{"device_code":"device-123","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`)
		case githubAccessTokenPath:
			writeGitHubJSONResponse(t, w, `{"error":"expired_token"}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := initializer.SetValue("github.client_id", "client-123"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}
	if err := initializer.SetValue("github.api_base_url", server.URL+"/api/v3"); err != nil {
		t.Fatalf("SetValue() returned error: %v", err)
	}

	runner := &GitHubLoginRunner{
		initializer: initializer,
		httpClient:  server.Client(),
		browserOpener: func(rawURL string) error {
			return nil
		},
		sleep: func(duration time.Duration) {},
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		tokenStore: &stubGitHubTokenStore{},
	}

	err := runner.Run(context.Background(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected Run() to return an error")
	}

	if got := err.Error(); got != "device code expired; please run `dev github login` again" {
		t.Fatalf("error = %q, want %q", got, "device code expired; please run `dev github login` again")
	}
}

func TestGitHubLoginRunnerRunReturnsErrorWhenClientIDIsEmpty(t *testing.T) {
	initializer := newGitHubLoginTestInitializer(t)

	runner := &GitHubLoginRunner{
		initializer: initializer,
		httpClient:  http.DefaultClient,
		browserOpener: func(rawURL string) error {
			return nil
		},
		sleep: func(duration time.Duration) {},
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		tokenStore: &stubGitHubTokenStore{},
	}

	err := runner.Run(context.Background(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected Run() to return an error")
	}

	if got := err.Error(); got != "config value github.client_id is empty" {
		t.Fatalf("error = %q, want %q", got, "config value github.client_id is empty")
	}
}

func TestGitHubAuthBaseURLMapsPublicAndEnterpriseHosts(t *testing.T) {
	testCases := []struct {
		name       string
		apiBaseURL string
		want       string
	}{
		{
			name:       "public GitHub",
			apiBaseURL: "https://api.github.com",
			want:       "https://github.com",
		},
		{
			name:       "enterprise host",
			apiBaseURL: "https://ghe.example.com/api/v3",
			want:       "https://ghe.example.com",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authBaseURL, err := githubAuthBaseURL(testCase.apiBaseURL)
			if err != nil {
				t.Fatalf("githubAuthBaseURL() returned error: %v", err)
			}

			if authBaseURL.String() != testCase.want {
				t.Fatalf("githubAuthBaseURL() = %q, want %q", authBaseURL.String(), testCase.want)
			}
		})
	}
}

func TestGitHubLoginCommandRequiresNoArgs(t *testing.T) {
	cmd := newGitHubLoginCmd(&GitHubLoginRunner{})
	cmd.SetArgs([]string{"unexpected"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected Execute() to return an error")
	}

	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg(s), received 1") {
		t.Fatalf("unexpected error: %q", err.Error())
	}
}

type stubGitHubTokenStore struct {
	savedAccount   string
	savedToken     GitHubStoredToken
	loadAccount    string
	loadToken      GitHubStoredToken
	loadErr        error
	saveErr        error
	deletedAccount string
	deleteErr      error
}

func (s *stubGitHubTokenStore) Save(account string, token GitHubStoredToken) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.savedAccount = account
	s.savedToken = token
	s.loadAccount = account
	s.loadToken = token
	s.loadErr = nil
	return nil
}

func (s *stubGitHubTokenStore) Load(account string) (GitHubStoredToken, error) {
	s.loadAccount = account
	return s.loadToken, s.loadErr
}

func (s *stubGitHubTokenStore) Delete(account string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	s.deletedAccount = account
	s.loadAccount = account
	s.loadToken = GitHubStoredToken{}
	s.loadErr = ErrGitHubTokenNotFound

	return nil
}

func newGitHubLoginTestInitializer(t *testing.T) *ConfigInitializer {
	t.Helper()

	initializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
	}

	if _, err := initializer.Init(); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	return initializer
}

func assertGitHubFormRequest(t *testing.T, request *http.Request, wantForm map[string]string) {
	t.Helper()

	if request.Method != http.MethodPost {
		t.Fatalf("request method = %q, want %q", request.Method, http.MethodPost)
	}
	if request.Header.Get("Accept") != "application/json" {
		t.Fatalf("Accept header = %q, want %q", request.Header.Get("Accept"), "application/json")
	}
	if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type header = %q, want %q", request.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
	}

	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() returned error: %v", err)
	}

	for key, wantValue := range wantForm {
		if gotValue := request.PostFormValue(key); gotValue != wantValue {
			t.Fatalf("form value %q = %q, want %q", key, gotValue, wantValue)
		}
	}
}

func writeGitHubJSONResponse(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()

	writer.Header().Set("Content-Type", "application/json")
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}
