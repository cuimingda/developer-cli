package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	githubCurrentUserPath                  = "/user"
	githubUserInstallationsPath            = "/user/installations"
	githubRESTAcceptHeader                 = "application/vnd.github+json"
	githubAccessTokenExpiringSoonThreshold = 15 * time.Minute
)

type GitHubAuthState string

const (
	GitHubAuthStateNotLoggedIn                   GitHubAuthState = "not_logged_in"
	GitHubAuthStateTokenValid                    GitHubAuthState = "token_valid"
	GitHubAuthStateAccessTokenExpiredRefreshable GitHubAuthState = "access_token_expired_refreshable"
	GitHubAuthStateAuthorizationInvalid          GitHubAuthState = "authorization_invalid"
	GitHubAuthStateReauthenticationRequired      GitHubAuthState = "reauthentication_required"
	GitHubAuthStateIndeterminate                 GitHubAuthState = "indeterminate"
)

type GitHubAuthNextAction string

const (
	GitHubAuthNextActionCallAPI   GitHubAuthNextAction = "call_api"
	GitHubAuthNextActionRefresh   GitHubAuthNextAction = "refresh"
	GitHubAuthNextActionLogin     GitHubAuthNextAction = "login"
	GitHubAuthNextActionFixConfig GitHubAuthNextAction = "fix_config"
	GitHubAuthNextActionRetry     GitHubAuthNextAction = "retry"
)

type GitHubRemoteProbeState string

const (
	GitHubRemoteProbeSkipped      GitHubRemoteProbeState = "skipped"
	GitHubRemoteProbeSucceeded    GitHubRemoteProbeState = "succeeded"
	GitHubRemoteProbeUnauthorized GitHubRemoteProbeState = "unauthorized"
	GitHubRemoteProbeFailed       GitHubRemoteProbeState = "failed"
)

type GitHubAppInstallationState string

const (
	GitHubAppInstallationUnknown      GitHubAppInstallationState = "unknown"
	GitHubAppInstallationInstalled    GitHubAppInstallationState = "installed"
	GitHubAppInstallationNotInstalled GitHubAppInstallationState = "not_installed"
)

type GitHubAuthStatusRunner struct {
	initializer           *ConfigInitializer
	httpClient            *http.Client
	now                   func() time.Time
	tokenStore            GitHubTokenStore
	expiringSoonThreshold time.Duration
}

type GitHubAuthStatusReport struct {
	State                 GitHubAuthState
	NextAction            GitHubAuthNextAction
	Summary               string
	APIBaseURL            string
	Host                  string
	Username              string
	ClientIDPresent       bool
	AccessTokenPresent    bool
	RefreshTokenPresent   bool
	AccessTokenExpired    bool
	AccessTokenNearExpiry bool
	RefreshTokenExpired   bool
	AccessTokenExpiresAt  *time.Time
	RefreshTokenExpiresAt *time.Time
	RemoteProbeState      GitHubRemoteProbeState
	RemoteProbeStatusCode int
	RemoteProbeStatus     string
	RemoteProbeMessage    string
	AppInstallationState  GitHubAppInstallationState
	AppInstallationCount  int
	AppInstallationOwners []string
	AppInstallationReason string
}

type gitHubCurrentUserResponse struct {
	Login string `json:"login"`
}

type gitHubAPIErrorResponse struct {
	Message string `json:"message"`
}

type gitHubUserInstallationsResponse struct {
	TotalCount    int                      `json:"total_count"`
	Installations []gitHubUserInstallation `json:"installations"`
}

type gitHubUserInstallation struct {
	Account    gitHubInstallationAccount `json:"account"`
	TargetType string                    `json:"target_type"`
}

type gitHubInstallationAccount struct {
	Login string `json:"login"`
}

type gitHubAppInstallationProbe struct {
	State  GitHubAppInstallationState
	Count  int
	Owners []string
}

func newGitHubAuthStatusRunner(initializer *ConfigInitializer) *GitHubAuthStatusRunner {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	return &GitHubAuthStatusRunner{
		initializer: initializer,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
		tokenStore:            newKeychainGitHubTokenStore(),
		expiringSoonThreshold: githubAccessTokenExpiringSoonThreshold,
	}
}

