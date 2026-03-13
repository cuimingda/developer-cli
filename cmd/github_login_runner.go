package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const (
	githubDeviceCodePath            = "/login/device/code"
	githubAccessTokenPath           = "/login/oauth/access_token"
	githubDefaultPollInterval       = 5 * time.Second
	githubSlowDownIntervalIncrement = 5 * time.Second
)

type GitHubLoginRunner struct {
	initializer   *ConfigInitializer
	httpClient    *http.Client
	browserOpener func(rawURL string) error
	sleep         func(time.Duration)
	now           func() time.Time
	tokenStore    GitHubTokenStore
}

type githubLoginConfig struct {
	ClientID    string
	APIBaseURL  string
	AuthBaseURL *url.URL
	Account     string
}

type gitHubDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type gitHubAccessTokenResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
	ErrorURI              string `json:"error_uri"`
}

func newGitHubLoginRunner(initializer *ConfigInitializer) *GitHubLoginRunner {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	return &GitHubLoginRunner{
		initializer: initializer,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		browserOpener: openBrowser,
		sleep:         time.Sleep,
		now: func() time.Time {
			return time.Now().UTC()
		},
		tokenStore: newKeychainGitHubTokenStore(),
	}
}

func (r *GitHubLoginRunner) Run(ctx context.Context, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := r.ensureDefaults(); err != nil {
		return err
	}

	config, err := r.loadConfig()
	if err != nil {
		return err
	}

	deviceCodeResponse, err := r.requestDeviceCode(ctx, config.AuthBaseURL, config.ClientID)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "Open %s and enter this code: %s\n", deviceCodeResponse.VerificationURI, deviceCodeResponse.UserCode); err != nil {
		return err
	}

	if err := r.browserOpener(deviceCodeResponse.VerificationURI); err != nil {
		if _, writeErr := fmt.Fprintf(stdout, "Could not open the browser automatically: %v\n", err); writeErr != nil {
			return writeErr
		}
	} else if _, err := fmt.Fprintln(stdout, "Opened the browser automatically."); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(stdout, "Waiting for GitHub authorization..."); err != nil {
		return err
	}

	accessTokenResponse, err := r.pollForAccessToken(
		ctx,
		config.AuthBaseURL,
		config.ClientID,
		deviceCodeResponse.DeviceCode,
		time.Duration(deviceCodeResponse.Interval)*time.Second,
		time.Duration(deviceCodeResponse.ExpiresIn)*time.Second,
	)
	if err != nil {
		return err
	}

	issuedAt := r.now().UTC()
	storedToken := buildGitHubStoredToken(config.APIBaseURL, config.AuthBaseURL.Host, accessTokenResponse, issuedAt)
	if err := r.tokenStore.Save(config.Account, storedToken); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "GitHub login succeeded. Token saved to the macOS keychain for %s.\n", config.Account); err != nil {
		return err
	}
	if storedToken.AccessTokenExpiresAt != nil {
		if _, err := fmt.Fprintf(stdout, "Access token expires at %s.\n", formatLocalTimeDisplay(*storedToken.AccessTokenExpiresAt)); err != nil {
			return err
		}
	}
	if storedToken.RefreshTokenExpiresAt != nil {
		if _, err := fmt.Fprintf(stdout, "Refresh token expires at %s.\n", formatLocalTimeDisplay(*storedToken.RefreshTokenExpiresAt)); err != nil {
			return err
		}
	}

	return nil
}

func (r *GitHubLoginRunner) ensureDefaults() error {
	if r.initializer == nil {
		r.initializer = newDefaultConfigInitializer()
	}
	if r.httpClient == nil {
		r.httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	if r.browserOpener == nil {
		r.browserOpener = openBrowser
	}
	if r.sleep == nil {
		r.sleep = time.Sleep
	}
	if r.now == nil {
		r.now = func() time.Time {
			return time.Now().UTC()
		}
	}
	if r.tokenStore == nil {
		r.tokenStore = newKeychainGitHubTokenStore()
	}

	return nil
}

func (r *GitHubLoginRunner) loadConfig() (githubLoginConfig, error) {
	return loadGitHubLoginConfig(r.initializer)
}

func (r *GitHubLoginRunner) requestDeviceCode(ctx context.Context, authBaseURL *url.URL, clientID string) (gitHubDeviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)

	var response gitHubDeviceCodeResponse
	if err := r.postForm(ctx, githubAuthEndpoint(authBaseURL, githubDeviceCodePath), form, &response); err != nil {
		return gitHubDeviceCodeResponse{}, fmt.Errorf("request device code: %w", err)
	}
	if strings.TrimSpace(response.DeviceCode) == "" {
		return gitHubDeviceCodeResponse{}, fmt.Errorf("request device code: missing device_code in response")
	}
	if strings.TrimSpace(response.UserCode) == "" {
		return gitHubDeviceCodeResponse{}, fmt.Errorf("request device code: missing user_code in response")
	}
	if strings.TrimSpace(response.VerificationURI) == "" {
		return gitHubDeviceCodeResponse{}, fmt.Errorf("request device code: missing verification_uri in response")
	}
	if response.ExpiresIn <= 0 {
		return gitHubDeviceCodeResponse{}, fmt.Errorf("request device code: missing expires_in in response")
	}
	if response.Interval <= 0 {
		response.Interval = int(githubDefaultPollInterval / time.Second)
	}

	return response, nil
}

