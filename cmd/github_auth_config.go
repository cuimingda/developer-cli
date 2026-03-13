package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const githubClientSecretEnv = "GITHUB_CLIENT_SECRET"

type githubAuthBaseConfig struct {
	APIBaseURL  string
	AuthBaseURL *url.URL
	Account     string
}

func loadGitHubAuthBaseConfig(initializer *ConfigInitializer) (githubAuthBaseConfig, error) {
	apiBaseURL, err := initializer.GetValue("github.api_base_url")
	if err != nil {
		return githubAuthBaseConfig{}, err
	}
	authBaseURL, err := githubAuthBaseURL(apiBaseURL)
	if err != nil {
		return githubAuthBaseConfig{}, err
	}

	return githubAuthBaseConfig{
		APIBaseURL:  strings.TrimSpace(apiBaseURL),
		AuthBaseURL: authBaseURL,
		Account:     authBaseURL.Host,
	}, nil
}

func loadGitHubLoginConfig(initializer *ConfigInitializer) (githubLoginConfig, error) {
	clientID, err := initializer.GetValue("github.client_id")
	if err != nil {
		return githubLoginConfig{}, err
	}
	if strings.TrimSpace(clientID) == "" {
		return githubLoginConfig{}, fmt.Errorf("config value github.client_id is empty")
	}

	baseConfig, err := loadGitHubAuthBaseConfig(initializer)
	if err != nil {
		return githubLoginConfig{}, err
	}

	return githubLoginConfig{
		ClientID:    strings.TrimSpace(clientID),
		APIBaseURL:  baseConfig.APIBaseURL,
		AuthBaseURL: baseConfig.AuthBaseURL,
		Account:     baseConfig.Account,
	}, nil
}

func loadGitHubClientSecret(initializer *ConfigInitializer) (string, bool, error) {
	if value, ok := os.LookupEnv(githubClientSecretEnv); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed, true, nil
		}
	}

	value, present, err := optionalConfigValue(initializer, "github.client_secret")
	if err != nil {
		return "", false, err
	}

	return strings.TrimSpace(value), present && strings.TrimSpace(value) != "", nil
}