func (r *GitHubAuthStatusRunner) Run(ctx context.Context, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}

	report, err := r.Evaluate(ctx)
	if err != nil {
		return err
	}

	return writeGitHubAuthStatusReport(stdout, report, r.now())
}

func (r *GitHubAuthStatusRunner) Evaluate(ctx context.Context) (GitHubAuthStatusReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.ensureDefaults(); err != nil {
		return GitHubAuthStatusReport{}, err
	}

	report, err := r.loadLocalStatus()
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}

	if !report.AccessTokenPresent {
		report.State = GitHubAuthStateNotLoggedIn
		report.Summary = "Not logged in: no access token is stored in the macOS keychain."
		report.NextAction = nextActionWithoutAccessToken(report.ClientIDPresent)
		report.RemoteProbeState = GitHubRemoteProbeSkipped
		report.RemoteProbeMessage = "skipped because no access token is stored locally"
		report.AppInstallationState = GitHubAppInstallationUnknown
		report.AppInstallationReason = "skipped because no access token is stored locally"
		return report, nil
	}

	if report.AccessTokenExpired {
		report.RemoteProbeState = GitHubRemoteProbeSkipped
		report.RemoteProbeMessage = "skipped because the access token is already expired"
		report.AppInstallationState = GitHubAppInstallationUnknown
		report.AppInstallationReason = "skipped because the access token is already expired"
		if report.RefreshTokenPresent && !report.RefreshTokenExpired {
			report.State = GitHubAuthStateAccessTokenExpiredRefreshable
			report.Summary = "Logged in, but the access token has expired and can be refreshed."
			report.NextAction = nextActionWithRefreshToken(report.ClientIDPresent)
			return report, nil
		}

		report.State = GitHubAuthStateReauthenticationRequired
		report.Summary = "Logged in, but the access token has expired and the refresh token is unavailable or expired."
		report.NextAction = nextActionWithoutRefreshToken(report.ClientIDPresent)
		return report, nil
	}

	remoteProbe, err := r.probeCurrentUser(ctx, report.APIBaseURL, report.Host)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return GitHubAuthStatusReport{}, err
		}

		report.State = GitHubAuthStateIndeterminate
		report.Summary = "Logged in locally, but the remote probe could not confirm token validity."
		report.NextAction = GitHubAuthNextActionRetry
		report.RemoteProbeState = GitHubRemoteProbeFailed
		report.RemoteProbeMessage = err.Error()
		report.AppInstallationState = GitHubAppInstallationUnknown
		report.AppInstallationReason = "skipped because token validity could not be confirmed"
		return report, nil
	}

	report.Username = remoteProbe.Username
	report.RemoteProbeState = remoteProbe.RemoteProbeState
	report.RemoteProbeStatusCode = remoteProbe.RemoteProbeStatusCode
	report.RemoteProbeStatus = remoteProbe.RemoteProbeStatus
	report.RemoteProbeMessage = remoteProbe.RemoteProbeMessage

	switch remoteProbe.RemoteProbeState {
	case GitHubRemoteProbeSucceeded:
		report.State = GitHubAuthStateTokenValid
		if report.AccessTokenNearExpiry {
			report.Summary = "Logged in, token is valid and expires soon."
		} else {
			report.Summary = "Logged in, token is valid."
		}
		report.NextAction = GitHubAuthNextActionCallAPI
		installationProbe, err := r.probeAppInstallations(ctx, report.APIBaseURL, report.Host)
		if err != nil {
			report.AppInstallationState = GitHubAppInstallationUnknown
			report.AppInstallationReason = err.Error()
			return report, nil
		}
		report.AppInstallationState = installationProbe.State
		report.AppInstallationCount = installationProbe.Count
		report.AppInstallationOwners = installationProbe.Owners
	case GitHubRemoteProbeUnauthorized:
		report.State = GitHubAuthStateAuthorizationInvalid
		report.Summary = "Logged in, but authorization is no longer valid."
		report.NextAction = nextActionWithoutRefreshToken(report.ClientIDPresent)
		if err := r.tokenStore.Delete(report.Host); err == nil {
			report.Summary = "GitHub authorization is no longer valid. Local token state was cleared."
			report.AccessTokenPresent = false
			report.RefreshTokenPresent = false
			report.AccessTokenExpired = false
			report.AccessTokenNearExpiry = false
			report.RefreshTokenExpired = false
			report.AccessTokenExpiresAt = nil
			report.RefreshTokenExpiresAt = nil
		} else {
			report.RemoteProbeMessage = appendStatusDetail(report.RemoteProbeMessage, "failed to clear local token state: "+err.Error())
		}
		report.AppInstallationState = GitHubAppInstallationUnknown
		report.AppInstallationReason = "skipped because authorization is no longer valid"
	default:
		report.State = GitHubAuthStateIndeterminate
		report.Summary = "Logged in locally, but the remote probe could not confirm token validity."
		report.NextAction = GitHubAuthNextActionRetry
		report.AppInstallationState = GitHubAppInstallationUnknown
		report.AppInstallationReason = "skipped because token validity could not be confirmed"
	}

	return report, nil
}

