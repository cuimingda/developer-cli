package cmd

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestKeychainGitHubTokenStoreSaveWritesStructuredPayload(t *testing.T) {
	store := &KeychainGitHubTokenStore{}

	var commandName string
	var commandArgs []string
	store.runCommand = func(name string, args ...string) ([]byte, error) {
		commandName = name
		commandArgs = append([]string(nil), args...)
		return []byte("ok"), nil
	}

	issuedAt := time.Date(2026, time.March, 13, 10, 0, 0, 0, time.UTC)
	accessTokenExpiresAt := issuedAt.Add(8 * time.Hour)
	refreshTokenExpiresAt := issuedAt.Add(180 * 24 * time.Hour)

	token := GitHubStoredToken{
		APIBaseURL:            "https://api.github.com",
		Host:                  "github.com",
		AccessToken:           "access-token",
		TokenType:             "bearer",
		Scope:                 "",
		IssuedAt:              issuedAt,
		AccessTokenExpiresAt:  &accessTokenExpiresAt,
		RefreshToken:          "refresh-token",
		RefreshTokenExpiresAt: &refreshTokenExpiresAt,
	}

	if err := store.Save("github.com", token); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	if commandName != "security" {
		t.Fatalf("command name = %q, want %q", commandName, "security")
	}

	wantPrefix := []string{
		"add-generic-password",
		"-U",
		"-a", "github.com",
		"-s", githubTokenKeychainService,
		"-l", githubTokenKeychainLabel,
		"-w",
	}
	if !reflect.DeepEqual(commandArgs[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("command args prefix = %#v, want %#v", commandArgs[:len(wantPrefix)], wantPrefix)
	}

	if len(commandArgs) != len(wantPrefix)+1 {
		t.Fatalf("len(commandArgs) = %d, want %d", len(commandArgs), len(wantPrefix)+1)
	}

	var stored GitHubStoredToken
	if err := json.Unmarshal([]byte(commandArgs[len(commandArgs)-1]), &stored); err != nil {
		t.Fatalf("Unmarshal() returned error: %v", err)
	}

	if !reflect.DeepEqual(stored, token) {
		t.Fatalf("stored token = %#v, want %#v", stored, token)
	}
}

func TestKeychainGitHubTokenStoreSaveIncludesSecurityOutputOnFailure(t *testing.T) {
	store := &KeychainGitHubTokenStore{
		runCommand: func(name string, args ...string) ([]byte, error) {
			return []byte("user interaction is not allowed"), errors.New("exit status 36")
		},
	}

	err := store.Save("github.com", GitHubStoredToken{
		AccessToken: "access-token",
		IssuedAt:    time.Date(2026, time.March, 13, 10, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected Save() to return an error")
	}

	if got := err.Error(); got != "write github token to keychain: exit status 36: user interaction is not allowed" {
		t.Fatalf("error = %q, want %q", got, "write github token to keychain: exit status 36: user interaction is not allowed")
	}
}

func TestKeychainGitHubTokenStoreLoadReadsStructuredPayload(t *testing.T) {
	store := &KeychainGitHubTokenStore{}

	issuedAt := time.Date(2026, time.March, 13, 10, 0, 0, 0, time.UTC)
	accessTokenExpiresAt := issuedAt.Add(8 * time.Hour)
	token := GitHubStoredToken{
		APIBaseURL:           "https://api.github.com",
		Host:                 "github.com",
		AccessToken:          "access-token",
		IssuedAt:             issuedAt,
		AccessTokenExpiresAt: &accessTokenExpiresAt,
	}

	payload, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("Marshal() returned error: %v", err)
	}

	store.runCommand = func(name string, args ...string) ([]byte, error) {
		return payload, nil
	}

	loadedToken, err := store.Load("github.com")
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !reflect.DeepEqual(loadedToken, token) {
		t.Fatalf("loaded token = %#v, want %#v", loadedToken, token)
	}
}

func TestKeychainGitHubTokenStoreLoadReturnsNotFoundError(t *testing.T) {
	store := &KeychainGitHubTokenStore{
		runCommand: func(name string, args ...string) ([]byte, error) {
			return []byte("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain."), errors.New("exit status 44")
		},
	}

	_, err := store.Load("github.com")
	if !errors.Is(err, ErrGitHubTokenNotFound) {
		t.Fatalf("Load() error = %v, want ErrGitHubTokenNotFound", err)
	}
}

func TestKeychainGitHubTokenStoreLoadReturnsInvalidPayloadError(t *testing.T) {
	store := &KeychainGitHubTokenStore{
		runCommand: func(name string, args ...string) ([]byte, error) {
			return []byte("{not-json"), nil
		},
	}

	_, err := store.Load("github.com")
	if !errors.Is(err, ErrGitHubStoredTokenInvalid) {
		t.Fatalf("Load() error = %v, want ErrGitHubStoredTokenInvalid", err)
	}
}

func TestKeychainGitHubTokenStoreDeleteRemovesStoredToken(t *testing.T) {
	store := &KeychainGitHubTokenStore{}

	var commandName string
	var commandArgs []string
	store.runCommand = func(name string, args ...string) ([]byte, error) {
		commandName = name
		commandArgs = append([]string(nil), args...)
		return []byte("deleted"), nil
	}

	if err := store.Delete("github.com"); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}

	if commandName != "security" {
		t.Fatalf("command name = %q, want %q", commandName, "security")
	}

	wantArgs := []string{
		"delete-generic-password",
		"-a", "github.com",
		"-s", githubTokenKeychainService,
	}
	if !reflect.DeepEqual(commandArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", commandArgs, wantArgs)
	}
}

func TestKeychainGitHubTokenStoreDeleteIgnoresMissingToken(t *testing.T) {
	store := &KeychainGitHubTokenStore{
		runCommand: func(name string, args ...string) ([]byte, error) {
			return []byte("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain."), errors.New("exit status 44")
		},
	}

	if err := store.Delete("github.com"); err != nil {
		t.Fatalf("Delete() returned error: %v", err)
	}
}

func TestKeychainGitHubTokenStoreDeleteIncludesSecurityOutputOnFailure(t *testing.T) {
	store := &KeychainGitHubTokenStore{
		runCommand: func(name string, args ...string) ([]byte, error) {
			return []byte("user interaction is not allowed"), errors.New("exit status 36")
		},
	}

	err := store.Delete("github.com")
	if err == nil {
		t.Fatal("expected Delete() to return an error")
	}

	if got := err.Error(); got != "delete github token from keychain: exit status 36: user interaction is not allowed" {
		t.Fatalf("error = %q, want %q", got, "delete github token from keychain: exit status 36: user interaction is not allowed")
	}
}