func (r *GitHubLoginRunner) pollForAccessToken(
	ctx context.Context,
	authBaseURL *url.URL,
	clientID string,
	deviceCode string,
	interval time.Duration,
	expiresAfter time.Duration,
) (gitHubAccessTokenResponse, error) {
	if interval <= 0 {
		interval = githubDefaultPollInterval
	}
	if expiresAfter <= 0 {
		return gitHubAccessTokenResponse{}, fmt.Errorf("device code expiration is invalid")
	}

	deadline := r.now().Add(expiresAfter)
	nextInterval := interval

	for {
		if !r.now().Before(deadline) {
			return gitHubAccessTokenResponse{}, fmt.Errorf("device code expired; please run `dev github login` again")
		}

		response, err := r.exchangeDeviceCode(ctx, authBaseURL, clientID, deviceCode)
		if err != nil {
			return gitHubAccessTokenResponse{}, err
		}
		if strings.TrimSpace(response.AccessToken) != "" {
			return response, nil
		}

		switch response.Error {
		case "authorization_pending":
			if err := r.wait(ctx, nextInterval); err != nil {
				return gitHubAccessTokenResponse{}, err
			}
		case "slow_down":
			nextInterval += githubSlowDownIntervalIncrement
			if err := r.wait(ctx, nextInterval); err != nil {
				return gitHubAccessTokenResponse{}, err
			}
		case "expired_token":
			return gitHubAccessTokenResponse{}, fmt.Errorf("device code expired; please run `dev github login` again")
		case "access_denied":
			return gitHubAccessTokenResponse{}, fmt.Errorf("authorization was denied by the user")
		default:
			if strings.TrimSpace(response.Error) == "" {
				return gitHubAccessTokenResponse{}, fmt.Errorf("GitHub access token response did not include an access_token")
			}

			return gitHubAccessTokenResponse{}, fmt.Errorf("GitHub token exchange failed: %s", formatGitHubTokenExchangeError(response))
		}
	}
}

func (r *GitHubLoginRunner) exchangeDeviceCode(
	ctx context.Context,
	authBaseURL *url.URL,
	clientID string,
	deviceCode string,
) (gitHubAccessTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	var response gitHubAccessTokenResponse
	if err := r.postForm(ctx, githubAuthEndpoint(authBaseURL, githubAccessTokenPath), form, &response); err != nil {
		return gitHubAccessTokenResponse{}, fmt.Errorf("exchange device code for access token: %w", err)
	}

	return response, nil
}

func (r *GitHubLoginRunner) postForm(ctx context.Context, endpoint string, form url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
		message := strings.TrimSpace(string(body))
		if message == "" {
			return fmt.Errorf("unexpected status: %s", response.Status)
		}

		return fmt.Errorf("unexpected status: %s: %s", response.Status, message)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (r *GitHubLoginRunner) wait(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.sleep(duration)

	return ctx.Err()
}

func buildGitHubStoredToken(
	apiBaseURL string,
	host string,
	response gitHubAccessTokenResponse,
	issuedAt time.Time,
) GitHubStoredToken {
	storedToken := GitHubStoredToken{
		APIBaseURL:   apiBaseURL,
		Host:         host,
		AccessToken:  response.AccessToken,
		TokenType:    response.TokenType,
		Scope:        response.Scope,
		IssuedAt:     issuedAt,
		RefreshToken: response.RefreshToken,
	}

	if response.ExpiresIn > 0 {
		accessTokenExpiresAt := issuedAt.Add(time.Duration(response.ExpiresIn) * time.Second)
		storedToken.AccessTokenExpiresAt = &accessTokenExpiresAt
	}
	if response.RefreshTokenExpiresIn > 0 {
		refreshTokenExpiresAt := issuedAt.Add(time.Duration(response.RefreshTokenExpiresIn) * time.Second)
		storedToken.RefreshTokenExpiresAt = &refreshTokenExpiresAt
	}

	return storedToken
}

func githubAuthBaseURL(apiBaseURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(apiBaseURL)
	if trimmed == "" {
		return nil, fmt.Errorf("config value github.api_base_url is empty")
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse github.api_base_url: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("config value github.api_base_url is invalid: %s", apiBaseURL)
	}

	resolvedHost := parsedURL.Host
	if strings.EqualFold(parsedURL.Hostname(), "api.github.com") {
		resolvedHost = "github.com"
		if port := parsedURL.Port(); port != "" {
			resolvedHost = net.JoinHostPort("github.com", port)
		}
	}

	return &url.URL{
		Scheme: parsedURL.Scheme,
		Host:   resolvedHost,
	}, nil
}

func githubAuthEndpoint(baseURL *url.URL, path string) string {
	return (&url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
		Path:   path,
	}).String()
}

func formatGitHubTokenExchangeError(response gitHubAccessTokenResponse) string {
	parts := []string{response.Error}
	if strings.TrimSpace(response.ErrorDescription) != "" {
		parts = append(parts, response.ErrorDescription)
	}
	if strings.TrimSpace(response.ErrorURI) != "" {
		parts = append(parts, response.ErrorURI)
	}

	return strings.Join(parts, ": ")
}

func openBrowser(rawURL string) error {
	output, err := exec.Command("open", rawURL).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}

		return fmt.Errorf("%w: %s", err, message)
	}

	return nil
}