func (r *GitHubAuthStatusRunner) ensureDefaults() error {
	if r.initializer == nil {
		r.initializer = newDefaultConfigInitializer()
	}
	if r.httpClient == nil {
		r.httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	if r.now == nil {
		r.now = func() time.Time {
			return time.Now().UTC()
		}
	}
	if r.tokenStore == nil {
		r.tokenStore = newKeychainGitHubTokenStore()
	}
	if r.expiringSoonThreshold <= 0 {
		r.expiringSoonThreshold = githubAccessTokenExpiringSoonThreshold
	}

	return nil
}

func (r *GitHubAuthStatusRunner) loadLocalStatus() (GitHubAuthStatusReport, error) {
	clientID, clientIDPresent, err := optionalConfigValue(r.initializer, "github.client_id")
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}

	apiBaseURL, err := r.initializer.GetValue("github.api_base_url")
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}
	authBaseURL, err := githubAuthBaseURL(apiBaseURL)
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}

	report := GitHubAuthStatusReport{
		APIBaseURL:      strings.TrimSpace(apiBaseURL),
		Host:            authBaseURL.Host,
		ClientIDPresent: clientIDPresent && strings.TrimSpace(clientID) != "",
	}

	token, err := r.tokenStore.Load(report.Host)
	if errors.Is(err, ErrGitHubTokenNotFound) {
		return report, nil
	}
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}

	now := r.now().UTC()
	report.AccessTokenPresent = strings.TrimSpace(token.AccessToken) != ""
	report.RefreshTokenPresent = strings.TrimSpace(token.RefreshToken) != ""
	report.AccessTokenExpiresAt = normalizeTimePointer(token.AccessTokenExpiresAt)
	report.RefreshTokenExpiresAt = normalizeTimePointer(token.RefreshTokenExpiresAt)
	report.AccessTokenExpired, report.AccessTokenNearExpiry = evaluateExpiry(report.AccessTokenExpiresAt, now, r.expiringSoonThreshold)
	report.RefreshTokenExpired, _ = evaluateExpiry(report.RefreshTokenExpiresAt, now, 0)

	return report, nil
}

