package cmd

import (
	"fmt"
	"strings"
)

func loadGitHubLoginConfig(initializer *ConfigInitializer) (githubLoginConfig, error) {
	clientID, err := initializer.GetValue("github.client_id")
	if err != nil {
		return githubLoginConfig{}, err
	}
	if strings.TrimSpace(clientID) == "" {
		return githubLoginConfig{}, fmt.Errorf("config value github.client_id is empty")
	}

	apiBaseURL, err := initializer.GetValue("github.api_base_url")
	if err != nil {
		return githubLoginConfig{}, err
	}
	authBaseURL, err := githubAuthBaseURL(apiBaseURL)
	if err != nil {
		return githubLoginConfig{}, err
	}

	return githubLoginConfig{
		ClientID:    strings.TrimSpace(clientID),
		APIBaseURL:  strings.TrimSpace(apiBaseURL),
		AuthBaseURL: authBaseURL,
		Account:     authBaseURL.Host,
	}, nil
}
