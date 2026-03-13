package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	githubTokenKeychainService = developerIdentifier + "." + cliName + ".github.user-token"
	githubTokenKeychainLabel   = cliName + " github user token"
)

var ErrGitHubTokenNotFound = errors.New("github token not found")
var ErrGitHubStoredTokenInvalid = errors.New("github stored token invalid")

type GitHubTokenStore interface {
	Save(account string, token GitHubStoredToken) error
	Load(account string) (GitHubStoredToken, error)
	Delete(account string) error
}

type KeychainGitHubTokenStore struct {
	runCommand func(name string, args ...string) ([]byte, error)
}

type GitHubStoredToken struct {
	APIBaseURL            string     `json:"api_base_url"`
	Host                  string     `json:"host"`
	AccessToken           string     `json:"access_token"`
	TokenType             string     `json:"token_type,omitempty"`
	Scope                 string     `json:"scope,omitempty"`
	IssuedAt              time.Time  `json:"issued_at"`
	AccessTokenExpiresAt  *time.Time `json:"access_token_expires_at,omitempty"`
	RefreshToken          string     `json:"refresh_token,omitempty"`
	RefreshTokenExpiresAt *time.Time `json:"refresh_token_expires_at,omitempty"`
}

func newKeychainGitHubTokenStore() *KeychainGitHubTokenStore {
	return &KeychainGitHubTokenStore{
		runCommand: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
	}
}

func (s *KeychainGitHubTokenStore) Save(account string, token GitHubStoredToken) error {
	if strings.TrimSpace(account) == "" {
		return fmt.Errorf("keychain account is empty")
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return fmt.Errorf("github access token is empty")
	}
	if s == nil || s.runCommand == nil {
		return fmt.Errorf("keychain command runner is not configured")
	}

	payload, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal github token payload: %w", err)
	}

	args := []string{
		"add-generic-password",
		"-U",
		"-a", account,
		"-s", githubTokenKeychainService,
		"-l", githubTokenKeychainLabel,
		"-w", string(payload),
	}

	output, err := s.runCommand("security", args...)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return fmt.Errorf("write github token to keychain: %w", err)
		}

		return fmt.Errorf("write github token to keychain: %w: %s", err, message)
	}

	return nil
}

func (s *KeychainGitHubTokenStore) Load(account string) (GitHubStoredToken, error) {
	if strings.TrimSpace(account) == "" {
		return GitHubStoredToken{}, fmt.Errorf("keychain account is empty")
	}
	if s == nil || s.runCommand == nil {
		return GitHubStoredToken{}, fmt.Errorf("keychain command runner is not configured")
	}

	output, err := s.runCommand(
		"security",
		"find-generic-password",
		"-a", account,
		"-s", githubTokenKeychainService,
		"-w",
	)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if isKeychainItemNotFound(message) {
			return GitHubStoredToken{}, ErrGitHubTokenNotFound
		}
		if message == "" {
			return GitHubStoredToken{}, fmt.Errorf("read github token from keychain: %w", err)
		}

		return GitHubStoredToken{}, fmt.Errorf("read github token from keychain: %w: %s", err, message)
	}

	var token GitHubStoredToken
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &token); err != nil {
		return GitHubStoredToken{}, fmt.Errorf("%w: decode github token payload: %v", ErrGitHubStoredTokenInvalid, err)
	}

	return token, nil
}

func (s *KeychainGitHubTokenStore) Delete(account string) error {
	if strings.TrimSpace(account) == "" {
		return fmt.Errorf("keychain account is empty")
	}
	if s == nil || s.runCommand == nil {
		return fmt.Errorf("keychain command runner is not configured")
	}

	output, err := s.runCommand(
		"security",
		"delete-generic-password",
		"-a", account,
		"-s", githubTokenKeychainService,
	)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if isKeychainItemNotFound(message) {
			return nil
		}
		if message == "" {
			return fmt.Errorf("delete github token from keychain: %w", err)
		}

		return fmt.Errorf("delete github token from keychain: %w: %s", err, message)
	}

	return nil
}

func isKeychainItemNotFound(message string) bool {
	lowerMessage := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(lowerMessage, "could not be found in the keychain") ||
		strings.Contains(lowerMessage, "the specified item could not be found")
}