func (r *GitHubAuthStatusRunner) probeCurrentUser(
	ctx context.Context,
	apiBaseURL string,
	account string,
) (GitHubAuthStatusReport, error) {
	token, err := r.tokenStore.Load(account)
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}

	endpoint, err := githubAPIEndpoint(apiBaseURL, githubCurrentUserPath)
	if err != nil {
		return GitHubAuthStatusReport{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GitHubAuthStatusReport{}, fmt.Errorf("build remote probe request: %w", err)
	}

	request.Header.Set("Accept", githubRESTAcceptHeader)
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)

	response, err := r.httpClient.Do(request)
	if err != nil {
		return GitHubAuthStatusReport{}, fmt.Errorf("send remote probe request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return GitHubAuthStatusReport{}, fmt.Errorf("read remote probe response: %w", err)
	}

	probe := GitHubAuthStatusReport{
		RemoteProbeStatusCode: response.StatusCode,
		RemoteProbeStatus:     response.Status,
	}

	switch response.StatusCode {
	case http.StatusOK:
		var currentUser gitHubCurrentUserResponse
		if err := json.Unmarshal(body, &currentUser); err != nil {
			return GitHubAuthStatusReport{}, fmt.Errorf("decode remote probe response: %w", err)
		}
		if strings.TrimSpace(currentUser.Login) == "" {
			return GitHubAuthStatusReport{}, fmt.Errorf("remote probe response did not include a login")
		}

		probe.RemoteProbeState = GitHubRemoteProbeSucceeded
		probe.Username = currentUser.Login
		probe.RemoteProbeMessage = "GET /user succeeded"
	case http.StatusUnauthorized:
		probe.RemoteProbeState = GitHubRemoteProbeUnauthorized
		probe.RemoteProbeMessage = decodeGitHubAPIErrorMessage(body)
	default:
		probe.RemoteProbeState = GitHubRemoteProbeFailed
		probe.RemoteProbeMessage = response.Status
		if message := decodeGitHubAPIErrorMessage(body); message != "" {
			probe.RemoteProbeMessage = response.Status + ": " + message
		}
	}

	return probe, nil
}

func (r *GitHubAuthStatusRunner) probeAppInstallations(
	ctx context.Context,
	apiBaseURL string,
	account string,
) (gitHubAppInstallationProbe, error) {
	token, err := r.tokenStore.Load(account)
	if err != nil {
		return gitHubAppInstallationProbe{}, err
	}

	endpoint, err := githubAPIEndpoint(apiBaseURL, githubUserInstallationsPath)
	if err != nil {
		return gitHubAppInstallationProbe{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return gitHubAppInstallationProbe{}, fmt.Errorf("build app installation request: %w", err)
	}

	request.Header.Set("Accept", githubRESTAcceptHeader)
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)

	response, err := r.httpClient.Do(request)
	if err != nil {
		return gitHubAppInstallationProbe{}, fmt.Errorf("send app installation request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return gitHubAppInstallationProbe{}, fmt.Errorf("read app installation response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		message := response.Status
		if apiMessage := decodeGitHubAPIErrorMessage(body); apiMessage != "" {
			message = response.Status + ": " + apiMessage
		}

		return gitHubAppInstallationProbe{}, fmt.Errorf("check app installation status: %s", message)
	}

	var installationsResponse gitHubUserInstallationsResponse
	if err := json.Unmarshal(body, &installationsResponse); err != nil {
		return gitHubAppInstallationProbe{}, fmt.Errorf("decode app installation response: %w", err)
	}

	count := installationsResponse.TotalCount
	if count == 0 {
		count = len(installationsResponse.Installations)
	}
	if count == 0 {
		return gitHubAppInstallationProbe{
			State: GitHubAppInstallationNotInstalled,
		}, nil
	}

	return gitHubAppInstallationProbe{
		State:  GitHubAppInstallationInstalled,
		Count:  count,
		Owners: summarizeInstallationOwners(installationsResponse.Installations),
	}, nil
}

func writeGitHubAuthStatusReport(writer io.Writer, report GitHubAuthStatusReport, now time.Time) error {
	lines := []string{
		fmt.Sprintf("Status: %s", report.Summary),
		fmt.Sprintf("Recommended next step: %s", nextActionDescription(report.NextAction)),
		fmt.Sprintf("Host: %s", report.Host),
	}

	if strings.TrimSpace(report.Username) != "" {
		lines = append(lines, fmt.Sprintf("Username: %s", report.Username))
	}

	lines = append(lines,
		"",
		"Local checks:",
		fmt.Sprintf("- github.client_id: %s", presentOrMissing(report.ClientIDPresent)),
		fmt.Sprintf("- access token: %s", tokenPresenceDescription(report.AccessTokenPresent)),
		fmt.Sprintf("- refresh token: %s", tokenPresenceDescription(report.RefreshTokenPresent)),
		"",
		"Time checks:",
		fmt.Sprintf("- access token: %s", tokenTimeDescription(report.AccessTokenPresent, report.AccessTokenExpiresAt, report.AccessTokenExpired, report.AccessTokenNearExpiry, now)),
		fmt.Sprintf("- refresh token: %s", refreshTokenTimeDescription(report.RefreshTokenPresent, report.RefreshTokenExpiresAt, report.RefreshTokenExpired, now)),
		"",
		"Remote check:",
		fmt.Sprintf("- GET /user: %s", remoteProbeDescription(report)),
		fmt.Sprintf("- GitHub App installed: %s", appInstallationDescription(report)),
	)

	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}

	return nil
}

