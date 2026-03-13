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

const githubRefreshLeeway = 5 * time.Minute

type GitHubAuthService struct {
	initializer   *ConfigInitializer
	httpClient    *http.Client
	now           func() time.Time
	tokenStore    GitHubTokenStore
	refreshLeeway time.Duration
}

type gitHubRefreshResponseError struct {
	StatusCode int
	Status     string
	Response   gitHubAccessTokenResponse
	Message    string
}

func (e *gitHubRefreshResponseError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if strings.TrimSpace(e.Response.Error) != "" {
		return formatGitHubTokenExchangeError(e.Response)
	}
	if strings.TrimSpace(e.Status) != "" {
		return e.Status
	}

	return "unexpected GitHub token refresh response"
}

func newGitHubAuthService(initializer *ConfigInitializer) *GitHubAuthService {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	return &GitHubAuthService{
		initializer: initializer,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
		tokenStore:    newKeychainGitHubTokenStore(),
		refreshLeeway: githubRefreshLeeway,
	}
}

func (s *GitHubAuthService) Refresh(ctx context.Context) error {
	_, _, err := s.refreshToken(ctx)
	return err
}

func (s *GitHubAuthService) EnsureValidToken(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.ensureDefaults(); err != nil {
		return "", err
	}

	config, err := loadGitHubLoginConfig(s.initializer)
	if err != nil {
		return "", err
	}

	token, err := s.loadStoredToken(config.Account)
	if err != nil {
		return "", err
	}

	if accessToken := strings.TrimSpace(token.AccessToken); accessToken != "" && accessTokenStillValid(token.AccessTokenExpiresAt, s.now().UTC(), s.refreshLeeway) {
		return accessToken, nil
	}

	_, refreshedToken, err := s.refreshToken(ctx)
	if err != nil {
		return "", err
	}

	return refreshedToken.AccessToken, nil
}

