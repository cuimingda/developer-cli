package cmd

import (
	"context"
	"fmt"
	"io"
)

type GitHubRefreshRunner struct {
	service *GitHubAuthService
}

func newGitHubRefreshRunner(initializer *ConfigInitializer) *GitHubRefreshRunner {
	return &GitHubRefreshRunner{
		service: newGitHubAuthService(initializer),
	}
}

func (r *GitHubRefreshRunner) Run(ctx context.Context, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.service == nil {
		r.service = newGitHubAuthService(nil)
	}

	config, refreshedToken, err := r.service.refreshToken(ctx)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "GitHub token refresh succeeded. Token saved to the macOS keychain for %s.\n", config.Account); err != nil {
		return err
	}
	if refreshedToken.AccessTokenExpiresAt != nil {
		if _, err := fmt.Fprintf(stdout, "Access token expires at %s.\n", formatLocalTimeDisplay(*refreshedToken.AccessTokenExpiresAt)); err != nil {
			return err
		}
	}
	if refreshedToken.RefreshTokenExpiresAt != nil {
		if _, err := fmt.Fprintf(stdout, "Refresh token expires at %s.\n", formatLocalTimeDisplay(*refreshedToken.RefreshTokenExpiresAt)); err != nil {
			return err
		}
	}

	return nil
}