func nextActionWithoutAccessToken(clientIDPresent bool) GitHubAuthNextAction {
	if clientIDPresent {
		return GitHubAuthNextActionLogin
	}

	return GitHubAuthNextActionFixConfig
}

func nextActionWithRefreshToken(clientIDPresent bool) GitHubAuthNextAction {
	if clientIDPresent {
		return GitHubAuthNextActionRefresh
	}

	return GitHubAuthNextActionFixConfig
}

func nextActionWithoutRefreshToken(clientIDPresent bool) GitHubAuthNextAction {
	if clientIDPresent {
		return GitHubAuthNextActionLogin
	}

	return GitHubAuthNextActionFixConfig
}

func optionalConfigValue(initializer *ConfigInitializer, key string) (string, bool, error) {
	value, err := initializer.GetValue(key)
	if err != nil {
		if strings.Contains(err.Error(), "config key not found: "+key) {
			return "", false, nil
		}
		return "", false, err
	}

	trimmedValue := strings.TrimSpace(value)
	return trimmedValue, trimmedValue != "", nil
}

func evaluateExpiry(expiresAt *time.Time, now time.Time, nearExpiryThreshold time.Duration) (bool, bool) {
	if expiresAt == nil {
		return false, false
	}

	normalized := expiresAt.UTC()
	if !now.Before(normalized) {
		return true, false
	}
	if nearExpiryThreshold <= 0 {
		return false, false
	}

	return false, normalized.Sub(now) <= nearExpiryThreshold
}

func normalizeTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	normalized := value.UTC()
	return &normalized
}

func githubAPIEndpoint(apiBaseURL string, path string) (string, error) {
	trimmedBaseURL := strings.TrimSpace(apiBaseURL)
	if trimmedBaseURL == "" {
		return "", fmt.Errorf("config value github.api_base_url is empty")
	}

	parsedURL, err := url.Parse(trimmedBaseURL)
	if err != nil {
		return "", fmt.Errorf("parse github.api_base_url: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", fmt.Errorf("config value github.api_base_url is invalid: %s", apiBaseURL)
	}

	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}

	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + normalizedPath
	parsedURL.RawPath = ""

	return parsedURL.String(), nil
}

func decodeGitHubAPIErrorMessage(body []byte) string {
	var errorResponse gitHubAPIErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err != nil {
		return strings.TrimSpace(string(body))
	}

	return strings.TrimSpace(errorResponse.Message)
}

func nextActionDescription(action GitHubAuthNextAction) string {
	switch action {
	case GitHubAuthNextActionCallAPI:
		return "call the GitHub API directly"
	case GitHubAuthNextActionRefresh:
		return "refresh the access token before calling the API"
	case GitHubAuthNextActionLogin:
		return "run `dev github login`"
	case GitHubAuthNextActionFixConfig:
		return "configure `github.client_id` before logging in or refreshing"
	default:
		return "retry the status check after fixing the remote probe failure"
	}
}

func presentOrMissing(present bool) string {
	if present {
		return "present"
	}

	return "missing"
}

func tokenPresenceDescription(present bool) string {
	if present {
		return "present"
	}

	return "not found"
}

