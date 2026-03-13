package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type GitHubLogoutRunner struct {
	service *GitHubAuthService
}

func newGitHubLogoutRunner(initializer *ConfigInitializer) *GitHubLogoutRunner {
	return &GitHubLogoutRunner{
		service: newGitHubAuthService(initializer),
	}
}

func (r *GitHubLogoutRunner) Run(ctx context.Context, stdout io.Writer, options GitHubLogoutOptions) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.service == nil {
		r.service = newGitHubAuthService(nil)
	}

	result, err := r.service.Logout(ctx, options)

	switch {
	case result.LocalStateCleared && result.LocalTokenFound:
		if _, writeErr := fmt.Fprintf(stdout, "GitHub logout completed. Local token state was cleared for %s.\n", result.Account); writeErr != nil {
			return writeErr
		}
	case result.LocalStateCleared:
		if _, writeErr := fmt.Fprintf(stdout, "GitHub logout completed. No local token was stored for %s.\n", result.Account); writeErr != nil {
			return writeErr
		}
	}

	if options.RevokeRemote {
		switch {
		case result.RemoteRevokeSucceeded:
			if _, writeErr := fmt.Fprintln(stdout, "Remote token revoked on GitHub."); writeErr != nil {
				return writeErr
			}
		case result.RemoteRevokeSkipped:
			if _, writeErr := fmt.Fprintf(stdout, "Remote token revoke was skipped: %s.\n", strings.TrimSpace(result.RemoteRevokeSkipReason)); writeErr != nil {
				return writeErr
			}
		}
	}

	return err
}