func (s *GitHubAuthService) ensureDefaults() error {
	if s.initializer == nil {
		s.initializer = newDefaultConfigInitializer()
	}
	if s.httpClient == nil {
		s.httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	if s.now == nil {
		s.now = func() time.Time {
			return time.Now().UTC()
		}
	}
	if s.tokenStore == nil {
		s.tokenStore = newKeychainGitHubTokenStore()
	}
	if s.refreshLeeway <= 0 {
		s.refreshLeeway = githubRefreshLeeway
	}

	return nil
}

func (s *GitHubAuthService) refreshToken(ctx context.Context) (githubLoginConfig, GitHubStoredToken, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.ensureDefaults(); err != nil {
		return githubLoginConfig{}, GitHubStoredToken{}, err
	}

	config, err := loadGitHubLoginConfig(s.initializer)
	if err != nil {
		return githubLoginConfig{}, GitHubStoredToken{}, err
	}

	storedToken, err := s.loadStoredToken(config.Account)
	if err != nil {
		return githubLoginConfig{}, GitHubStoredToken{}, err
	}

	refreshToken := strings.TrimSpace(storedToken.RefreshToken)
	if refreshToken == "" {
		return githubLoginConfig{}, GitHubStoredToken{}, fmt.Errorf("no refresh token is stored locally; please run `dev github login` again")
	}

	if expired, _ := evaluateExpiry(normalizeTimePointer(storedToken.RefreshTokenExpiresAt), s.now().UTC(), 0); expired {
		if err := s.invalidateStoredToken(config.Account); err != nil {
			return githubLoginConfig{}, GitHubStoredToken{}, fmt.Errorf("refresh token expired and local token state could not be cleared: %w", err)
		}

		return githubLoginConfig{}, GitHubStoredToken{}, fmt.Errorf("refresh token expired; please run `dev github login` again")
	}

	response, err := s.exchangeRefreshToken(ctx, config.AuthBaseURL, config.ClientID, refreshToken)
	if err != nil {
		var responseErr *gitHubRefreshResponseError
		if errors.As(err, &responseErr) {
			switch {
			case isGitHubRefreshAuthorizationError(responseErr):
				if clearErr := s.invalidateStoredToken(config.Account); clearErr != nil {
					return githubLoginConfig{}, GitHubStoredToken{}, fmt.Errorf("GitHub authorization is no longer valid and local token state could not be cleared: %w", clearErr)
				}

				return githubLoginConfig{}, GitHubStoredToken{}, fmt.Errorf("GitHub authorization is no longer valid; please run `dev github login` again")
			case isGitHubRefreshConfigurationError(responseErr):
				return githubLoginConfig{}, GitHubStoredToken{}, fmt.Errorf("GitHub client credentials are incorrect; check github.client_id")
			}
		}

		return githubLoginConfig{}, GitHubStoredToken{}, err
	}

	issuedAt := s.now().UTC()
	refreshedToken := buildGitHubStoredToken(config.APIBaseURL, config.AuthBaseURL.Host, response, issuedAt)
	if err := s.tokenStore.Save(config.Account, refreshedToken); err != nil {
		return githubLoginConfig{}, GitHubStoredToken{}, err
	}

	return config, refreshedToken, nil
}

func (s *GitHubAuthService) loadStoredToken(account string) (GitHubStoredToken, error) {
	token, err := s.tokenStore.Load(account)
	switch {
	case errors.Is(err, ErrGitHubTokenNotFound):
		return GitHubStoredToken{}, fmt.Errorf("no GitHub token is stored locally; please run `dev github login` again")
	case errors.Is(err, ErrGitHubStoredTokenInvalid):
		if clearErr := s.invalidateStoredToken(account); clearErr != nil {
			return GitHubStoredToken{}, fmt.Errorf("local GitHub token state is invalid and could not be cleared: %w", clearErr)
		}

		return GitHubStoredToken{}, fmt.Errorf("local GitHub token state is invalid; please run `dev github login` again")
	case err != nil:
		return GitHubStoredToken{}, err
	}

	if err := validateGitHubStoredToken(account, token); err != nil {
		if clearErr := s.invalidateStoredToken(account); clearErr != nil {
			return GitHubStoredToken{}, fmt.Errorf("local GitHub token state is invalid and could not be cleared: %w", clearErr)
		}

		return GitHubStoredToken{}, fmt.Errorf("local GitHub token state is invalid; please run `dev github login` again")
	}

	return token, nil
}

func (s *GitHubAuthService) exchangeRefreshToken(
	ctx context.Context,
	authBaseURL *url.URL,
	clientID string,
	refreshToken string,
) (gitHubAccessTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	endpoint := githubAuthEndpoint(authBaseURL, githubAccessTokenPath)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return gitHubAccessTokenResponse{}, fmt.Errorf("build refresh token request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return gitHubAccessTokenResponse{}, fmt.Errorf("refresh GitHub user token: send request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return gitHubAccessTokenResponse{}, fmt.Errorf("refresh GitHub user token: read response body: %w", err)
	}

	var tokenResponse gitHubAccessTokenResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &tokenResponse); err != nil {
			if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
				message := strings.TrimSpace(string(body))
				if message == "" {
					message = response.Status
				}

				return gitHubAccessTokenResponse{}, &gitHubRefreshResponseError{
					StatusCode: response.StatusCode,
					Status:     response.Status,
					Message:    message,
				}
			}

			return gitHubAccessTokenResponse{}, fmt.Errorf("refresh GitHub user token: decode response body: %w", err)
		}
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if tokenResponse.Error != "" {
			message = formatGitHubTokenExchangeError(tokenResponse)
		}
		if message == "" {
			message = response.Status
		}

		return gitHubAccessTokenResponse{}, &gitHubRefreshResponseError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Response:   tokenResponse,
			Message:    message,
		}
	}

	if strings.TrimSpace(tokenResponse.AccessToken) == "" {
		if strings.TrimSpace(tokenResponse.Error) != "" {
			return gitHubAccessTokenResponse{}, &gitHubRefreshResponseError{
				StatusCode: response.StatusCode,
				Status:     response.Status,
				Response:   tokenResponse,
				Message:    formatGitHubTokenExchangeError(tokenResponse),
			}
		}

		return gitHubAccessTokenResponse{}, fmt.Errorf("refresh GitHub user token: GitHub response did not include an access_token")
	}

	return tokenResponse, nil
}

func (s *GitHubAuthService) invalidateStoredToken(account string) error {
	return s.tokenStore.Delete(account)
}

func accessTokenStillValid(expiresAt *time.Time, now time.Time, leeway time.Duration) bool {
	if expiresAt == nil {
		return true
	}

	expired, nearExpiry := evaluateExpiry(normalizeTimePointer(expiresAt), now, leeway)
	return !expired && !nearExpiry
}

func validateGitHubStoredToken(account string, token GitHubStoredToken) error {
	if strings.TrimSpace(token.Host) != "" && strings.TrimSpace(token.Host) != strings.TrimSpace(account) {
		return fmt.Errorf("stored GitHub token host %q does not match account %q", token.Host, account)
	}
	if strings.TrimSpace(token.AccessToken) == "" && strings.TrimSpace(token.RefreshToken) == "" {
		return fmt.Errorf("stored GitHub token does not contain an access token or refresh token")
	}

	return nil
}

func isGitHubRefreshAuthorizationError(err *gitHubRefreshResponseError) bool {
	if err == nil {
		return false
	}

	if err.StatusCode == http.StatusUnauthorized {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(err.Response.Error), "bad_refresh_token")
}

func isGitHubRefreshConfigurationError(err *gitHubRefreshResponseError) bool {
	if err == nil {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(err.Response.Error), "incorrect_client_credentials")
}