func tokenTimeDescription(present bool, expiresAt *time.Time, expired bool, nearExpiry bool, now time.Time) string {
	if !present {
		return "unavailable"
	}
	if expiresAt == nil {
		return "present, expiration unknown"
	}
	if expired {
		return fmt.Sprintf("expired at %s", formatLocalTimeDisplay(*expiresAt))
	}

	description := fmt.Sprintf("valid until %s (%s)", formatLocalTimeDisplay(*expiresAt), relativeTimeDescription(*expiresAt, now))
	if nearExpiry {
		return description + ", expires soon"
	}

	return description
}

func refreshTokenTimeDescription(present bool, expiresAt *time.Time, expired bool, now time.Time) string {
	if !present {
		return "unavailable"
	}
	if expiresAt == nil {
		return "present, expiration unknown"
	}
	if expired {
		return fmt.Sprintf("expired at %s", formatLocalTimeDisplay(*expiresAt))
	}

	return fmt.Sprintf("valid until %s (%s)", formatLocalTimeDisplay(*expiresAt), relativeTimeDescription(*expiresAt, now))
}

func relativeTimeDescription(target time.Time, now time.Time) string {
	duration := target.Sub(now)
	if duration < 0 {
		return fmt.Sprintf("%s ago", roundedDurationString(-duration))
	}

	return "in " + roundedDurationString(duration)
}

func roundedDurationString(duration time.Duration) string {
	if duration >= time.Minute {
		return duration.Round(time.Minute).String()
	}

	return duration.Round(time.Second).String()
}

func remoteProbeDescription(report GitHubAuthStatusReport) string {
	switch report.RemoteProbeState {
	case GitHubRemoteProbeSucceeded:
		return fmt.Sprintf("%s as %s", report.RemoteProbeStatus, report.Username)
	case GitHubRemoteProbeUnauthorized:
		if strings.TrimSpace(report.RemoteProbeMessage) != "" {
			return fmt.Sprintf("%s (%s)", report.RemoteProbeStatus, report.RemoteProbeMessage)
		}
		return report.RemoteProbeStatus
	case GitHubRemoteProbeSkipped:
		return report.RemoteProbeMessage
	default:
		if strings.TrimSpace(report.RemoteProbeMessage) != "" {
			return report.RemoteProbeMessage
		}
		if strings.TrimSpace(report.RemoteProbeStatus) != "" {
			return report.RemoteProbeStatus
		}
		return "unavailable"
	}
}

func appendStatusDetail(base string, detail string) string {
	base = strings.TrimSpace(base)
	detail = strings.TrimSpace(detail)

	switch {
	case base == "":
		return detail
	case detail == "":
		return base
	default:
		return base + "; " + detail
	}
}

func appInstallationDescription(report GitHubAuthStatusReport) string {
	switch report.AppInstallationState {
	case GitHubAppInstallationInstalled:
		if len(report.AppInstallationOwners) == 0 {
			return fmt.Sprintf("yes (%d accessible installation%s)", report.AppInstallationCount, pluralSuffix(report.AppInstallationCount))
		}

		return fmt.Sprintf(
			"yes (%d accessible installation%s: %s)",
			report.AppInstallationCount,
			pluralSuffix(report.AppInstallationCount),
			strings.Join(report.AppInstallationOwners, ", "),
		)
	case GitHubAppInstallationNotInstalled:
		return "no"
	default:
		reason := strings.TrimSpace(report.AppInstallationReason)
		if reason == "" {
			return "unknown"
		}

		return fmt.Sprintf("unknown (%s)", reason)
	}
}

func summarizeInstallationOwners(installations []gitHubUserInstallation) []string {
	if len(installations) == 0 {
		return nil
	}

	owners := make([]string, 0, len(installations))
	for _, installation := range installations {
		owner := strings.TrimSpace(installation.Account.Login)
		targetType := strings.TrimSpace(installation.TargetType)

		switch {
		case owner != "" && targetType != "":
			owners = append(owners, fmt.Sprintf("%s (%s)", owner, targetType))
		case owner != "":
			owners = append(owners, owner)
		case targetType != "":
			owners = append(owners, targetType)
		}
	}

	if len(owners) <= 3 {
		return owners
	}

	return append(owners[:3], fmt.Sprintf("+%d more", len(owners)-3))
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}

	return "s"
}
